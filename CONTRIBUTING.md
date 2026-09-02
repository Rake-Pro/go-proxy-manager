# Contributing

Read [CLAUDE.md](CLAUDE.md) first - it is the working-notes file for this
project and states the conventions below in full.

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

- **Zero new dependencies by default.** Prefer the Go standard library and
  `golang.org/x/...`. Every new third-party import must be justified in the
  PR description - reach for one only when the stdlib genuinely cannot do
  the job. Minimizing the dependency and advisory surface is the point of
  this project.
- **gofmt clean.** Run `gofmt -l .` before pushing; CI rejects unformatted
  files.
- **Table-driven, hermetic tests.** No real network calls, no real DNS
  providers, ACME CAs, or IdPs, and no deployment-specific paths or domains.
  Use `example.com`, RFC 5737, or RFC 1918 documentation ranges in test
  fixtures and examples.
- **ASCII only** in source and docs - no em-dashes, no smart quotes, no
  curly punctuation.
- **Docs stay in sync with code.** For any change - feature, fix, refactor,
  or tweak - update every affected doc in the same PR: `README.md`,
  `docs/configuration.md`, `docs/architecture.md`, `docs/deployment.md`,
  `docs/api/openapi.yaml` (a test enforces route coverage),
  `CHANGELOG.md` (`[Unreleased]`), `BACKLOG.md`, and `FEATURES.md`. See
  CLAUDE.md's "Keep documentation in sync with the code" section for the
  full list and reasoning.
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
- Fill in the PR template - it mirrors the docs-in-sync checklist above.
- A PR that changes behavior without a corresponding doc/changelog update
  will be asked to add one before merge.

## Reporting bugs and requesting features

Use the issue templates under `.github/ISSUE_TEMPLATE/`. Open-ended
questions belong in [Discussions](../../discussions), not issues.

## Security

Do not open a public issue for a suspected vulnerability - see
[SECURITY.md](SECURITY.md) for the private reporting process.
