# Contributing

Use the pinned Go toolchain and pnpm version. Preserve existing project configuration and runtime data; avoid global Docker operations.

```sh
pnpm install --frozen-lockfile
gofmt -w pkg cmd desktop_main.go desktop_app.go
go mod verify
go test -race ./...
go vet ./...
pnpm typecheck
pnpm test
pnpm build
pnpm desktop:build
```

Safety changes require failure-case tests that demonstrate project isolation. UI changes require reviewing the affected native flow in addition to frontend checks. Mark fixture/mock results separately from live Docker proof.

For live validation, see `scripts/live-acceptance.py`. It requires absolute `TOYS_BINARY` and `SUPABASE_BINARY` paths, creates uniquely named test stacks, and stops them while preserving data. Never point it at an existing project.

When adding CLI compatibility, inspect real `--help` output and versioned source, expand fixtures, and pass the same live tests. Unsupported versions must remain diagnostic-only until verified.
