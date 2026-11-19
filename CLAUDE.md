# db-mcp-http

Generic read-only MySQL MCP server. Setup, env vars, deploy and connect steps: `README.md`.

## Boundary: engine vs guide pack

The Go code is domain-agnostic. Every business fact (system name, tables, statuses, formulas, answer language, number format) belongs in a guide pack (`GUIDE_DIR`), never in `server/`, `mcpb/`, `.env.example` or this repo's history. `guide.example/` is the only pack committed here: a template with generic rules and `<...>` placeholders. `/guide/` is git-ignored on purpose; real packs live in their own private repos.

Check before committing: no client/company names, client table names or client business terms in code, tests or docs. Tests use generic names (`users`, `orders`, `api_logs`).

## Guide pack rules

- `instructions.md` → MCP server instructions. Stay under 4096 chars (Claude Code truncates there, and a cut text looks complete). Its job is to make the model call `get_guide` first; the guide itself goes only through `get_guide`.
- `reminder.md` → appended to every `query`/`list_tables`/`describe_table` result, because server instructions fade in long chats.
- `NN-*.md` → joined in filename order. Section headings are `## N. Title`; `TestGuideSectionsInOrder` checks sections 1..7 in `guide.example`, so adding a template section means bumping that loop.
- The environment comes from `DB_ENV` (prefixed to instructions and guide at startup). Packs write env-specific rules as conditions ("if the database environment is dev: ...").
- Guide edits need a restart, never a rebuild.

## Safety layers (`server/guard.go`, `server/main.go`)

`checkReadOnly` is the first line of defense only. The real guarantees are a `SELECT`-only DB user plus every query running inside `START TRANSACTION READ ONLY` … `ROLLBACK`. Keep all three.

- `secretColumn` regex: queries naming a match are refused, and `redact` masks matching columns plus bcrypt hashes in results (covers `SELECT *`). String literals are stripped first so `LIKE '%token%'` still passes.
- `secretTables`: MySQL system schemas in code, app tables from `BLOCKED_TABLES` (required, may be empty).
- Any guard change gets cases in `guard_test.go` on both sides: what must be blocked and what must still pass.

## Query execution

- One DB connection per query, no idle pool: pooled conns die silently when the SSH tunnel reconnects (`ponytail:` comment marks the upgrade path).
- `sql_select_limit = countRows+1` (1000) caps rows DB-side; `maxRows` (200) are returned. The two truncation messages differ: over 1000 = result incomplete, tell the model to aggregate.
- 35 s context timeout wraps the 30 s `max_execution_time` because MariaDB ignores the latter.
- `cell()` returns BIGINT/DECIMAL/dates as strings (no float precision loss) and binary as `<binary N bytes>`.
- MySQL error codes are stripped from messages; the model sees only the text.

## HTTP

Stateless Streamable HTTP, JSON responses, `POST /mcp` only, static bearer token compared in constant time. SDK localhost protection is off because it 403s behind a reverse proxy; the token already blocks DNS rebinding. claude.ai connectors need OAuth: add it before exposing the server publicly.

## Adding or renaming a tool

Touch all of: the `tools` list in `newServer`, the `switch` in `call`, `mcpb/manifest.json` `tools` (display only, names must match), the "4 tools" mentions in `README.md`. `mcpb/server/index.js` is a dumb stdio↔HTTP bridge and needs no change.

## Style

Single `main` package, stdlib first, no abstractions with one implementation. Deliberate shortcuts carry a `ponytail:` comment naming the ceiling and the upgrade path; keep that convention.

## Verify

```bash
go vet ./server && go test ./...
```

`TestMain` loads `../guide.example`, so a broken template fails the suite.
