# Validation record

Go migration verified on 2026-10-05 on macOS with a local OrbStack Docker endpoint. Existing unrelated Supabase projects were left running. Acceptance projects used unique identities, disposable folders, private test tables, and an isolated Toys registry. Cleanup stops only test projects and preserves their volumes.

## Completed locally

| Check | Result |
| --- | --- |
| Go engine/CLI safety suite | 38 passing tests with race detection |
| Vue workflow suite | 10 passing tests |
| Offline archive/installer checks | 2 passing tests; valid checksum installs, invalid checksum preserves existing binary |
| Go formatting, go vet, module checksums | Pass |
| TypeScript/Vue typecheck and production frontend build | Pass |
| Native macOS app bundle | Built successfully with Go/Wails 2.15.0; deployment target macOS 13 |
| Local desktop installer | ARM64 DMG packaged successfully; public signing/notarization pending |
| CLI cross-compilation | Windows AMD64 and Linux AMD64 build successfully; execution on those systems pending |
| Supported CLI capability fixtures | Supabase 2.119.0 and 2.118.0 verified against actual executables |
| Live two-project acceptance | Pass on both supported CLI versions |
| Experimental Docker stack adoption | Pass on 2.119.0: full stack identity, scoped services, endpoints, stop and resume |
| Browser interface preview | Visually inspected; actions explicitly disabled outside Wails; no horizontal overflow at 720px |
| Native desktop flow | Add folder, load live state/resources/endpoints, masked connections, preview and explicitly write environment settings, scoped logs, generate redacted diagnostics, targeted start/stop, per-service resource readings, and reload persisted registration verified |

The live two-project test starts A and B together, writes different database markers, verifies separate API endpoints, restarts/stops B while checking A remains operational, starts B again, verifies both database markers, and verifies stable endpoints after reopening engine/CLI state. It exports a diagnostic report and cleans up with data-preserving targeted stops. This exercises API/Auth/Studio/email/database services; optional port handling also has fixture coverage.

The Go suite covers identity conflicts, optional and stopped-project port reservations, stale/tampered repair previews, shared mutation locks, partial startup reconciliation, unsupported dependencies, remote Docker refusal, environment preservation and privileged-key restrictions, diagnostic/log redaction, scoped statistics with unavailable readings, unused-identity repair, and registration-only removal. Socket-sensitive tests run sequentially to avoid treating another test's temporary bind as an external conflict.

## Reproduce

```sh
go test -race ./...
go vet ./...
go mod verify
gofmt -l pkg cmd desktop_main.go desktop_app.go
pnpm typecheck
pnpm test
pnpm build
python3 scripts/test-distribution.py
python3 scripts/check-compatibility.py /absolute/path/to/supabase
TOYS_BINARY="$PWD/build/bin/supabase-toys" SUPABASE_BINARY=/absolute/path/to/supabase python3 scripts/live-acceptance.py
TOYS_BINARY="$PWD/build/bin/supabase-toys" SUPABASE_BINARY=/absolute/path/to/supabase python3 scripts/stack-acceptance.py
pnpm desktop:build
```

Live scripts require local Docker and create test stacks; their printed paths identify retained configurations and data. They never stop unrelated stacks. Run the stack test only with the supported experimental baseline.

## Release gates still requiring external proof

The three-platform CI matrix and Linux Docker acceptance jobs are implemented but have not run on GitHub in this workspace. Windows/Linux native builds, GUI workflows, runtime lifecycle smoke tests, and the PowerShell installer require verification on those operating systems. The archive-layout smoke test runs on all three CI platforms; the Unix installer smoke test runs on macOS/Linux. The Windows job includes an offline PowerShell installer/CLI launch/checksum-rejection test in `scripts/test-install.ps1`.

Public asset downloads, clean-user installation/upgrade checks, macOS signing/notarization, Windows signing, and release publication need a configured GitHub repository and signing secrets. A locally self-signed app bundle is not a signed/notarized public release. See [release preparation](releases.md).

## Port editing and Graphify (2026-10-05)

Manual-port unit and integration fixtures verify single edits, swaps, unchanged ports, invalid/unknown settings, duplicate ports, stopped registrations, host listeners, stale or tampered previews, explicit restart approval, preservation of another project, TOML comments, backups, and status agreement. The Vue suite verifies stopped-project visibility, review before apply, explicit restart approval, conflict errors, and a fresh preview after failed apply.

Native macOS verification changed a stopped disposable project's `db.port` from 58000 to 58100 through the editor. The overview reflected the saved value; `db.shadow_port` stayed 58001, the comment survived, and a timestamped backup was present. The temporary registration was removed after verification.

Graphify 0.9.46 generated the local graph, report, and interactive HTML, with locally clustered communities. Query and reverse-impact navigation verified the port planner's consumers. The Docker JSON fixture and local hook JSON produce no source symbols; dependency references and graph clustering are heuristic. Graphify's Codex Desktop hook intentionally does nothing; repository instructions provide guidance. No LLM labeling was used.

The extended live A/B acceptance passed on Supabase CLI 2.119.0 and 2.118.0: explicit API-port changes on B, correct new runtime endpoint, stable A endpoint and availability, persistent database markers, and persistence of the changed allocation after another stop/start.

A regression test also verifies that an external configuration edit made during the final runtime inventory check is preserved and rejects the apply. Both live test runs cleaned up with targeted stops, preserving their data.
