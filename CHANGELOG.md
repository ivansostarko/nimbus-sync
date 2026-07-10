# Changelog

## 0.1.0 — 2026-07-10
Initial release.
- OneDrive, Google Drive, and Dropbox providers (OAuth 2.0, auto token refresh)
- One-way cloud → local mirror with atomic writes and mtime preservation
- Exclusion globs, optional deletion mirroring, per-remote enable flag
- Dry-run mode, verbose logging
- JSON report history + terminal summary + HTML export
- Built-in cron daemon and systemd user service/timer
- Linux installer & uninstaller scripts, Makefile with cross-compile release target
