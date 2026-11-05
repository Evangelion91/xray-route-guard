# Recovery and ownership

Before activation the guard writes a private `pending.json` journal, including
the exact previous file and the candidate hash. It validates the candidate,
atomically replaces the active file, runs the configured restart command, then
checks the managed route twice. Failed restart or health verification restores
the previous bytes and requests another restart.

A crash or power interruption leaves the journal for the next `run`. Recovery
restores it only if the current bytes match that transition's old or new file.
An unrecognized file stops automatic recovery with `manual_recovery_required`.
The guard also stops if rollback itself fails. A subsequent transition must never
overwrite an unresolved journal.

The previous route may already be unavailable: successful rollback means the
old configuration was restored and its restart command succeeded. It does not
mean Internet connectivity has recovered.

## Operator procedure

1. Stop the guard and allow its 45-second termination window to finish.
2. Read the active Xray config and private journal locally. They contain credentials.
3. Choose which file should be authoritative and validate it with your Xray binary.
4. Restart the managed service and test its dedicated SOCKS route.
5. Once resolved, archive/remove the journal yourself before restarting the guard.

Do not blindly delete the journal or copy it over a manually repaired config.
`previous.json` keeps the last successfully replaced configuration for inspection.

The active-config lock is advisory and is not inherited by child processes.
Administrators and other managers must not write the same config while the guard
is enabled. Hash checks detect intervening changes but are not a filesystem
compare-and-swap guarantee against an uncooperative simultaneous writer.

To remove the tool, run `sh uninstall.sh` from its release archive. It stops and
disables only the guard, retaining your Xray file, private settings and recovery
records. It does not remove Xray or revert a successfully selected node.
