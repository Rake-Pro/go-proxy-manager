# Security policy

## Supported versions

Only the latest minor release is supported with security fixes. Pin
deployments to a semver tag or image digest; never `:latest`. See
[CHANGELOG.md](CHANGELOG.md) for release history.

| Version | Supported |
|---|---|
| Latest minor (see the newest `vX.Y.Z` tag) | yes |
| Older minors | no |

## Reporting a vulnerability

- **Do not open a public issue for a suspected vulnerability.**
- Report privately via [GitHub Security Advisories](../../security/advisories/new)
  for this repository.
- Include: affected version, deployment type (Docker/bare-metal/Kubernetes),
  a minimal reproduction, and the impact you believe it has.

## Response targets

| Stage | Target |
|---|---|
| Acknowledgement | 5 business days |
| Initial assessment (severity, affected versions) | 10 business days |
| Fix or mitigation, once confirmed | best-effort, scaled to severity |

## Scope notes

gpm terminates TLS and handles authentication (OIDC, forward-auth,
client-cert/mTLS, local admin login) for every request that crosses it. Its
attack surface includes:

- The data plane (public ports 80/443): TLS termination, routing, the
  middleware chain, and every auth gate.
- The control plane (admin port 8081): the REST API and web UI, which are
  **not** meant to be exposed directly to the internet - see
  [docs/deployment.md](docs/deployment.md) and the README's Security model
  section.
- The git-backed config store: secrets are referenced via `${ENV:}` /
  `${FILE:}` placeholders and never committed in plaintext by design; a
  report that a literal secret can end up committed is in scope.

Out of scope: vulnerabilities in a proxied backend application itself, or in
an operator's own reverse-proxy/ingress fronting gpm's admin plane.
