# ☁️ Nimbus Sync

**One-way cloud → local folder sync for Linux.** Mirror folders from **Microsoft OneDrive**, **Google Drive**, and **Dropbox** to your local disk from a single, dependency-light Go CLI — with scheduled runs, dry-run mode, exclusion patterns, and JSON/HTML sync reports.

```
nimbus auth gdrive          # authorize once
nimbus sync --dry-run       # preview
nimbus sync                 # mirror cloud → local
nimbus daemon               # keep syncing on a schedule
```

## Why

Backing up cloud storage to a local machine (NAS, home server, laptop) shouldn't require a heavyweight GUI client per provider. Nimbus Sync gives you one small static binary that:

- pulls from **OneDrive (Microsoft Graph)**, **Google Drive (Drive v3)**, and **Dropbox (API v2)**
- syncs any number of named remotes, each mapping a cloud folder to a local directory
- only downloads new or changed files (size + mtime comparison, atomic writes)
- optionally mirrors deletions (`delete_local: true`)
- produces a **report** (terminal table, JSON archive, exportable HTML) after every run
- runs on a **schedule** via its built-in cron daemon *or* a systemd user timer

## Features

| Feature | Detail |
|---|---|
| Providers | OneDrive, Google Drive, Dropbox (OAuth 2.0, tokens auto-refresh) |
| Direction | Cloud → local (one-way mirror; upload is on the roadmap) |
| Change detection | Size + modification time, atomic `.nimbus-tmp` writes, remote mtime preserved |
| Concurrency | Parallel downloads per remote (`concurrency` in config) |
| Exclusions | Glob patterns per remote (`*.tmp`, `~$*`, `.DS_Store`, …) |
| Deletion mirror | Optional per remote (`delete_local`) |
| Dry run | `--dry-run` shows every action without writing |
| Reports | Terminal summary, timestamped JSON history, `nimbus report --html out.html` |
| Scheduling | Built-in cron daemon (`nimbus daemon`) or systemd service + timer |
| Platform | Linux amd64/arm64, single static binary, no CGO |

## Installation

### From source (recommended)

Requires Go 1.22+.

```bash
git clone https://github.com/ivansostarko/nimbus-sync.git
cd nimbus-sync
./scripts/install.sh
```

The installer builds the binary, copies it to `/usr/local/bin/nimbus`, seeds `~/.config/nimbus-sync/config.yaml` from the example, and offers to enable the systemd user timer.

### Manual build

```bash
make build            # → bin/nimbus
sudo make install     # → /usr/local/bin/nimbus
make release          # cross-compiled dist/ binaries for amd64 + arm64
```

### Uninstall

```bash
./scripts/uninstall.sh
```

## Configuration

Config lives at `~/.config/nimbus-sync/config.yaml`. See [`configs/config.example.yaml`](configs/config.example.yaml) for a fully commented example.

```yaml
schedule: "0 */6 * * *"     # cron, used by `nimbus daemon`
concurrency: 4

providers:
  gdrive:
    client_id: "…"
    client_secret: "…"

remotes:
  - name: personal-gdrive
    provider: gdrive
    remote_path: "Photos/2026"
    local_path: "~/CloudSync/GDrive/Photos-2026"
    enabled: true
    delete_local: false
    exclude: ["*.tmp"]
```

Tokens are stored per provider in `~/.config/nimbus-sync/tokens/` with `0600` permissions; reports in `~/.config/nimbus-sync/reports/`.

## Creating API credentials

Each provider requires a (free) OAuth app so Nimbus can act on your behalf. Add `http://localhost:8676/callback` as the redirect URI in every case.

