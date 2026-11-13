// Command decarta-bot is a Telegram bot that validates restaurant menus and
// publishes them to a static site repository.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"

	"github.com/luis-serrano-l/decarta-bot/internal/bot"
	"github.com/luis-serrano-l/decarta-bot/internal/config"
	"github.com/luis-serrano-l/decarta-bot/internal/menuid"
	"github.com/luis-serrano-l/decarta-bot/internal/publisher"
	"github.com/luis-serrano-l/decarta-bot/internal/ratelimit"
	"github.com/luis-serrano-l/decarta-bot/internal/redact"
	"github.com/luis-serrano-l/decarta-bot/internal/store"
)

const (
	// accessRequestsPerHour bounds how often one user can ping the admins.
	accessRequestsPerHour = 1
	// uploadsPerWindow bounds how many menus one user can have checked and
	// previewed. It is deliberately looser than the publish limit, which
	// meters commits: correcting a menu and sending it again is normal.
	uploadsPerWindow = 20
	uploadWindow     = 10 * time.Minute
	shutdownGrace    = 10 * time.Second
)

func main() {
	os.Exit(run())
}

func run() int {
	healthcheck := flag.Bool("healthcheck", false, "probe the local /health endpoint and exit 0 if healthy (used by the container HEALTHCHECK)")
	flag.Parse()

	// A .env file is a convenience for local runs; in containers the
	// environment is injected and the file does not exist.
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(os.Stderr, "warning: could not read .env; using the environment only")
	}

	if *healthcheck {
		return probe(os.Getenv("PORT"))
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		// Config errors name variables, never values.
		fmt.Fprintf(os.Stderr, "configuration error:\n%v\n", err)
		return 2
	}

	// Every log line, including those of the Telegram library, passes
	// through the redactor.
	out := redact.NewWriter(os.Stderr, cfg.Secrets()...)
	logger := slog.New(slog.NewTextHandler(out, nil))
	if err := tgbotapi.SetLogger(log.New(out, "telegram: ", log.LstdFlags)); err != nil {
		logger.Error("could not set the telegram logger", "err", err)
		return 1
	}

	if err := serve(cfg, logger); err != nil {
		logger.Error("fatal", "err", err)
		return 1
	}
	return 0
}

func serve(cfg *config.Config, logger *slog.Logger) error {
	ids, err := menuid.New(cfg.HashSecret)
	if err != nil {
		return err
	}
	pub, err := publisher.NewGitHub(cfg.GitHubToken, cfg.GitHubRepo, cfg.GitHubBranch)
	if err != nil {
		return err
	}
	st, err := store.NewFile(cfg.StoragePath)
	if err != nil {
		return err
	}

	client := bot.NewClient(cfg.TelegramToken, bot.DefaultTelegramAPI, logger)
	handler := bot.NewHandler(
		bot.Options{
			SiteBaseURL:    cfg.SiteBaseURL,
			AllowedUserIDs: cfg.AllowedUserIDs,
			AdminUserIDs:   cfg.AdminUserIDs,
			MaxUploadBytes: cfg.MaxUploadBytes,
		},
		ids, pub, st,
		ratelimit.New(cfg.PublishLimit, cfg.PublishWindow, nil),
		ratelimit.New(uploadsPerWindow, uploadWindow, nil),
		ratelimit.New(accessRequestsPerHour, time.Hour, nil),
		client, logger,
	)

	srv := &http.Server{
		Addr:              net.JoinHostPort("", strconv.Itoa(cfg.Port)),
		Handler:           healthHandler(client.Connected),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       time.Minute,
		MaxHeaderBytes:    8 << 10,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("starting",
		"port", cfg.Port,
		"repo", cfg.GitHubRepo,
		"branch", cfg.GitHubBranch,
		"publishers", len(cfg.AllowedUserIDs)+len(cfg.AdminUserIDs),
		"max_upload_bytes", cfg.MaxUploadBytes,
		"publish_limit", cfg.PublishLimit,
		"publish_window", cfg.PublishWindow.String(),
	)
	if len(cfg.AdminUserIDs) == 0 {
		logger.Warn("ADMIN_USER_IDS is empty: nobody will receive access requests")
	}

	httpErr := make(chan error, 1)
	go func() { httpErr <- srv.ListenAndServe() }()
	botDone := make(chan error, 1)
	go func() { botDone <- client.Run(ctx, handler.Handle) }()

	var runErr error
	select {
	case <-ctx.Done():
		logger.Info("shutting down")
	case err := <-httpErr:
		runErr = fmt.Errorf("health server: %w", err)
	case err := <-botDone:
		if err != nil && !errors.Is(err, context.Canceled) {
			runErr = fmt.Errorf("bot: %w", err)
		}
		botDone = nil // already drained
	}
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("health server shutdown", "err", err)
	}
	if botDone != nil {
		select {
		case err := <-botDone:
			if err != nil && !errors.Is(err, context.Canceled) {
				logger.Warn("bot stopped with an error", "err", err)
			}
		case <-shutdownCtx.Done():
			logger.Warn("in-flight updates did not finish before the shutdown deadline")
		}
	}
	return runErr
}

// healthHandler serves GET /health and nothing else. It reports liveness:
// the process is up and able to answer. Whether Telegram has accepted the
// token is included as information, because a wrong token is an operator
// error that restarting the container would not fix.
func healthHandler(connected func() bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		telegram := "connecting"
		if connected() {
			telegram = "connected"
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = fmt.Fprintf(w, `{"status":"ok","telegram":%q}`+"\n", telegram)
	})
	return mux
}

// probe is the container health check: distroless images have no shell or
// curl, so the binary checks itself.
func probe(port string) int {
	n := config.DefaultPort
	port = strings.TrimSpace(port) // as config.Load does
	if port != "" {
		var err error
		if n, err = strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			fmt.Fprintln(os.Stderr, "unhealthy: PORT is not a valid port number")
			return 1
		}
	}
	client := &http.Client{Timeout: 3 * time.Second}
	// Loopback only, and the port is a validated integer.
	resp, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(n)) + "/health") //nolint:gosec,noctx // see above; client has a timeout
	if err != nil {
		fmt.Fprintln(os.Stderr, "unhealthy:", err)
		return 1
	}
	defer resp.Body.Close() //nolint:errcheck // exiting anyway
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "unhealthy: HTTP", resp.StatusCode)
		return 1
	}
	return 0
}
