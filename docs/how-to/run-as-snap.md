# How to run the bot as a snap

The snap runs the bot as a system service on any Linux with
[snapd](https://snapcraft.io/docs/installing-snapd), without Docker. It is
strictly confined: the bot can reach the network and write to its own data
directory, and nothing else.

This guide assumes you have a bot token
([how](create-botfather-token.md)), a GitHub token
([how](create-github-token.md)) and a site repository with GitHub Pages
enabled. If you have none of these yet, follow the
[tutorial](../tutorial/first-menu.md) first.

## Install

```bash
sudo snap install decarta-bot --edge
```

`edge` is built from every commit on `main`. A released version is in
`candidate`, and in `stable` once it has passed the
[smoke test](smoke-test.md).

The service is installed stopped, because the bot cannot do anything
before it is configured.

## Configure

The snap reads the same `.env` format as every other way of running the
bot, from its own directory. Create the file readable by root only, since
it holds two tokens:

```bash
sudo install -m 600 /dev/null /var/snap/decarta-bot/common/.env
sudoedit /var/snap/decarta-bot/common/.env
```

Fill in at least the five required variables from
[.env.example](../../.env.example); the
[environment reference](../reference/environment.md) describes each one.
Do not set `STORAGE_PATH`: the snap fixes it to
`/var/snap/decarta-bot/common/data`.

## Start

```bash
sudo snap start --enable decarta-bot
```

`--enable` also starts it at boot.

## Check that it works

```bash
snap services decarta-bot       # enabled, active
sudo snap logs decarta-bot      # look for "telegram: polling for updates"
sudo snap run decarta-bot -healthcheck && echo healthy
snap run decarta-bot --version
```

The snap installs a service and no command of its own, so the binary is
reached through `snap run`.

Then send `/status` to the bot in Telegram.

## Change the configuration

Edit the file, then restart:

```bash
sudoedit /var/snap/decarta-bot/common/.env
sudo snap restart decarta-bot
```

To grant someone access, add their ID to `ALLOWED_USER_IDS` this way.

## Update

snapd refreshes the snap by itself, several times a day, and restarts the
service. To update now, or to hold a version back:

```bash
sudo snap refresh decarta-bot
sudo snap refresh --hold decarta-bot
```

The configuration and the data directory survive a refresh, and
`sudo snap revert decarta-bot` returns to the previous version.

## Stop and remove

```bash
sudo snap stop --disable decarta-bot
sudo snap remove decarta-bot        # keeps a snapshot of the data for 31 days
```

Published menus stay online: they live in the site repository, not in the
bot.

## What is different from the container

| | Container | Snap |
|---|---|---|
| User | Unprivileged (`65532`) | `root`, restricted by the snap's confinement |
| `/health` | Reachable only inside the container | Listens on `PORT` (8080) on every interface of the host. It only ever says whether the bot is up; close the port in your firewall if the host is on the public internet. |
| Updates | When you pull and rebuild | Automatic |
| Configuration | `.env` next to `compose.yaml` | `/var/snap/decarta-bot/common/.env` |

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| `snap services` shows `disabled inactive` | Expected after installing. Configure, then `sudo snap start --enable decarta-bot`. |
| Logs show `configuration error:` and the service restarts every 30 seconds | A required variable is missing or malformed, or the `.env` file is not at the path above. The message names each variable. |
| `bind: address already in use` in the logs | Something else uses port 8080. Set `PORT` in the `.env` file to a free port. |

Everything in the Docker guide's
[troubleshooting table](run-with-docker.md#troubleshooting) about Telegram,
GitHub and the site applies here too; read `sudo snap logs decarta-bot`
where it says `docker compose logs bot`.

## For maintainers: publishing the snap

CI builds the snap for amd64 and arm64 on every push and pull request,
installs it and checks its health endpoint. On `main` it then publishes to
`edge`, and on a version tag to `candidate`, if the repository has a store
login. Setting that up is done once:

1. Create an [Ubuntu One](https://login.ubuntu.com/) account and register
   the name:

   ```bash
   sudo snap install snapcraft --classic
   snapcraft login
   snapcraft register decarta-bot
   ```

2. Export a login limited to this snap and store it as a repository
   secret:

   ```bash
   snapcraft export-login --snaps=decarta-bot \
     --acls package_access,package_push,package_update,package_release \
     creds.txt
   gh secret set SNAPCRAFT_STORE_CREDENTIALS < creds.txt
   rm creds.txt
   ```

   The login expires after a year; repeat this step then.

3. After a release has passed the smoke test, promote it:

   ```bash
   snapcraft release decarta-bot <revision> stable
   ```

   `snapcraft status decarta-bot` lists the revisions in each channel.
