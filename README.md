# Go Proxy Manager

Exploratory idea: a Go rewrite of an Nginx Proxy Manager-style reverse-proxy
manager. **Status: idea only, not started.**

## Why consider this

Running the private NPM fork (`example/nginx-proxy-manager`, v2.15.1 + OIDC PR
#5494) surfaced the usual Node dependency churn and a steady stream of advisory
noise (Dependabot flagged ~29 advisories in the inherited tree). A focused Go
implementation with a small, vendored dependency set could cut that maintenance
and security-surface burden - if it can be done without recreating the same mess.

## Decision gate before committing

- Is the dependency footprint genuinely smaller/cleaner in Go for this problem?
- Ongoing security surface vs. the fork: actually better, or just different?
- Effort to reach feature parity vs. value over maintaining the fork.

## Parity targets (what NPM does today)

- Proxy hosts / streams / redirection hosts / 404 hosts, generating nginx (or a
  native Go proxy) config.
- Let's Encrypt issuance + renewal (DNS-01 wildcard, like the current setup).
- Access lists (IP allow/deny) + forward-auth compatibility.
- OIDC/SSO admin login (the feature we forked NPM to get) + local break-glass.
- Web admin UI + API.

## Notes

- Project conventions: Go, zerolog for logging (project convention).
- Placeholder repo; revisit when there's appetite to prototype.