### Microsoft OneDrive
1. Go to the [Azure Portal → App registrations](https://portal.azure.com) and create a new registration.
2. Supported account types: *Personal Microsoft accounts* (or org accounts as needed).
3. Add a **Web** redirect URI: `http://localhost:8676/callback`.
4. Under *API permissions*, add Microsoft Graph delegated permissions: `Files.Read.All`, `offline_access`, `User.Read`.
5. Copy the *Application (client) ID* into `providers.onedrive.client_id`. A client secret is optional for public clients.

### Google Drive
1. In the [Google Cloud Console](https://console.cloud.google.com), create a project and enable the **Google Drive API**.
2. Configure the OAuth consent screen (External, add yourself as a test user).
3. Create an **OAuth client ID** of type *Web application* with redirect URI `http://localhost:8676/callback`.
4. Copy client ID and secret into `providers.gdrive`.

### Dropbox
1. Create an app in the [Dropbox App Console](https://www.dropbox.com/developers/apps) with scoped access (`files.metadata.read`, `files.content.read`).
2. Add redirect URI `http://localhost:8676/callback`.
3. Copy the *App key* / *App secret* into `providers.dropbox`.

Then authorize each provider once:

```bash
nimbus auth onedrive
nimbus auth gdrive
nimbus auth dropbox
```

A browser URL is printed; approve access and the token is stored locally. Refresh tokens keep it working indefinitely.

## Usage

```bash
nimbus sync                        # sync all enabled remotes
nimbus sync --remote work-onedrive # one remote only
nimbus sync --dry-run --verbose    # preview with detail
nimbus remotes                     # list configured remotes
nimbus report                      # print latest report
nimbus report --html report.html   # export HTML report
nimbus daemon                      # foreground scheduler (cron from config)
```

Exit code is non-zero when any remote fails, so Nimbus plays nicely with cron, systemd, and monitoring.

## Scheduling

**Option A — built-in daemon.** `nimbus daemon` runs an immediate sync, then follows the `schedule` cron expression in the config. Run it under tmux, supervisord, or a systemd service.

**Option B — systemd user timer (recommended on Linux desktops/servers).** The installer can set this up, or manually:

```bash
mkdir -p ~/.config/systemd/user
cp systemd/nimbus-sync.{service,timer} ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now nimbus-sync.timer
systemctl --user list-timers nimbus-sync.timer   # verify
journalctl --user -u nimbus-sync.service         # logs
```

For headless servers, enable lingering so the timer runs without an active login session: `sudo loginctl enable-linger $USER`.

**Option C — plain cron.**

```cron
0 */6 * * * /usr/local/bin/nimbus sync >> ~/.config/nimbus-sync/cron.log 2>&1
```

## Reports

Every run writes a timestamped JSON report to `~/.config/nimbus-sync/reports/`:

```json
{
  "started": "2026-07-10T08:00:01Z",
  "remotes": [
    {
      "name": "personal-gdrive",
      "status": "ok",
      "downloaded": ["IMG_2041.jpg"],
      "unchanged": 1523,
      "bytes": 4194304,
      "errors": []
    }
  ]
}
```

`nimbus report` prints the latest one; `nimbus report --html sync.html` renders a shareable HTML page with per-remote tables and file lists.

## Project layout

```
cmd/nimbus/          CLI entrypoint and subcommands
internal/config/     YAML config loading + validation
internal/auth/       OAuth2 flow, token storage & refresh
internal/providers/  OneDrive / Google Drive / Dropbox backends
internal/sync/       Sync engine (diffing, worker pool, atomic writes)
internal/report/     Report model, JSON persistence, HTML export
internal/scheduler/  Cron daemon
configs/             Example configuration
scripts/             install.sh / uninstall.sh
systemd/             User service + timer units
```

## Limitations & roadmap

- One-way (cloud → local) only for now; two-way sync and upload are planned.
- Google-native files (Docs/Sheets/Slides) are skipped — export-to-Office/PDF is planned.
- Change detection uses size + mtime; per-provider hash verification is planned (each provider uses a different hash algorithm: quickXorHash / MD5 / Dropbox content hash).
- No delta/incremental listing APIs yet (full listing per run).

Contributions welcome — see below.

## Contributing

1. Fork and clone the repo.
2. `make test && make lint` must pass.
3. Open a PR with a clear description; small, focused changes are easiest to review.

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## Security

- Tokens and config are stored with `0600` permissions under your user config dir.
- Nimbus requests **read-only** scopes wherever the provider offers them.
- Never commit `config.yaml` or `tokens/` — both are in `.gitignore`.
- Found a vulnerability? Please open a private security advisory on GitHub rather than a public issue.

## License

[MIT](LICENSE)
