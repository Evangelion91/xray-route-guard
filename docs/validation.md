# Validation record — 5 November 2026

Version 0.1.0 is a **public preview**, not acceptance on a physical OpenWrt router.
Tests ran in Ubuntu 22.04 under WSL2 using Go 1.27.3. All network fixtures were
local and synthetic; no subscriber key or provider server was used.

| Check | Result and scope |
|---|---|
| Static analysis | `go vet ./...` passed |
| Race detector and unit/integration tests | Passed; 74.9% statement coverage of `internal/guard`. The CLI is not included in that percentage |
| Subscription fuzzing | Two workers, 10-second target, 109,095 executions; no crash in that bounded run |
| Official Xray 25.1.1 and 26.10.31 | Actual isolated Xray SOCKS probe passed twice and its child process released the port |
| VLESS transport matrix, both cores | `run -test` accepted TCP/TLS, WS/TLS, gRPC/TLS, XHTTP/TLS, TCP/REALITY and XHTTP/REALITY. Configuration acceptance is not a live remote handshake test |
| Activation and recovery | Temporary real files, child validation process, local SOCKS/HTTP fixtures: commit, failed post-check rollback, interrupted transition recovery and external-edit rejection |
| Failure paths | Failed rollback preserves the journal; rollback after journal unlink is recoverable; occupied probe ports and existing locks are respected |
| Import policy | New label filters apply to old cache; unknown parameters, unsupported nodes and direct/proxy-chain entries are rejected |
| Credential handling | Owner-only regular files; symlinks, FIFOs and a different owner's files are rejected. Logs do not include subscription tokens or endpoints |
| Release packages | Static Linux amd64, arm64 and ARMv7 builds; SHA-256 checksums verified. Only amd64 was executed |
| Installer | Staged filesystem test: private directory/example modes, executable version, no active config or autostart created. This is not a live procd acceptance test |

CI repeats static analysis, race tests, fuzzing, three architecture builds and
the checksum-pinned real-core checks. Inspect the actual workflow run for its
current result.

## Not established by this record

- Physical router performance, flash endurance or behavior during actual power loss.
- procd integration on a particular firmware, package-manager integration, or firmware flashing.
- Durev VPN interoperability or provider endorsement.
- Live TLS/REALITY sessions across every advertised transport.
- Long-duration outage, throughput, connection migration or uninterrupted connectivity.

The earlier Lua monitor's use on a GL.iNet router motivated this implementation;
that history is not evidence that the new binary passed those hardware checks.
Start with a backed-up, isolated deployment and test the configured restart
command before using automatic switching on a working network.
