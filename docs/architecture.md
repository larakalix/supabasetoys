# Architecture and safety contracts

The Go module contains `pkg/engine`, `cmd/supabase-toys` (standalone CLI), and the Wails desktop shell (`desktop_app.go` and `desktop_main.go`). The Vue frontend invokes typed requests; it cannot execute arbitrary shell commands. Both clients use the same engine for registration, preflight, repairs, lifecycle, connections, logs, and diagnostics. The desktop build tag keeps native GUI dependencies out of standalone CLI compilation.

## Ownership and persistence

Registration canonicalizes the project folder and records its current project ID. Standard containers are identified by `com.supabase.cli.project`; workdir labels, when present, must match the registered folder. Registering a folder is an explicit association with its ID, including legacy containers without workdir labels. Unregistered inventory is displayed separately and cannot receive lifecycle commands.

Experimental stacks require explicit full IDs from `stack list`; the adapter verifies the project root and Docker runtime against the CLI registry. Containers use the upstream `com.supabase.stack` label. One-off containers are omitted. Stack-root labels identify CLI state directories, not project folders.

The registry persists paths, names, identities, adapter selections, and executable/endpoint preferences. It never stores keys or connection secrets. Config files remain in their existing project locations; runtime data remains owned by Supabase.

All mutations acquire one application-data file lock shared by desktop and CLI processes. Registry and config updates use synced temporary files and atomic replacement. Config/environment writes preserve existing file permissions; newly created files and backup files are private on Unix. Backups sit beside the original with `.toys-<timestamp>.bak` suffixes.

## Project-specific operations

Docker calls use an explicitly resolved local Unix socket or Windows named pipe. TCP/SSH endpoints are refused, including loopback TCP. Lifecycle operations pin the initial endpoint in Settings. The selected endpoint is reused throughout an operation; inherited Docker and project/port overrides are removed.

Supabase subprocesses receive an explicit working directory and adapter mode. Standard stop includes the exact `--project-id`; stack commands include the exact `--stack-id`. No mutation uses `--all`, `--no-backup`, reset, destroy, or volume deletion. Supabase telemetry is disabled for Toys-invoked commands with the upstream `SUPA_TELEMETRY=off` override; users' global consent setting is unchanged.

Missing/unreadable configs, changed identities, duplicate standard IDs, container workdir mismatches, unsupported CLI releases, and unavailable Docker block lifecycle mutations. A failed or cancelled subprocess triggers a fresh inventory read and reports partial state. Cancelling a command does not assume its detached services stopped and does not undo an operation already committed.

Standard identity repair is restricted to verified unused IDs: both old and proposed identities must have no known containers, Supabase-named volumes, or networks. Existing-data migrations are deliberately outside v1.

## Port and environment repairs

The versioned port catalog covers API, database, shadow database, pooler, Studio, mail UI/SMTP/POP3, analytics/vector, and Edge inspector bindings. Disabled optional services stay disabled. Defaults are considered when applicable; unsupported dynamic `env(...)` port expressions produce a diagnostic instead of being guessed.

Config patches retain comments and unrelated source text, then compare the parsed result against the exact intended semantic change. Ambiguous multiline/inline/dotted-key syntax is refused when it cannot be preserved safely; it is never rewritten speculatively.

Port reservations include registered stopped projects, all inspected Docker host bindings, and host TCP listeners. Working target ports are preserved. Conflicts receive the lowest available port from 20000–32767, avoiding every existing target allocation. The written config is the persistent allocation, so direct Supabase CLI starts and Toys share it.

Repair previews include a config hash and exact port changes. Apply regenerates the proposal under the mutation lock; modified configs, tampered previews, or changed availability cause rejection. Ports are checked again just before start. Supabase's own bind errors remain the final guard against external races.

Environment writes require explicit user action. Mappings are checked against live runtime values, validated variable names, browser-public prefixes, secret key prefixes, credential-bearing URLs, and service-role JWT payloads. File paths must stay inside the project and cannot target symlinks. A stale file or changed runtime values requires a new preview. Existing unrelated variables and comments remain intact.

## Observability

Stats use only full IDs of selected running containers. Empty inventory never falls back to unfiltered stats. CPU can exceed 100%; memory accepts decimal and binary Docker units. Invalid/missing measurements remain null. Desktop sampling is serialized, occurs every five seconds while visible, and never stacks overlapping calls.

Resource pressure is a labeled heuristic (200% CPU or 4 GiB RAM), not a host-capacity measurement. Stopped, missing, unhealthy, and unavailable states remain distinct.

Logs are redacted for common token/key/password assignments, JWTs, Supabase keys, and credential-bearing URLs. Reports remove project/workdir paths and recursively sanitize string values while preserving valid JSON. Application logs can contain arbitrary personal text; the UI asks users to review selected logs before sharing.

## Upstream references

- [Supabase CLI configuration](https://supabase.com/docs/guides/local-development/cli/config)
- [Experimental multi-project stacks](https://supabase.com/docs/guides/local-development/running-multiple-local-projects)
- [Versioned telemetry contract](https://github.com/supabase/cli/blob/v2.119.0/docs/telemetry.md)
- [Versioned experimental container labels](https://github.com/supabase/cli/blob/v2.119.0/packages/stack/src/runtime/Container.ts)
- [Wails desktop development](https://v2.wails.io/docs/guides/application-development/)


## Go migration compatibility

Application-data locations, JSON field names, standard registration hashes, and lock filenames remain compatible with the original implementation. Existing registrations and Supabase-owned volumes require no migration. All subprocess work accepts a context, has a deadline, drains bounded buffers, and limits inherited-pipe waits. Cancelled mutations use a separate bounded inventory read for reconciliation. No request exposes an arbitrary command runner.

## Explicit port editing

`ports.go` owns manual allocation policy. `PlanPortChanges` is a pure function with an injected availability check; it validates the complete resulting allocation and leaves source configuration untouched. Both automatic repair and manual editing share project/container reservations. `PreviewPorts` uses the existing readiness checks and returns a hash-bound manual preview. `ApplyRepair` verifies the mode, original values, supported keys, runtime ownership, current availability, and file hash under the shared mutation lock. It checks manual allocations again before writing, creates a backup, and restarts only the selected project when explicitly approved.

The desktop `PortsDialog` owns form and review state. `ProjectOverview` displays configured ports independently from discovered runtime service bindings. The CLI `ports` adapter parses flags and serializes shared engine contracts. There are no Wails or Vue dependencies in allocation policy. Graphify remains a contributor-only navigation aid; generated graphs and local hook settings are ignored.
