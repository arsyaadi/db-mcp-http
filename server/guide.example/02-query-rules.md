## 2. Query rules (for you, never shown to the user)

- Big tables: <list tables with ~100k+ rows>. Always filter by period and aggregate; 200 rows shown, 30 s timeout.
- Performance:
  - A filter on a non-indexed column reads the whole table. One such scan is fine; many are not → one query with GROUP BY over months/units, never one query per month, never a correlated subquery over a big table.
  - JOIN big tables only on indexed keys (`Key` = PRI/MUL/UNI in describe_table). A join on a non-indexed column → timeout.
  - ORDER BY / GROUP BY on a big table sorts every matching row; LIMIT does not make it cheaper. Filter first.
  - Date filters as ranges on the raw column (`created_at >= '2026-10-01' AND created_at < '2026-11-01'`), not `MONTH()`/`DATE()` on the column.
  - Unsure about a heavy query? Run EXPLAIN first; `type = ALL` on two big tables in one join → rewrite.
  - Timeout = query too heavy, not "no data". Narrow the period or aggregate further, then retry; never report it as empty.
- One statement only. Avoid `;` and `#` inside string literals (the guard rejects them).
- Money: <unit and type, e.g. whole currency units as BIGINT>. Times: <timezone>.
- Soft deletes: <e.g. always filter deleted_at IS NULL / is_deleted = 0 on which tables>.
- A status code in the DB that this guide does not list → call it "other status", do not guess.
- Grouping or listing by a status/type code → map it to the §4 name in the SELECT (`CASE status WHEN 1 THEN '<name>' ... ELSE 'Other status' END`), so results carry names, not codes.
