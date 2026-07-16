# Design: High availability (gpm itself)

Status: **proposal (not started).** Upstream groups already remove the
single-node dependency *behind* gpm (a host's backends fail over); the gpm
instance itself is still a single point of failure. This document settles a
supported multi-instance story before any code lands, per the BACKLOG item
"High availability (gpm itself)". It resolves the five open questions raised
there: config replication, shared secrets + single-writer ACME, SSO revocation
propagation, traffic-side failover, and stream/UDP state.

The target is the real deployment: a **two-node homelab edge pair**, each an
instance of the single-binary/single-compose stack described in
[deployment.md](../deployment.md). The design is deliberately sized for that,
not for a large fleet.

## Dependency posture (the deciding lens)

This project exists to escape dependency/advisory churn, and the constraint is
sharper for HA than for anything else: the obvious industry answer is a
consensus store (etcd/consul/raft), and that is exactly what we will **not**
adopt. Every mechanism below is built from tools already in the stack.

| Building block | Mechanism | New dependency |
|---|---|---|
| Config replication | `git pull --ff-only` (the store is already a git repo) | **none** (git already required) |
| Cert / secret replication | shared volume or `rsync` of the cert dir | none (operator-supplied) |
| Leader designation (phase 1) | static role via env | **none** |
| Leader election (phase 2, optional) | lease file on shared storage (atomic rename + mtime TTL) | none |
| Traffic failover | keepalived VRRP VIP (operator infra) | none in gpm |
| SSO revocation propagation | periodic re-read of the existing watermark file | none (stdlib `os.ReadFile` on a ticker) |

Common thread: HA is **replicate the durable state, elect a single writer, and
let one process serve at a time.** No node talks to another node's process; they
communicate only through replicated files. That keeps gpm's core untouched and
adds zero clustering libraries.

The git surface already anticipates this: `store.GitRepo` carries a
`PullFFOnly(ctx)` method (fast-forwards from a configured remote, and
**refuses** a diverged history rather than auto-merging or discarding). It has
no non-test caller today - the follower poll loop below is its first user.

---

## 1. What is per-instance state (the inventory that drives everything)

HA is only tractable once every piece of durable or in-memory state is
classified. Reading the code, gpm's state splits cleanly into three buckets.
"Replicate" = copied leader->follower on change; "Share" = one copy both read;
"Lossy" = per-instance, not synchronised, and acceptable to lose on failover.

| State | Where | Verdict | Rationale |
|---|---|---|---|
| Config objects (`/data/config`, git) | `internal/store` | **Replicate** | git-native; leader commits, follower `pull --ff-only`. Sole source of routing/TLS/auth config. |
| ACME issued certs + `meta.json` (`<cert-dir>/acme/issued/*`) | `internal/acme/store.go` | **Share/Replicate** | follower must serve the same TLS leaf; **not** in git (private keys). Single writer (leader). |
| ACME account key (`<cert-dir>/acme/accounts/*.key`) | `internal/acme/account.go` | **Share/Replicate** | one account per directory URL; both must present the same account on renewal. |
| SSO signing key (`<cert-dir>/sso_signing.key` or `GPM_SSO_SIGNING_KEY`) | `internal/dataplane/oidcgate.go` | **Share** | data-plane SSO cookies are HMAC-signed and stateless; a shared key makes any node accept any node's cookie. Prefer the env var (one secret, out of the file-sync path). |
| SSO revocation watermark (`<cert-dir>/sso_not_before`) | `internal/dataplane/oidcgate.go` | **Share + watch** | read once at startup today (see section 4); HA needs a poll so a peer honors a revocation without a restart. |
| Admin session DB (`/data/session.db`, SQLite) | `internal/session` | **Lossy** | control-plane only. On failover an admin re-logs in. Sharing it is possible (shared path) but low value and adds a write-contention edge; document as lossy. |
| Access-log ring (in-memory, opt-in) | `internal/dataplane/accesslog.go` | **Lossy** | bounded best-effort viewer buffer; each node shows its own recent traffic. |
| Rate-limit token buckets (in-memory, per host middleware) | `internal/dataplane/ratelimit.go` | **Lossy** | in active/standby only one node serves, so limits are correct; in active/active they are enforced per-node (effectively 2x). Documented, not replicated. |
| Local-login lockout + pending-OIDC/throttle maps (in-memory) | `internal/auth/authenticator.go` | **Lossy** | control-plane; a login in flight at failover simply restarts, lockout counters reset. Negligible for a 2-node pair. |
| TCP/UDP stream listeners + UDP session table (in-memory) | `internal/dataplane/streamhost.go` | **Lossy (failover-with-reconnect)** | connectionless/connection state is non-replicable; see section 6. |

The two sharp edges fall straight out of this table:

1. **Config and secrets replicate on different channels.** The config repo is
   git; the cert dir (issued certs, account key, SSO key, watermark) is plain
   files with private keys and **cannot** go in git. HA must wire *both*.
2. **Almost all lossy state is control-plane or best-effort.** The only
   data-plane lossy state that a client notices is stream reconnect (section 6).
   That is what makes a simple active/standby design correct.

---

## 2. (a) Config replication between instances

**Requirement:** the follower must run the exact config the leader committed,
picked up without a restart, with no risk of the two repos silently diverging.

### Options

- **A. Follower pulls directly from the leader's repo.** The follower adds the
  leader's `/data/config` as a git remote (over ssh, e.g.
  `ssh://gpm@leader/data/config`) and runs `git pull --ff-only` on a poll
  interval. No push, no third component; reuses `PullFFOnly` verbatim.
  *Trade-off:* while the leader is down the follower cannot pull - but it keeps
  serving the last-synced config on disk, which is exactly what we want.
