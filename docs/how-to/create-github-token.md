# How to create the fine-grained GitHub token

`GITHUB_TOKEN` lets the bot commit menu files to the site repository. Use
a **fine-grained** personal access token, limited to that one repository
and one permission, so that a leaked token can do nothing else.

## Create the token

1. Sign in as the account that owns the site repository (or, for an
   organisation, an account with write access to it).
2. Open
   [github.com/settings/personal-access-tokens/new](https://github.com/settings/personal-access-tokens/new)
   (**Settings → Developer settings → Personal access tokens →
   Fine-grained tokens → Generate new token**).
3. **Token name**: something you will recognise, such as `demenu_bot`.
4. **Resource owner**: the owner of the site repository.
5. **Expiration**: choose a date. GitHub emails you before it expires.
6. **Repository access**: **Only select repositories**, and select the
   site repository only.
7. **Permissions → Repository permissions → Contents**: **Read and
   write**. GitHub adds **Metadata: Read-only** automatically. Grant
   nothing else.
8. **Generate token**, and copy it now: GitHub shows it once. It starts
   with `github_pat_`.

Put it in `.env`, with the repository it belongs to:

```ini
GITHUB_TOKEN=github_pat_...
GITHUB_REPO=<owner>/demenu
GITHUB_BRANCH=main
```

If the repository belongs to an organisation, an owner may have to approve
the token before it works (**Organisation settings → Personal access
tokens → Pending requests**).

## What the token is used for

Exactly three API calls, all on `static/menus/<id>.json` in `GITHUB_REPO`:

| Call | When |
|---|---|
| `GET /repos/{repo}/contents/static/menus/{id}.json` | To find the current version of a menu before replacing or deleting it |
| `PUT` on the same path | A user taps **Publish** |
| `DELETE` on the same path | A user confirms `/delete` |

Each publish or delete is one commit on `GITHUB_BRANCH`, with the message
`menu: update <id>` or `menu: delete <id>`.

## Check it

```bash
docker compose up -d --force-recreate --wait
```

Then publish a menu. If the bot answers "could not be published right
now", run `docker compose logs bot`:

| Log says | Meaning |
|---|---|
| `HTTP 401` | The token is wrong, expired or revoked |
| `HTTP 403` or `HTTP 404` | The token cannot write to `GITHUB_REPO`: wrong repository selected, **Contents** not set to **Read and write**, wrong `GITHUB_BRANCH`, or organisation approval pending |

## Rotate it

1. Create a new token as above.
2. Replace `GITHUB_TOKEN` in `.env` and recreate the container.
3. Delete the old token at
   [github.com/settings/personal-access-tokens](https://github.com/settings/personal-access-tokens).

Do this before the expiry date, and immediately if the token may have
leaked.
