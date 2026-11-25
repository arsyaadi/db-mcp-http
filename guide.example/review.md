# Review

<!--
What the last guide-builder run could not settle, and the evidence behind what it wrote. Written by the skill and overwritten on every run; the MCP server never serves this file.

How to use it:
1. Answer each open question: reply to the skill in chat, or write the answer into the section file yourself and delete the question here.
2. Spot-check weak facts against the app or a teammate.
3. Evidence is for review only: open the file:line or rerun the query when a fact in the pack looks wrong.
4. After an audit: confirm each Outdated, Conflict and Missing row, then run the skill in update mode, which takes them into its scope.
Then restart the db-mcp service so the reviewed pack is served.
-->

Run: <date> · Mode: <build | update | audit> · Sources: <repo@sha, ...>

## Audit

<!-- Audit mode only: every claim in the pack checked against production code and the database. Verdict: Outdated | Conflict | Missing | Unverified | Valid, in that order. Below the table: the lines skipped as answering style. -->

| Section | Claim | Verdict | Current truth | Evidence |
|---|---|---|---|---|

## Open questions

<!-- Facts the pack needs but no code or data settles. One line each: section, question, what was found so far. -->

| Section | Question | Found so far |
|---|---|---|

## Weak facts

<!-- Written into the pack but resting on a single or indirect source (UI label only, comment only, one old commit). -->

| Section | Fact | Why weak |
|---|---|---|

## Formula checks

<!-- Top metrics computed with `query` for one recent month. Compare with the app's report screen; a mismatch = the formula is wrong. -->

| Metric | Period | Pack result | App report | Match |
|---|---|---|---|---|

## Evidence

<!-- Every fact written this run: section, fact, file:line at the production SHA or the query that shows it. -->

| Section | Fact | Evidence |
|---|---|---|
