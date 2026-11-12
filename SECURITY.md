# Security policy

## Reporting a vulnerability

Please report vulnerabilities privately through
[GitHub security advisories](https://github.com/luis-serrano-l/demenu_bot/security/advisories/new)
rather than a public issue. You should get a reply within a week.

## Security model

- Only the Telegram users listed in `ALLOWED_USER_IDS` and `ADMIN_USER_IDS`
  can publish. Everything they send is untrusted: it is size-capped,
  strictly parsed, validated and re-encoded before it is published.
- A menu's public id is base62 of a truncated HMAC-SHA256 of the Telegram
  user ID under `HASH_SECRET`. It cannot be reversed without the secret.
  Menus themselves are public by design.
- The bot holds two credentials: the Telegram bot token and a GitHub token.
  Give the GitHub token access to the site repository only, with
  "Contents: Read and write".
- Logs never contain tokens, and identify users only by their HMAC id.
  Administrators see a requester's Telegram ID in `/request_access`
  notifications, which is what they need to grant access.
- The only HTTP endpoint is `GET /health`; it exposes no user data.
