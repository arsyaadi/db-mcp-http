# Sources

<!--
Which code is production for this pack. Read and written by the guide-builder skill; the MCP server never serves this file.

One row per source repo:
- Repo: short name.
- Path: repo folder, relative to this pack folder (e.g. ../../app-api when the pack is <repo>/guide/).
- Role: app (business rules, statuses, flows) | reports (official report formulas) | other.
- Production: the branch deployed to production, as named on origin. Check it yourself, do not assume main/master:
    git -C <path> symbolic-ref refs/remotes/origin/HEAD        # remote default, often but not always production
    git -C <path> log -1 --format='%h %cs' origin/<branch>     # recent commit = alive branch
- Dev: branch with features not yet on production (used for "dev only" marks). Empty if none.
- Last read: origin/<Production> SHA the pack was last built from. Leave empty for a first build; the skill fills it.

Example:
| app-api     | ../../app-api     | app     | production | dev | 1a2b3c4d |
| app-reports | ../../app-reports | reports | main       | dev |          |
-->

| Repo | Path | Role | Production | Dev | Last read |
|---|---|---|---|---|---|
