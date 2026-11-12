# Contributing

Thanks for helping. Bug reports, documentation fixes and code are all
welcome. For a vulnerability, do not open an issue: see
[SECURITY.md](SECURITY.md).

## Set up

You need Go 1.26 or newer. Docker is only needed for the container check.

```bash
git clone https://github.com/luis-serrano-l/demenu_bot.git
cd demenu_bot
go test -race ./...
```

The tests need no network, no tokens and no `.env`: Telegram and GitHub
are replaced by fakes.

To try the bot for real, follow the
[quick start](README.md#quick-start) with a throwaway bot and a fork of
the site.

## Before you open a pull request

Run what CI runs:

```bash
gofmt -l .                          # must print nothing
go vet ./...
golangci-lint run                   # v2, config in .golangci.yml
go test -race ./...
go mod tidy -diff                   # must print nothing
govulncheck ./...
docker compose up --build --wait    # needs a .env; checks the image and its HEALTHCHECK
```

## Conventions

- **Commits** follow [Conventional Commits](https://www.conventionalcommits.org/)
  (`feat(bot): …`, `fix(menu): …`, `docs: …`). Keep them small: one
  change per commit.
- **Tests** are table-driven and never touch the network. Use
  `net/http/httptest`, `publisher.Fake` and the fake Telegram client in
  `internal/bot/handler_test.go`. A new command or button needs a test.
- **Dependencies**: do not add one without saying why in the pull request.
- **Logging**: never log tokens, raw Telegram user IDs, chat IDs,
  usernames or names. Identify a user by `u.id`, the HMAC menu id.
- **User-facing text**: write for a restaurant owner on a phone. Say what
  happened and what to do next; for a rejected menu, name the field and
  the fix.
- **Docs** follow [Diátaxis](https://diataxis.fr/). A change in behaviour
  updates the matching page under [docs/reference](docs/reference), and
  gets a line under `Unreleased` in [CHANGELOG.md](CHANGELOG.md), which
  follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## Where things are

The package layout is described in
[docs/explanation/design.md](docs/explanation/design.md#small-pieces).
Adding a command usually touches three places: `Commands` and `command`
in `internal/bot/handler.go`, a test in `internal/bot/handler_test.go`,
and [docs/reference/commands.md](docs/reference/commands.md).

## Pull requests

- Describe what changes for the user or the operator, and how you tested
  it. Say plainly what you could not test (for example, against the real
  Telegram or GitHub APIs).
- Keep a pull request to one topic.
- CI must pass.

By contributing you agree that your work is released under the
[MIT licence](LICENSE).
