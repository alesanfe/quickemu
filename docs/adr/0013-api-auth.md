# ADR 0013: API exposure model — loopback by default, token-gated writes

## Status: accepted

## Context
`quickemu-api` serves read-only VM introspection plus a mutating
`/v1/vm-cmd/` endpoint. A static bearer token is proportionate for
localhost, but silently binding `0.0.0.0` with no token would expose
start/stop/kill to the LAN.

## Decision
- Default listen address is `127.0.0.1`; unix sockets get mode 0600
  and a live-socket steal check.
- `--token` gates writes; with no token configured the mutating
  endpoint returns `WRITE_DISABLED` (403) — read-only stays open on
  loopback.
- Non-loopback binds require `--token` (or `--insecure-allow-exposed`
  override): a refuse-by-default posture.
- Token comparison is constant-time (`subtle.ConstantTimeCompare`);
  tokens are never echoed in logs or error bodies.
- POST bodies are capped at 1 MiB.

## Alternatives
- mTLS/JWT — deferred: disproportionate for a local-first daemon;
  reverse-proxy deployments can add their own layer.
- Fail-closed read endpoints — rejected: introspection is the API's
  purpose on loopback.

## Consequences
- (+) Misconfiguration fails loudly at bind time, not at attack time.
- (−) Single shared token — no per-client revocation yet.

## Review when
Multi-client or remote deployments become a supported target.

## Addendum (rotation + scope decisions)
- `--token`/`--token-file` are repeatable → old+new coexist during a
  rotation window; no insecure gap. All candidates are compared in
  constant time per entry.
- Scopes/mTLS/per-client tokens were evaluated and rejected for the
  localhost+LAN design point: they add a real key-management surface
  for a daemon whose threat model is "local + optional LAN". Reverse
  proxy deployments should terminate auth at the proxy instead.
