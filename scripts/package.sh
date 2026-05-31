#!/usr/bin/env bash
# Bundle RETROX into a portable .zip ready to ship to another Mac:
#
#   ./scripts/package.sh
#   → dist/retrox-portable-YYYYMMDD.zip
#
# What goes in: the Go binary, run.sh / RETROX.command launchers, the
# emulators dir, the data dir (DB + OpenVGDB), and roms (subdirs only
# by default — pass `--with-roms` to bundle ROM files too).
#
# What's excluded: source code, npm bits, image cache (regenerable).

set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

WITH_ROMS=0
if [[ "${1:-}" == "--with-roms" ]]; then
  WITH_ROMS=1
fi

DIST="dist"
NAME="retrox-portable-$(date +%Y%m%d)"
ZIP="$DIST/$NAME.zip"
mkdir -p "$DIST"
rm -f "$ZIP"

# Sanity: the binary must exist.
if [[ ! -x retrox ]]; then
  echo "✗ ./retrox manquant — lance d'abord 'make build'"
  exit 1
fi

# Build the file list. Always include binary, launchers, emulators, and
# the OpenVGDB SQLite. Conditionally include ROMs.
echo "→ Archive : $ZIP"
zip -ry "$ZIP" \
  retrox \
  run.sh RETROX.command \
  emulators \
  data \
  $([[ $WITH_ROMS -eq 1 ]] && echo roms || echo) \
  -x 'data/imgcache/*' \
  -x 'data/retrox.db' \
  -x '*/.DS_Store' \
  -x '.DS_Store' \
  >/dev/null

# Always include the roms/ tree as empty subdirs so the user has the
# right structure even without --with-roms.
if [[ $WITH_ROMS -eq 0 ]]; then
  TMP="$(mktemp -d)"
  mkdir -p "$TMP/roms"
  for d in roms/*/; do
    mkdir -p "$TMP/$d"
    # Add a sentinel so empty dirs survive the zip.
    : > "$TMP/$d/.keep"
  done
  (cd "$TMP" && zip -ry "$ROOT/$ZIP" roms >/dev/null)
  rm -rf "$TMP"
fi

SIZE=$(du -sh "$ZIP" | cut -f1)
echo "✓ Bundle prêt : $ZIP ($SIZE)"
echo
echo "Pour le déployer :"
echo "  1. Copie $ZIP sur l'autre Mac"
echo "  2. Décompresse-le où tu veux"
echo "  3. Double-clique RETROX.command"
