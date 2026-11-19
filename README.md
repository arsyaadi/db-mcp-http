# db-mcp-http

Read-only MySQL MCP server over Streamable HTTP, for non-technical users who ask questions about their data in plain language. The server holds no business knowledge: everything domain-specific (flows, status meanings, formulas, answer style) comes from a **guide pack**, a folder of Markdown files read at startup. Same binary, different `.env` + guide pack = a different database/business.

The DB is reached directly, or through a built-in SSH tunnel when `SSH_HOST` is set (key from `SSH_KEY_PATH`). Auth is a static bearer token for now.

Tools (all read-only): `get_guide` (the guide pack), `list_tables`, `describe_table`, `query` (SELECT/SHOW/DESCRIBE/EXPLAIN only, max 200 rows, 30 s timeout, run in a read-only transaction). Credential columns (passwords, PINs, tokens, API keys...) and `BLOCKED_TABLES` are refused in queries and masked in results.

## Requirements

- Go ≥ 1.25 to build (the server it runs on needs the binary plus the guide folder)
- A MySQL user (ideally `SELECT` only)

## Setup

```bash
cp .env.example .env
cp -r guide.example guide   # then fill in your own domain knowledge
openssl rand -hex 32        # paste as MCP_TOKEN in .env
```

Fill `.env`:

| Variable | Required | Meaning |
|---|---|---|
| `HOST`, `PORT` | no | Listen address, default `127.0.0.1:3333`. Use `0.0.0.0` to accept other machines. |
| `MCP_TOKEN` | yes | Shared secret (≥ 16 chars). Clients send `Authorization: Bearer <MCP_TOKEN>`. |
| `SSH_HOST` | no | SSH server that can reach MySQL. Empty = connect to `DB_HOST` directly. |
| `SSH_PORT`, `SSH_USER` | user if `SSH_HOST` set (port default 22) | SSH login. |
| `SSH_KEY_PATH`, `SSH_PASSPHRASE` | key if `SSH_HOST` set, passphrase if set | Private key path (may start with `~/`). |
| `DB_HOST`, `DB_PORT` | no | MySQL as seen from the SSH server (or this machine without SSH), default `127.0.0.1:3306`. |
| `DB_ENV` | yes | `dev` or `production`. Told to Claude at the top of the server instructions and of `get_guide`, so the guide never hardcodes it. |
| `DB_NAME`, `DB_USER`, `DB_PASSWORD` | user + password yes | Database login. |
| `MCP_NAME` | no | Server name shown to clients, default `db-mcp`. |
| `GUIDE_DIR` | no | Guide pack folder, default `guide` (relative to the working directory). |
| `BLOCKED_TABLES` | yes (may be empty) | Comma-separated app tables whose rows are secrets. MySQL system schemas are always blocked. |

`.env` is read from the working directory; variables already set in the environment win. The server refuses to start if a required value is missing and prints which one.

## Guide pack

All domain knowledge lives in the guide folder (`GUIDE_DIR`), read at startup:

- `instructions.md`: MCP server instructions. Keep under 4096 chars (clients truncate) and tell the model to call `get_guide` first.
- `reminder.md`: appended to every query result (instructions fade in long chats).
- `NN-*.md`: sections returned by `get_guide`, joined in filename order (a new section = a new numbered file).

`guide.example/` is a template with the generic answer/query rules filled in and the domain sections left as placeholders. `guide/` is git-ignored: keep each real pack in its own private repo (or anywhere on the server) and point `GUIDE_DIR` at it.

Never write which environment the DB is in: it comes from `DB_ENV`. Write environment-specific rules as conditions ("if the database environment is dev: ...").

Update = edit the files, then restart the service; no rebuild.

## Run

```bash
go build -o db-mcp ./server && ./db-mcp   # -> db-mcp MCP listening on http://127.0.0.1:3333/mcp
```

With SSH, the tunnel opens on the first query and reconnects by itself if it drops.

## Deploy

One process per database: same binary, its own folder with `.env` + guide pack, its own port and systemd unit.

Build for the server on your machine, copy the binary and the guide pack, put `.env` (and the SSH key, if used) next to it:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o db-mcp ./server
scp -r db-mcp guide <server>:/opt/<name>-db-mcp/
```

Keep it running with systemd (`/etc/systemd/system/<name>-db-mcp.service`, then `sudo systemctl enable --now <name>-db-mcp`):

```ini
[Service]
WorkingDirectory=/opt/<name>-db-mcp
ExecStart=/opt/<name>-db-mcp/db-mcp
Restart=always
User=<user>

[Install]
WantedBy=multi-user.target
```

Check it answers (expects a JSON list of 4 tools):

```bash
curl -s http://127.0.0.1:3333/mcp \
  -H "Authorization: Bearer $MCP_TOKEN" \
  -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

Only `POST /mcp` is served; wrong token → 401, other paths → 404.

## Connect

**Claude Code**

```bash
claude mcp add --transport http db-mcp http://127.0.0.1:3333/mcp --header "Authorization: Bearer <MCP_TOKEN>"
```

Or inspect with `npx @modelcontextprotocol/inspector` (transport: Streamable HTTP, add the same header).

**Claude Desktop extension (`mcpb/`)**

Thin stdio→HTTP bridge so users install a `.mcpb` and only fill **Server URL** + **Access Token**. Tools and instructions all come from the server. The bridge runs on the user's laptop, so the URL only has to be reachable from there (office network/VPN is fine).

Per deployment, edit `display_name`, `description` and the tool descriptions in `mcpb/manifest.json` (what users see in Claude Desktop, in their language), then:

```bash
cd mcpb && npm install && npm run pack   # -> mcpb/db-remote.mcpb (not committed)
```

Share the `.mcpb` file; users double-click it in Claude Desktop and enter the URL (e.g. `http://<server>:3333/mcp`) and token.

## Before putting it online

- claude.ai / Claude Desktop custom connectors call the server from Anthropic's cloud and only support OAuth (no custom headers), so the bearer token is local/Claude Code only. Add OAuth before exposing it.
- Serve it behind HTTPS.
- Use a MySQL user with `SELECT` only; grant masked views for PII columns.

## Dev

```bash
go test ./...               # SQL guard, formatting, .env, guide loading and HTTP auth tests (loads ../guide.example)
```
