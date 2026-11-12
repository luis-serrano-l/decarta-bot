# demenu_bot

Telegram bot that validates restaurant menus and publishes them to the
companion static site ([luis-serrano-l/demenu](https://github.com/luis-serrano-l/demenu)).

## Commands

```bash
go build ./...                      # build
go run .                            # run locally (reads .env)
go test -race ./...                 # tests
gofmt -l . && go vet ./...          # format check + vet
golangci-lint run                   # lint (v2, config in .golangci.yml)
govulncheck ./...                   # known-vulnerability scan
docker compose up --build --wait    # container (needs .env), waits for HEALTHCHECK
```

## Architecture

1. `main.go` wires everything: config, logging, health server, bot; exits on SIGINT/SIGTERM.
2. `internal/config` reads env vars and refuses to start on missing or weak settings.
3. `internal/bot` long-polls Telegram (the only mode) and handles commands, menu submissions and inline buttons; a menu is previewed and published only after the user taps Publish.
4. `internal/menu` parses the list or JSON format into typed structs and validates them strictly.
5. `internal/menuid` derives the public id: base62 of a truncated HMAC-SHA256(HASH_SECRET, userID); not reversible.
6. `internal/ratelimit` is an in-memory sliding window (default 5 publishes per user per hour).
7. `internal/publisher` is the `Publisher` interface; `GitHub` commits (or deletes) `static/menus/<id>.json` via the Contents API, `Fake` is for tests.
8. `internal/store` keeps the last published copy per id on the data volume (`STORAGE_PATH`).
9. `internal/redact` scrubs secrets from all log output; user IDs are logged only as the HMAC id.
10. The only HTTP endpoint is `GET /health`; menus are served by the static site, never by the bot.

## Documentation

`docs/` follows Diátaxis: `tutorial/`, `how-to/`, `reference/`, `explanation/`.
When behaviour changes, update the matching reference page, `CHANGELOG.md`
(Keep a Changelog, under Unreleased) and, for commands, `Commands` in
`internal/bot/handler.go`.

## Conventions

- Conventional commits; small commits; no new dependencies without a reason.
- Table-driven tests; no network in tests (use `httptest` and the fakes).
- Never log tokens, raw Telegram user IDs or chat IDs.
