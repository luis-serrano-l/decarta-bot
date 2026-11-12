# Security policy

## Supported versions

Security fixes are made on `main`. There are no maintained release
branches yet.

## Reporting a vulnerability

Please report vulnerabilities privately through
[GitHub security advisories](https://github.com/luis-serrano-l/demenu_bot/security/advisories/new),
not in a public issue. Include what you did, what happened and what you
expected; a failing test or a menu file that triggers the problem is
ideal. Never include real tokens.

You should get a reply within a week. Once a fix is released, the
advisory is published and you are credited unless you prefer otherwise.

## Scope

In scope: anything that lets someone publish or delete a menu they should
not be able to, get content past validation into the site repository,
read a secret or a Telegram user ID from logs or replies, or take the bot
down with a crafted message.

Out of scope: what an authorised publisher writes in their own menu,
attacks that need access to the host or to `.env`, and problems in
Telegram, GitHub or the companion site (report those to
[luis-serrano-l/demenu](https://github.com/luis-serrano-l/demenu)).

## Security model in brief

- Only the Telegram users in `ALLOWED_USER_IDS` and `ADMIN_USER_IDS` can
  publish or delete, and only their own menu. Publishing and deleting
  need a confirmation button that is single-use, bound to its user and
  expires after 15 minutes.
- Everything users send is untrusted: it is size-capped, strictly parsed,
  validated and re-encoded before it is published.
- A menu's public id is base62 of a truncated HMAC-SHA256 of the Telegram
  user ID under `HASH_SECRET`. It cannot be reversed without the secret.
  Menus themselves are public by design.
- The bot holds two credentials: the Telegram bot token and a GitHub
  token. Give the GitHub token access to the site repository only, with
  **Contents: Read and write**.
- Logs never contain tokens, and identify users only by their HMAC id.
  Administrators see a requester's Telegram ID in `/request_access`
  notifications, which is what they need to grant access.
- The bot long-polls Telegram, so it accepts no inbound connections. Its
  only HTTP endpoint is `GET /health`, which exposes no user data.

The full analysis, including what is deliberately not defended, is in the
[threat model](docs/explanation/threat-model.md).
