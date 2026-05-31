#!/usr/bin/env bash
# Lance RETROX en mode portable. Tout est résolu en relatif à ce script :
# data/, roms/ et emulators/ vivent à côté du binaire. Décompresse le zip,
# double-clique ce fichier (ou lance-le depuis Terminal) et c'est parti.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Ouvre le navigateur sur l'UI quelques secondes après le démarrage.
PORT="${RETROX_SERVER_PORT:-50000}"
(sleep 2 && command -v open >/dev/null && open "http://localhost:${PORT}") &

exec ./retrox
