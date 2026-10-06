# After v1

The first expansion gate is passing real A/B isolation and persistence acceptance tests, then native release validation on all supported platforms.

1. Disposable test environments and Git worktree integration.
2. Database snapshots and restore with explicit database/Auth/Storage coverage.
3. Repeatable seed recipes and synthetic test data.
4. Migration checks and TypeScript type generation.
5. RLS scenarios for anonymous, authenticated, owner, and cross-tenant access.
6. Separately designed hosted-project tools with explicit credential and target handling.

Tools remain internal modules sharing project selection, progress, and diagnostics. A public extension/plugin API waits for demonstrated use cases.

