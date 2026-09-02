# Contributing

This file is the single source for `go-proxy-manager` project conventions.
Read it before opening a PR.

## Prerequisites

- Go 1.26+
- `git` on `PATH` (the test suite shells out to it)

## Build and test

```
make build   # binary in bin/gpm
make test    # go test ./... (hermetic: httptest, fakes, t.TempDir())
make vet     # go vet ./...
make lint    # staticcheck
make vuln    # govulncheck
```

All four of `test`, `vet`, `lint`, and `vuln` are CI gates and must pass
before merge.

## Conventions

- **Language and logging.** Go, with zerolog as the one accepted logging
  dependency; no other logging library.
- **Zero new dependencies by default.** Prefer the Go standard library and
  `golang.org/x/...`. Every new third-party import must be justified in the
  PR description: reach for one only when the stdlib genuinely cannot do
  the job. Minimizing the dependency and advisory surface is the point of
  this project.
- **Config conventions.** Config is git-backed YAML (`config/<kind>/<name>.yaml`
  plus a single `config/settings.yaml`); secrets are referenced via
  `${ENV:}` / `${FILE:}` placeholders and never committed in plaintext.
  Nothing that should live in git is UI- or DB-only.
- **gofmt, go vet and staticcheck clean.** Run `gofmt -l .`, `go vet ./...`
  and `make lint` before pushing; all three are CI gates, alongside
  `make test` and `make vuln`.
- **Table-driven, hermetic tests.** No real network calls, no real DNS
  providers, ACME CAs, or IdPs, and no deployment-specific paths or domains.
  Use `example.com`, RFC 5737, or RFC 1918 documentation ranges in test
  fixtures and examples.
- **ASCII only** in source and docs: no em-dashes, no smart quotes, no
  curly punctuation.
- **Docs stay in sync with code.** For any change (feature, fix, refactor,
  or tweak), update every affected doc in the same PR:
  - `README.md` for the feature list and anything a first-time reader sees.
  - `docs/reference/config/<kind>.md` for every config object, field or
    default touched (one page per object kind), plus
    `docs/reference/config/settings/<section>.md` for a settings key.
  - `docs/concepts/architecture.md` when a data-plane or control-plane
    mechanism is added or changes behaviour, and
    `docs/concepts/request-pipeline.md` if it changes the chain order, the
    path composition or the client-IP derivation.
  - `docs/getting-started/` for installation changes and `docs/operations/`
    for backup, upgrade, HA, hardening or profiling steps; anything
    requiring an operator action on upgrade also goes in the version table
    in `docs/operations/upgrading.md`.
  - `docs/reference/env-vars-and-flags.md` for flags, env vars, listeners
    and CLI subcommands.
  - `docs/api/openapi.yaml` for every route (a test enforces coverage).
  - `internal/ui/hints/hints.json` for any new UI control (a test enforces
    coverage).
  - `CHANGELOG.md` for every notable Added / Changed / Fixed / Security
    change.
  - `FEATURES.md` when a roadmap item ships or its scope changes, and
    `BACKLOG.md`: check items off when done, add newly identified
    follow-ups.
- **Backwards compatibility.** Existing YAML under `/data/config` must keep
  loading. Deprecate a field rather than removing it (keep the struct field,
  mark it `// Deprecated:`, drop it from the UI/OpenAPI/docs) unless the
  change ships in a major version with a documented migration.

## Commits and sign-off

- No sign-off (DCO) is required.
- Write commit messages that describe the change, not the process of making
  it.

## Pull requests

- Target `main`. Promotion from `main` to the `prod` release branch is
  maintainer-run (see `.github/workflows/sync-prod.yml`); contributors do
  not need to touch it.
- Fill in the PR template: it mirrors the docs-in-sync checklist above.
- A PR that changes behavior without a corresponding doc/changelog update
  will be asked to add one before merge.

## Design notes

Design notes for contributors live in [design/](design/), outside the
published docs site.

## Reporting bugs and requesting features

Use the issue templates under `.github/ISSUE_TEMPLATE/`. Open-ended
questions belong in [Discussions](../../discussions), not issues.

## Security

Do not open a public issue for a suspected vulnerability: see
[SECURITY.md](SECURITY.md) for the private reporting process.