- **B. Shared bare repo.** A bare repo (on either node, or a small shared
  location) is the hub: the leader `git push`es after each `CommitAll`, the
  follower `pull --ff-only`s from it. Decouples the follower's sync from leader
  liveness. *Trade-off:* one more moving part, and the leader needs a new
  `Push` verb (the store has none today).
- **C. Shared filesystem for `/data/config`.** Both mount the same repo.
  *Rejected:* two processes running `git add -A`/`commit`/`read-tree` against
  one working tree race destructively (the store's `RestoreTree` does
  `read-tree --reset -u` + `clean -fd`), and it defeats the point of an
  independent standby.

### Decision: **A (follower pulls from the leader), git-native, ff-only.**

Cheapest and it reuses the existing `PullFFOnly` as-is with zero new git verbs.
The follower runs a poll loop: `PullFFOnly` -> if `Head()` changed, call the
existing `reload()` path (which re-reads from disk and re-applies to data plane
+ auth). Divergence is impossible by construction - the follower never commits,
and `pull --ff-only` refuses anything that is not a clean fast-forward, so a
follower that somehow acquired a local commit surfaces an error instead of
merging. Adopt B later only if leader-independent sync becomes worth the extra
component (it adds a `Push` to `GitRepo`).

Poll interval: config changes are rare and operator-driven, so 15-30s is ample;
it bounds how long the follower serves stale config after a leader write.

---

## 3. (b) Shared secrets and single-writer ACME

Two problems, one answer (a single writer).

**Shared secrets.** For failover to be seamless the follower must already hold:
`GPM_SSO_SIGNING_KEY` (so it accepts SSO cookies minted by the leader), the ACME
account key, and the issued certs. The SSO key is trivially shared by **setting
`GPM_SSO_SIGNING_KEY` identically on both nodes** (env), which also keeps it out
of the file-sync path. The ACME account key and issued certs live under the cert
dir and are **not** in git (they contain private keys); they replicate on a
separate channel - a **shared volume** for `<cert-dir>/acme` (single copy, only
the leader writes) or an **`rsync`-on-change** leader->follower. Phase 1
recommends the shared volume for `<cert-dir>/acme` and `<cert-dir>/sso_not_before`
because the single-writer gate below makes concurrent writes a non-issue.

**Single-writer ACME.** `acme.Manager.Run` renews on a per-process ticker and
serialises issuance only with an **in-process** mutex (`m.mu`). Two instances
both running that loop would race the same order: duplicate issuance, wasted
Let's Encrypt rate limit, and two divergent keypairs for one cert (last atomic
write wins, the other node keeps a stale key). ACME **must** have exactly one
writer.

### Leader election options (no new deps)

- **Static leader (env).** One node is declared leader
  (`GPM_HA_ROLE=leader|follower`, default `leader` so a lone node is unchanged).
  Leader runs the ACME loop and accepts admin writes; follower disables the ACME
  loop and serves the admin API read-only (its config arrives only via pull).
  *Trade-off:* leadership does not move automatically - promoting the follower
  after a leader loss is an operator action (flip the env, redeploy).
- **Lease file (phase 2).** A lease on shared storage (write `{holder, expiry}`
  via atomic `os.Rename`, renew before a short TTL, take over when expired)
  gives automatic election with stdlib only. More moving parts and split-brain
  corner cases; not needed for phase 1.
- **DB advisory lock.** A lock row in a shared SQLite/DB. *Rejected:* would
  require sharing (and contending on) the session DB or introducing a new DB,
  both against the grain here.

### Decision: **static leader (env) for phase 1.**

