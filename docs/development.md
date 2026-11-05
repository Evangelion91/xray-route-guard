# Development

Go 1.23 or newer; release artifacts are built with Go 1.27.3. Linux only.
No Go module dependencies beyond the standard library.

```sh
go vet ./...
go test -race -cover ./...
go test ./internal/guard -run '^$' -fuzz FuzzSubscription -fuzztime 10s -parallel 2
XRAY_BINARY=/absolute/path/to/xray go test ./internal/guard -run RealXray -v
sh scripts/release.sh v0.1.0
```

`Engine` operates through `Driver`: health, refresh, isolated probe and apply.
`Backend` owns processes, HTTPS/SOCKS requests, config validation and the durable
activation journal. `ParseSubscription` handles import independently. Add a new
protocol parser by producing a standalone `Node.Outbound`; do not bypass runtime
validation, routing verification or rollback when extending the backend.

Tests cover transient failures, recovery during selection, cache retention,
cooldown, unresolved-journal stops, preservation of unrelated JSON, post-check
rollback, crash recovery, concurrent edits, lock inheritance, occupied probe
ports and optional real-Xray process cleanup. Fault injection uses local mock
services; no test contains provider credentials.

For changes affecting activation, include a failure-path test, not just a success
example. Keep error messages free of subscription URLs, server addresses and
credentials. Send security reports through GitHub's private vulnerability reporting
when available; never attach a live subscription or an unredacted journal.
