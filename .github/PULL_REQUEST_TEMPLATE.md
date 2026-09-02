## What changed

<!-- One or two sentences: what changed and why. -->

## Docs-in-sync checklist

Check every box that applies (leave unrelated ones unchecked: see
CONTRIBUTING.md "Docs stay in sync with code"):

- [ ] `README.md`: feature list or anything a first-time reader sees
- [ ] `docs/configuration.md`: every config object/field/default touched
- [ ] `docs/architecture.md`: new or changed data-plane/control-plane mechanism
- [ ] `docs/api/openapi.yaml`: every new/changed route (enforced by `internal/server/openapi_test.go`)
- [ ] `docs/deployment.md`: flags, env vars, ports, upgrade caveats
- [ ] `CHANGELOG.md`: `[Unreleased]` entry (Added/Changed/Fixed/Security)
- [ ] `BACKLOG.md`: checked off a done item, or added a newly identified follow-up
- [ ] `FEATURES.md`: a roadmap item shipped or its scope changed
- [ ] N/A: this change has no user-visible or config-shape effect

## Tests

- [ ] `go test ./...` passes
- [ ] `gofmt -l .` is clean
- [ ] `go vet ./...` is clean
- [ ] New/changed behavior has a table-driven, hermetic test (no real network, DNS, ACME CA, or IdP)

## Breaking changes / upgrade notes

<!-- If this requires an operator action on upgrade, it must also be listed
     under an "### Upgrade notes" heading in CHANGELOG.md [Unreleased].
     Otherwise, write NONE. -->

NONE
