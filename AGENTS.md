# Supabase Toys contributor guidance

Keep allocation policy in the shared Go engine, independent of desktop and CLI frameworks. Prefer small cohesive functions and inject external operations through the existing runner; add abstractions only for real variation. Desktop and CLI adapters must use the same validation and preview/apply contracts.

Preserve project identity, local data, unrelated settings, and other projects' runtime state. Port changes require a preview, stale-file and availability rechecks, shared mutation locking, and a backup. Running projects restart only after explicit approval. Keep experimental stacks capability-gated.

Add behavior tests for successful changes and rejected changes. Run Go race tests, relevant Vue workflow tests, typecheck, formatting, and vet; socket tests require local bind permission. Live acceptance must use disposable projects and targeted data-preserving stops.

## graphify

This project has a knowledge graph at graphify-out/ for source symbols and cross-file relationships. Run `pnpm graph:report` for community structure and visualization.

When the user types `/graphify`, use the installed graphify skill or instructions before doing anything else.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- Dirty graphify-out/ files are expected after hooks or incremental updates; dirty graph files are not a reason to skip graphify. Only skip graphify if the task is about stale or incorrect graph output, or the user explicitly says not to use it.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `pnpm graph:update` to keep the graph current (AST-only, no API cost).
