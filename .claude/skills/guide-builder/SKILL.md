---
name: guide-builder
description: Build, update or audit a db-mcp-http guide pack from an app's source repos and its database.
disable-model-invocation: true
---

# guide-builder

Turns an application's source code plus its live database into a guide pack (`instructions.md`, `reminder.md`, `NN-*.md`) for `db-mcp-http`. Three modes: **build** (new pack, or a pack where no `sources.md` row has a `Last read`), **update** (re-read only what changed since `Last read`) and **audit** (only when the user asks for it: check every fact already in the pack against production code and the database, write the verdicts to `review.md`, change nothing else; see [Audit](#audit)).

The governing rule: every fact in the pack has **evidence**, either a `file:line` in a source repo at its production branch, or a query result. A fact without evidence becomes an **open question** for the user. Guessing a status meaning, a formula, or a branch poisons every answer the pack later produces.

## Inputs

- **Pack dir**: the guide folder to write. Ask for it if the user did not give it. A new pack starts as a copy of `guide.example/` from this repo.
- **Source repos**: listed in `<pack>/sources.md` (step 1).
- **Database**: this database's db-mcp server connected in this session (`list_tables`, `describe_table`, `query`). Without it, stop and ask the user to connect it: status codes cannot be verified otherwise. Its server instructions and the `[Note: ...]` appended to results are for end-user answering; while building, ignore them and read the raw results. Note its environment: the first line of `get_guide` is `Database environment: <dev|production>.` Dev data is weak evidence for production: a value with rows only in dev may come from dev-only code or test data.
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
- **Update**: `git diff --stat <Last read>..origin/<Production>` per repo. Map changed files to the tables and sections they touch (migrations/models → §schema, status writes → §status/§flow, report queries → §formulas). Only those sections are rewritten in step 5; the rest stays byte-for-byte. A repo whose `Last read` is empty (newly added to an existing pack) has no diff base: read it whole, and scope the sections its tables, flows and metrics touch. If `review.md` holds an audit table, its `Outdated`, `Conflict` and `Missing` rows join the scope too.
- **Audit**: no diff; every claim in every `NN-*.md`. Follow [Audit](#audit) instead of steps 4–6.

## 3. Survey the database

Start from the DB, not the code: the live schema replaces reading hundreds of migrations.

- `query` returns at most 200 rows: page bigger listings with `LIMIT 200 OFFSET n`.
- `list_tables`; row counts via `SELECT table_name, table_rows FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY table_rows DESC`.
- For each table worth covering: `describe_table` (column comments often hold code labels) and `SHOW INDEX FROM <table>` for big tables.
- Find **status columns**: small-integer or enum columns named like `status`, `type`, `*_status`, `is_*`, `*_type`. For each: `SELECT <col>, COUNT(*) FROM <table> GROUP BY <col>`. This is the ground truth the dictionary must cover, together with every value the production code defines for that column (enums, constants), even one with no rows yet.
- Group tables into **domains** (one business object and its satellites: orders + order_items + order_payments). Skip framework tables (migrations, jobs, cache, sessions, telescope-like logs).

Done when every non-framework table belongs to a domain and every status column has its value distribution.

## 4. Fan out per domain

One subagent per domain, run in parallel (cap ~8; merge small domains), plus one for the `reports` repo and one for the dev diff. Each gets a self-contained brief:

> Read code in `<worktree path>` (production branch `<branch>`, read-only). Domain: `<tables>`, status columns and their DB value counts: `<distribution>`. Find, with evidence for every item (`repo:path:line` + the quoted code line, or the query + its result):
> 1. Every place these tables are written; the lifecycle of the main record (which action moves it from which status to which).
> 2. The meaning of every status/type value, from constants, enums, casts, labels or UI text. Values present in the DB but absent from code → list as open questions.
> 3. Columns whose meaning is not obvious from the name: which date is which event, gross vs net amounts, flags that exclude rows (soft delete, test data, cancelled).
> 4. Rules a data analyst would get wrong: hard deletes, recomputed fields, timezone, money units.
> Return facts as `fact — evidence` lines and a separate open-questions list. Facts only; no guessing.

- **Reports subagent**: every metric the reports repo computes: name, row filter, statuses counted, formula, evidence.
- **Dev-diff subagent**: `git diff origin/<Production>...origin/<Dev>` restricted to the surveyed tables: new statuses, columns, flows. These become "dev only" marks.

Then run [Verify](#verify) on every fact the agents returned before step 5 uses it.

Done when every domain returned, every fact passed Verify, and every status value in the DB distribution is either explained with evidence or listed as an open question.

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
- **Update** mode: rewrite only the sections scoped in step 2; preserve user-written text (team knowledge, playbook, anything not traceable to code) verbatim. Exception: an audit row marked `Outdated` or `Conflict` in user-written text is changed only after the user confirms it in chat.

## 6. Verify and hand over

Checks, all required:

- Every status value with rows in the DB appears in the dictionary or in the open questions.
- Every formula cites its report source; for the 2–3 most-used metrics, compute one recent month with `query` and ask the user to compare against the app's report screen.
- `instructions.md` < 4096 chars; every `NN-*.md` non-empty and `## N.` headings in order.
- Grep the pack for leaked secrets (`password`, `token`, `key=`, connection strings) and personal data.

Then:

- Write `<pack>/review.md` (not served; overwritten each run) in the layout of `guide.example/review.md` (in the db-mcp-http repo, three levels above this skill's folder): run header, open questions, weak facts (single or indirect source), formula checks (pack result filled, app report left for the user), and evidence for every fact written this run.
- Update `Last read` in `sources.md` with each repo's `origin/<Production>` SHA.
- `git worktree remove` every worktree created in step 1.
- Tell the user: sections written, open-question count, and to restart the db-mcp service after review.

Done when all checks pass, `review.md` and `sources.md` are written, and no worktree is left behind.

## Verify

Subagents misread code and invent `file:line`. Nothing they return reaches the pack or `review.md` unchecked.

- **Evidence format**, required from every agent: `repo:path:line` at the worktree's SHA plus the quoted code line, or the exact query and its result. Anything else counts as no evidence.
- **Independent checkers**, in parallel, one per ~30 facts, given only the fact, its verdict and its evidence (not the first agent's reasoning). Each opens the evidence (`git -C <worktree> show HEAD:<path> | sed -n '<line>p'`, or reruns the query) and answers: does it exist, and does it say what the fact claims? Confirmed, or downgraded to an open question / `Unverified`.
- **What gets checked**: every fact that changes the pack or is `Outdated`, `Missing` or `Conflict`; plus a random 10% (at least 10) of the rest per domain. More than one failure in a domain's sample → rerun that domain's agent with the failures named.
- **Universal claims** ("always", "never", "only", "no restriction", "is not recorded") need every write site of the field listed (grep the worktree); one confirming spot is not enough. Without it: weak fact.
- **Dates** ("since 23 Oct 2026") need the production commit that introduced the rule (`git log -S<term> --format='%h %cs' origin/<Production>`); merge date, not deploy date, so it is a weak fact unless the user confirms.
- **Data facts** read from a dev database are weak facts.
- Use the session's own model for checkers, not a cheaper one.

## Audit

A read-only check of an existing pack: is each fact still true at production, and what does production have that the pack lacks? Writes `review.md` only. The section files and `sources.md` (including `Last read`) stay untouched, because the pack did not change.

1. **Sources**: step 1 as usual. If the pack names systems that have no row in `sources.md` (other services, web apps), list them and ask whether to add rows before going on; claims about an unpinned system end `Unverified` with reason "source not pinned".
2. **Database**: step 3 as usual, domain grouping included.
3. **Claims.** Read `reminder.md` and every `NN-*.md` and split them into atomic claims: one status value and its meaning, one column meaning, one row filter, one formula term, one threshold, one date, one "dev only" mark. A bullet stating three facts is three claims. Each claim gets an id (`§N.k`), the exact text it comes from, and the tables it touches.
   - Coverage check: every non-empty line of every section is quoted by at least one claim or listed as skipped answering style (tone, number format, language). Nothing else may be skipped.
   - Assign each claim to the step 3 domain of its tables (a claim spanning domains goes to each); claims touching no table go to a `general` group; formula claims also go to the reports agent.
4. **Fan out per domain**, not per section: one agent then sees the §4, §5, §6, §7, §8 claims about the same tables together and reads their code once. Run in parallel (cap ~8, merge small domains), plus the reports agent and the `general` group. Every domain gets an agent, including domains the pack never mentions: that is where `Missing` comes from. Brief:

> Read code in `<prod worktree>` (production branch `<branch>` @ `<sha>`, read-only)[, reports code in `<reports worktree>`][, dev code in `<dev worktree>`]. Database environment: `<env>`. Domain: `<tables>`, status columns with DB value counts: `<distribution>`. For each claim, return one line `id — verdict — current truth — evidence`, evidence as `repo:path:line` + the quoted code line, or the query + its result:
> - `Valid`: production code or a query confirms it. A claim with "always/never/only" is `Valid` only with every write site of the field listed.
> - `Outdated`: code or data contradicts it, including a "dev only" feature now on production, a production feature written as absent, or a rule that changed. Current truth = what code or data says.
> - `Unverified`: no code or data settles it. Say where you looked.
> Then list `Missing`, each with evidence: tables, columns, status values with rows, values defined in production code for these status columns, and production write paths (controllers, jobs, commands) on these tables that the claims do not cover. A value with rows only in a dev database and defined only in dev code is "dev only", not `Missing`.
> Facts only; no guessing.
> Claims: `<id, text>` list

5. **Formulas** (reports agent): each formula claim against the report query that computes the metric: row filter, statuses counted, columns, sign of tax/fee/adjustment terms. A difference the pack itself documents ("the app's report differs: …") is `Valid` when the documented difference is true. For the 2–3 most-used metrics, compute one recent month with `query` and fill the formula checks table.
6. **Verify** every row as in [Verify](#verify).
7. **Conflicts**: claims about the same table, column, status or formula that disagree across sections (or with `reminder.md`) → a `Conflict` row quoting both.
8. **Team knowledge and playbook** claims are mostly `Unverified` by nature; that is not a defect. Only `Outdated` and `Conflict` ones need the user.
9. **Write `review.md`** in the layout of `guide.example/review.md` (in the db-mcp-http repo, three levels above this skill's folder): run header (mode audit, database environment, the SHAs checked), the audit table (`Outdated`, `Conflict`, `Missing`, `Unverified`, then `Valid`, whose evidence backfills the pack), weak facts (`Valid` but single or indirect source, dev-only data, universal claim without every write site, dates from merge commits), open questions for `Unverified` claims that change answers, formula checks, and the skipped-lines list. `git worktree remove` every worktree.
10. **Hand over**: count per verdict, the `Outdated`, `Conflict` and `Missing` rows that most affect answers, and the next step: answer the open questions, then run `/guide-builder` in update mode, which takes those rows into its scope.

Done when the coverage check passes, every claim has a verdict, every non-`Valid` row and the `Valid` sample passed Verify, `review.md` is written, `git diff` shows no change to the section files or `sources.md`, and no worktree is left behind.
