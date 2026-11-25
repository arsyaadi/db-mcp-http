#!/bin/sh
# Install this repo's Claude Code skills user-level (~/.claude/skills), usable from any folder.
#   From a clone:  ./install-skills.sh
#   Without one:   curl -fsSL https://raw.githubusercontent.com/arsyaadi/db-mcp-http/main/install-skills.sh | sh
# Re-run to update. REF=<branch|tag> picks the version when downloading; CLAUDE_SKILLS_DIR overrides the target.
set -eu
dest=${CLAUDE_SKILLS_DIR:-$HOME/.claude/skills}
case $0 in
*install-skills.sh) src=$(dirname "$0")/.claude/skills ;;
*)  # piped into sh: no clone, download the repo
  tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
  curl -fsSL "https://codeload.github.com/arsyaadi/db-mcp-http/tar.gz/${REF:-main}" | tar -xz -C "$tmp" --strip-components=1
  src=$tmp/.claude/skills ;;
esac
mkdir -p "$dest"
for s in "$src"/*/; do
  n=$(basename "$s")
  rm -rf "${dest:?}/$n"            # also replaces an old symlink install
  cp -RL "$s" "$dest/$n"           # -L: template/ symlink becomes real files
  echo "installed $n -> $dest/$n"
done
