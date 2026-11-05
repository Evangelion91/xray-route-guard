# Configuration

Supported deployment: a root-owned, single JSON Xray configuration and a service
that can read it with mode `0600`. Includes, JSONC, generated configs owned by
another manager, outbound chains and concurrent writers are outside this scope.
Stop the guard before editing the managed Xray file. One guard instance owns the
whole file, even if different outbound tags are configured.

1. Make a private backup of your working Xray configuration.
2. Identify the exact outbound tag and service restart command.
3. Set up a dedicated local SOCKS health inbound, for example:

```json
{"tag":"guard-health","listen":"127.0.0.1","port":1080,
 "protocol":"socks","settings":{"auth":"noauth","udp":false}}
```

The **first** routing rule must be exactly this shape, using your outbound tag:

```json
{"type":"field","inboundTag":["guard-health"],"outboundTag":"proxy"}
```

The rule must contain no domain, IP or other filters. This ensures a successful
health request actually traversed the managed outbound instead of a direct
route. The guard checks this precondition but never inserts the inbound or rule.
Keep your other inbounds and rules after it. Validate/restart your service using
your normal administration workflow before enabling the monitor.

4. Copy `config.example.json`, then set `active_config`, `xray`, `outbound_tag`,
   `active_socks` and `restart`. `restart` is an argv array, not shell code; use an
   idempotent command that returns after requesting the service restart.
5. Save the provider's HTTPS subscription URL in the file named by
   `subscription_file`. Use an editor, so the token is not saved in shell history:

```sh
umask 077
vi /etc/xray-route-guard/subscription.url
chmod 600 /etc/xray-route-guard/config.json /etc/xray-route-guard/subscription.url
chmod 600 /etc/xray/config.json  # only for a service running as this same owner
```

The active file and state must be owned by the service account running the
guard (normally root). Owner-only checks reject files visible to other users.
Do not point the tool at a configuration requiring group-based access.

6. Pick two small, stable HTTPS endpoints reachable through all eligible nodes.
   Status codes must match exactly; health redirects are not followed. HTTPS
   certificate verification is always enabled. Subscription redirects can follow
   at most five HTTPS locations. A failed direct subscription fetch is retried
   through the active SOCKS proxy; a failed/empty refresh retains the valid cache.
7. Optionally restrict node names with `include` (Go regular expression). Empty
   means all supported entries. Restart the guard after changing configuration;
   the new filter also applies to cached nodes, even if refresh is unavailable.
8. Run `xray-route-guard check` and `xray-route-guard refresh`, then start the
   service. A failed active health check makes `check` exit nonzero.

`state_dir` holds secrets and the recovery journal on persistent storage.
`runtime_dir` should be RAM-backed (the default is `/tmp`): status and temporary
probe configs do not cause a flash write every polling cycle. Both directories
are private. State is rewritten on subscription refresh or a switch, not on
each successful health check.

## Protocol boundary

The link decoder supports UUID-based VLESS, TLS/REALITY, TCP/RAW, WS, gRPC and
XHTTP. Unknown parameters, insecure TLS, unsupported transports and malformed
links are skipped. XHTTP advanced `extra` options are not decoded. Export a JSON
array of standalone Xray outbound objects when the provider needs additional
settings. The installed Xray must accept every candidate (`run -test` checks it).

JSON protocol allowlist: VLESS, VMess, Trojan, Shadowsocks, SOCKS, HTTP, WireGuard.
Direct/blackhole/DNS/loopback nodes and proxy/dialer chains are rejected. JSON
imports preserve protocol settings; use a trusted subscription provider.

## Other Linux systems

Copy the matching binary into a location you manage and run
`xray-route-guard -config /absolute/config.json run` under your service manager.
Use a stop timeout of at least 45 seconds to permit rollback. There is no
automatic systemd installer in this release. Restarting the managed Xray service
may interrupt existing connections; this is not seamless connection migration.