DNS-01 needs no inbound port and no VIP, so the leader renews certs regardless
of which node currently holds the traffic VIP - ACME is decoupled from failover.
For operational clarity, make the static leader the node that is *normally* the
keepalived MASTER, so "leader" and "active" usually coincide. A leader outage
degrades to: follower keeps serving the last replicated certs/config, renewals
pause, admin is read-only, until the operator promotes the follower. For a
2-node homelab with 30-day-before-expiry renewal (`renewBefore` default), a
pause of hours-to-days is harmless. Automatic election is deferred to phase 2.

---

## 4. (c) SSO revocation propagation

**The gap.** `RevokeAllSSOSessions` moves the `sso_not_before` watermark to now
and persists it next to the signing key; the data-plane gate rejects any SSO
cookie issued strictly before the watermark. But `ssoRevokedAt()` reads that
file **exactly once**, lazily, under a `sync.Once`, caching it in an
`atomic.Int64` for the process lifetime. So a peer instance that shares the
signing key will keep honoring a revoked cookie until *its own* next restart -
the watermark it loaded at startup never advances. Sharing the key without
fixing this actively *weakens* revocation: a stolen cookie rejected on the
leader still works on the follower.

### Design: replace the one-shot read with a bounded poll

- Persist/replicate the watermark on the shared channel (it already lives under
  the cert dir alongside the SSO key). `RevokeAllSSOSessions` writes to the
  shared path; because admin writes are leader-only (section 3), the revoke is
  issued on the leader and the file is the single source of truth.
- Add a **watermark refresh loop** on every instance: on a ticker (e.g. 15-30s)
  re-read `sso_not_before` and advance the in-memory atomic **monotonically**
  (`if v > current { store(v) }` - never move it backwards, matching the
  existing clock-skew guard in `RevokeAllSSOSessions`). The atomic is already
  the read path for the gate, so no gate change is needed; only its source
  refreshes. This turns the `sync.Once` one-shot into a periodic reconcile.
- On a shared volume this is a plain re-read; on `rsync` replication it inherits
  the sync latency. Either way, revocation propagates within one poll interval
  instead of "next restart."

This is the minimal, stdlib-only fix (`os.ReadFile` on a ticker) and it also
improves the single-node story - a watermark edited out-of-band is now honored
live.

---

## 5. (d) Traffic-side failover between instances

Steering traffic to the surviving node is **out of gpm's process scope** (gpm
does not move IPs), but the deployment doc must document at least one concrete
pattern. For a homelab edge pair the natural fit is a **VRRP virtual IP via
keepalived**, active/standby.

### Recommended pattern: keepalived VIP (active/standby)

- A floating VIP is advertised by whichever node is VRRP MASTER; public DNS (and
  the firewall/NAT forward for :80/:443, plus any published stream ports) points
  at the VIP. keepalived health-checks the local gpm (a TCP check on :443, or an
  HTTP probe) and demotes a node whose gpm is unhealthy, floating the VIP to the
  peer within a couple of VRRP intervals.
- **Client IP is preserved.** VRRP is an L2 same-subnet mechanism, so there is
  no SNAT: the real client source IP reaches gpm unchanged. This matters -
  access lists, GeoIP, rate limiting, and XFF trust all key off the resolved
  client IP, and a DNS-failover or naive L4-NAT approach would break or muddy
  that. The VIP approach keeps the existing client-IP resolver correct with no
  changes.
- **Interaction with ACME.** None on the traffic path: DNS-01 uses no inbound
  port, so the leader issues/renews whether or not it currently holds the VIP.
  Recommend leader == normally-MASTER so the active node is also the writer, but
  it is not required for correctness.
- **Interaction with streams.** Both instances bind their TCP/UDP stream
  listeners continuously (each runs its own `streamManager`), so the survivor
  already has the ports open when the VIP arrives - no bind delay, no rebind
  race. Existing flows drop and reconnect (section 6).

### Alternatives (documented, not recommended for the homelab)

- **DNS failover** (low-TTL A record swap): simple, no infra, but TTL-bound and
  resolver-cache-bound recovery (minutes), and it muddies client IP if fronted
  by anything that proxies. Fine as a coarse cross-site fallback, not for
  sub-second edge failover.
- **External L4 balancer** (both nodes active behind an upstream LB): enables
  active/active but requires the per-node lossy state (rate limits, sessions) to
  be acceptable-per-node or shared, and adds a component the homelab does not
  have. Deferred with phase 2 active/active.

---

## 6. (e) Stream (TCP/UDP) listeners and UDP session state

