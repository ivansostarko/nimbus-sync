# Contributing to Nimbus Sync

Thanks for your interest in improving Nimbus Sync!

## Development setup

```bash
git clone https://github.com/YOUR_USERNAME/nimbus-sync.git
cd nimbus-sync
go build ./...
go test ./...
```

Go 1.22+ is required. The project has only three direct dependencies
(`robfig/cron`, `golang.org/x/oauth2`, `yaml.v3`) — please keep it that way
unless there's a strong reason.

## Guidelines

- Run `make test` and `make lint` (go vet) before opening a PR.
- Keep providers behind the `providers.Provider` interface; new backends
  (Box, S3, Nextcloud, ...) should slot in without touching the sync engine.
- One logical change per PR, with a short description of *why*.
- New user-facing behavior needs a README update.

## Reporting bugs

Open a GitHub issue including:
- `nimbus version` output
- your distro and Go version (if building from source)
- redacted config snippet and the relevant report/log output

## Feature roadmap (help wanted)

- [ ] Upload / two-way sync
- [ ] Google Docs export (Docs → docx, Sheets → xlsx)
- [ ] Provider-native hash verification
- [ ] Delta listing APIs (Graph delta, Drive changes, Dropbox cursor reuse)
- [ ] Bandwidth limiting
- [ ] Desktop notifications on failure
