# Guide pack slot

The guide pack built into the binary comes from this folder. Copy a pack's served files here, then build:
`instructions.md`, `reminder.md` and `[0-9]*.md` (not `sources.md` or `review.md`: every `.md` here is embedded).

Everything in this folder except this README is git-ignored. Never commit a real pack here.

Or leave it empty and set `GUIDE_DIR` to a pack folder at runtime.
