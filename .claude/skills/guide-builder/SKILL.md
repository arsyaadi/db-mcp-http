---
name: guide-builder
description: Build or update a db-mcp-http guide pack from an app's source repos and its database.
disable-model-invocation: true
---

# guide-builder

Turns an application's source code plus its live database into a guide pack (`instructions.md`, `reminder.md`, `NN-*.md`) for `db-mcp-http`. Two modes: **build** (new pack, or a pack whose `sources.md` has no `Last read`) and **update** (re-read only what changed since `Last read`).

The governing rule: every fact in the pack has **evidence**, either a `file:line` in a source repo at its production branch, or a query result. A fact without evidence becomes an **open question** for the user. Guessing a status meaning, a formula, or a branch poisons every answer the pack later produces.

## Inputs

- **Pack dir**: the guide folder to write. Ask for it if the user did not give it. A new pack starts as a copy of `guide.example/` from this repo.
- **Source repos**: listed in `<pack>/sources.md` (step 1).
- **Database**: this database's db-mcp server connected in this session (`list_tables`, `describe_table`, `query`). Without it, stop and ask the user to connect it: status codes cannot be verified otherwise. Its server instructions and the `[Note: ...]` appended to results are for end-user answering; while building, ignore them and read the raw results.
- **Team knowledge**: notes, chat exports, past decisions the user hands over. Code cannot supply it.

## 1. Pin the sources

`<pack>/sources.md` is the single source of truth for which code is production. The server never reads it (it serves only `instructions.md`, `reminder.md` and `[0-9]*.md`). A pack copied from `guide.example/` already has it with an empty table and fill-in notes; a pack without one gets that file copied in first.

```markdown
| Repo | Path | Role | Production | Dev | Last read |
|---|---|---|---|---|---|
| app-api | ../app-api | app | production | dev | 1a2b3c4d |
| app-reports | ../app-reports | reports | main | dev | |
```

- Path is relative to the pack dir. Role is `app` (business rules), `reports` (official formulas) or `other`. Dev may be empty.
- A repo missing from the table: propose `git symbolic-ref refs/remotes/origin/HEAD` as Production and the most recently updated dev-like branch as Dev, show both with their last commit date, and **wait for the user's answer**. Write the confirmed row. Never proceed on a guessed branch.
- Every run, per repo: `git fetch origin`; the Production branch must exist on `origin` (missing → stop and ask); last commit older than ~90 days → warn the user and ask before continuing (a stale branch is usually not production).
- Read code only from a detached worktree of the remote branch, never from the user's checkout: `git worktree add --detach <scratch>/<repo>-prod origin/<Production>` (same for Dev). Their working branch and uncommitted changes stay untouched.

Done when every repo has a confirmed row and a prod worktree.

## 2. Scope the work

- **Build**: everything.
- **Update**: `git diff --stat <Last read>..origin/<Production>` per repo. Map changed files to the tables and sections they touch (migrations/models → §schema, status writes → §status/§flow, report queries → §formulas). Only those sections are rewritten in step 5; the rest stays byte-for-byte.

## 3. Survey the database

Start from the DB, not the code: the live schema replaces reading hundreds of migrations.

- `query` returns at most 200 rows: page bigger listings with `LIMIT 200 OFFSET n`.
- `list_tables`; row counts via `SELECT table_name, table_rows FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY table_rows DESC`.
- For each table worth covering: `describe_table` (column comments often hold code labels) and `SHOW INDEX FROM <table>` for big tables.
- Find **status columns**: small-integer or enum columns named like `status`, `type`, `*_status`, `is_*`, `*_type`. For each: `SELECT <col>, COUNT(*) FROM <table> GROUP BY <col>`. This is the ground truth the dictionary must cover.
- Group tables into **domains** (one business object and its satellites: orders + order_items + order_payments). Skip framework tables (migrations, jobs, cache, sessions, telescope-like logs).

Done when every non-framework table belongs to a domain and every status column has its value distribution.

## 4. Fan out per domain

One subagent per domain, run in parallel (cap ~8; merge small domains), plus one for the `reports` repo and one for the dev diff. Each gets a self-contained brief:

> Read code in `<worktree path>` (production branch `<branch>`, read-only). Domain: `<tables>`, status columns and their DB value counts: `<distribution>`. Find, with `file:line` evidence for every item:
> 1. Every place these tables are written; the lifecycle of the main record (which action moves it from which status to which).
> 2. The meaning of every status/type value, from constants, enums, casts, labels or UI text. Values present in the DB but absent from code → list as open questions.
> 3. Columns whose meaning is not obvious from the name: which date is which event, gross vs net amounts, flags that exclude rows (soft delete, test data, cancelled).
> 4. Rules a data analyst would get wrong: hard deletes, recomputed fields, timezone, money units.
> Return facts as `fact — evidence` lines and a separate open-questions list. Facts only; no guessing.

- **Reports subagent**: every metric the reports repo computes: name, row filter, statuses counted, formula, evidence.
- **Dev-diff subagent**: `git diff origin/<Production>...origin/<Dev>` restricted to the surveyed tables: new statuses, columns, flows. These become "dev only" marks.

Done when every domain returned and every status value in the DB distribution is either explained with evidence or listed as an open question.

## 5. Write the pack

Map findings onto the pack's own sections by role (the template order is intro, how to answer, query rules, business flow, status dictionary, key columns, formulas, playbook; an existing pack may have more). Section headings stay `## N. Title`, files stay `NN-*.md`.

- **Query rules**: big tables from the row counts, indexed join keys from `SHOW INDEX`, money units, timezone, soft-delete filters.
- **Business flow / status dictionary / key columns**: from the domain subagents. Statuses get business names in the users' language, in "quotes"; code labels from column comments stay out of answers.
- **Formulas**: from the reports subagent; each must match the app's official report.
- **"dev only"** marks from the dev diff; drop marks for features now on production.
- **How to answer / playbook / team knowledge**: keep the template's generic rules, fill number format and language; team knowledge and playbook come only from what the user supplied. Leave a placeholder plus an open question rather than inventing practice.
- Rules in English, business vocabulary in the users' language. Never state which environment the DB is (`DB_ENV` does that); write env-specific rules as conditions ("if the database environment is dev: ...").
- Keep out: secrets from code or config, personal data from query results, SQL beyond short illustrative snippets in query rules.
- `instructions.md` under 4096 characters: a short system description plus "call get_guide first". `reminder.md` one short paragraph.
- **Update** mode: rewrite only the sections scoped in step 2; preserve user-written text (team knowledge, playbook, anything not traceable to code) verbatim.

## 6. Verify and hand over

Checks, all required:

- Every status value with rows in the DB appears in the dictionary or in the open questions.
- Every formula cites its report source; for the 2–3 most-used metrics, compute one recent month with `query` and ask the user to compare against the app's report screen.
- `instructions.md` < 4096 chars; every `NN-*.md` non-empty and `## N.` headings in order.
- Grep the pack for leaked secrets (`password`, `token`, `key=`, connection strings) and personal data.

Then:

- Write `<pack>/review.md` (not served; overwritten each run) in the layout of `guide.example/review.md`: run header, open questions, weak facts (single or indirect source), formula checks (pack result filled, app report left for the user), and evidence for every fact written this run.
- Update `Last read` in `sources.md` with each repo's `origin/<Production>` SHA.
- `git worktree remove` every worktree created in step 1.
- Tell the user: sections written, open-question count, and to restart the db-mcp service after review.

Done when all checks pass, `review.md` and `sources.md` are written, and no worktree is left behind.