`streamManager` holds per-instance in-memory state that is **non-replicable by
nature**: live TCP `net.Conn` pairs, and a UDP session table mapping each client
address to a backend `net.Conn` (`udpForwarder.sessions`, idle-reaped after 60s,
capped at `maxUDPSessions`). None of this can be handed to another process.

**Verdict: failover-with-reconnect, documented as expected behavior.**

- On failover the survivor has an **empty** session table. A TCP client's
  existing connection is severed and it dials again (the survivor's listener is
  already bound and accepts immediately). A UDP client simply keeps sending; its
  first post-failover packet creates a fresh backend session on the survivor
  (`forward` allocates on cache miss). UDP being connectionless, "reconnect" is
  transparent for stateless protocols and a re-handshake for stateful ones.
- This is the same reconnect semantics a client already sees on a single-node
  restart, so it introduces no new failure mode - it just needs to be stated in
  the deployment doc so operators do not expect stream sessions to survive a
  failover.

No code change is required for stream failover beyond having both nodes run the
listeners (which they do by default). It is purely a documentation item.

---

## Recommended phase 1: active/standby, 2-node homelab

The minimal thing a two-node pair can actually run, with no new gpm
dependencies:

1. **keepalived VIP, active/standby.** Public DNS + firewall forward point at
   the VIP; keepalived health-checks gpm on :443 and floats the VIP on failure.
2. **Static leader (`GPM_HA_ROLE=leader|follower`).** Leader = normally-MASTER
   node: runs the ACME loop, accepts admin writes. Follower: ACME loop off,
   admin API read-only.
3. **Config replication.** Follower polls `git pull --ff-only` from the leader's
   repo (reusing `PullFFOnly`) every 15-30s and calls the existing `reload()`
   when HEAD advances.
4. **Cert + secret replication.** `<cert-dir>/acme` and
   `<cert-dir>/sso_not_before` on a shared volume (leader-only writer), and
   `GPM_SSO_SIGNING_KEY` set identically on both nodes.
5. **SSO revocation.** Add the watermark refresh loop (section 4) so a revoke
   issued on the leader is honored by the follower within a poll interval.
6. **Streams.** Both nodes bind listeners; failover-with-reconnect (section 6),
   documented.
7. **Accept as lossy:** admin sessions (re-login on failover), access-log ring,
   rate-limit buckets, login-lockout/throttle maps.

New code this requires is small and additive: a follower poll loop (config pull
+ conditional reload), a leader/follower role gate on the ACME loop and admin
writes, and the SSO watermark refresh ticker. Nothing in the data-plane hot path
changes.

## Phase 2 sketch (only where genuinely needed)

- **Automatic leader election** via a lease file on shared storage (atomic
  rename + mtime TTL, stdlib only), so a leader loss promotes the follower
  without operator action. Best coupled to keepalived state (a `notify_master`
  script that flips the role) so "active" and "leader" stay in lockstep with one
  source of truth.
- **Config sync via a shared bare repo** (option B in section 2) if
  leader-independent sync becomes worth a `Push` verb on `GitRepo`.
- **Active/active behind an L4 balancer**, if it is ever wanted - explicitly
  gated on making the per-node lossy state (rate limits especially) either
  acceptable-per-node or shared. For a homelab pair it is not worth it, and
  phase 1 active/standby is the recommended terminal state.

## Non-goals

- **No consensus/clustering dependency.** No etcd, consul, raft, or embedded
  equivalent - the entire point of the project is to avoid that surface.
- **No multi-writer / synchronous config consensus.** Exactly one writer
  (leader) at any time; the follower is read-only.
- **No replication of live stream state.** TCP/UDP sessions are failover-with-
  reconnect, never handed between instances.
- **No active/active by default.** Phase 1 is active/standby; active/active is a
  phase-2 option, not the baseline.
- **No split-brain hardening beyond a 2-node pair.** The design assumes keepalived
  + a single designated writer, not a large quorum-based fleet.
- **No multi-region / geographic failover.** Same-subnet VRRP is the target;
  cross-site is at most the coarse DNS fallback noted in section 5.

## Suggested sequencing

1. **SSO watermark refresh loop** - smallest, and it improves the single-node
   story too (out-of-band watermark edits honored live). No HA prerequisites.
2. **Leader/follower role gate** on the ACME loop and admin writes (env-driven).
3. **Follower config poll loop** (`PullFFOnly` + conditional `reload`).
4. **Deployment doc**: keepalived VIP recipe, cert-dir shared-volume layout,
   `GPM_SSO_SIGNING_KEY` sharing, and the stream failover-with-reconnect note.

**Effort:** S (watermark loop) + S (role gate) + M (poll loop + docs). No new
dependency; no data-plane hot-path change.
