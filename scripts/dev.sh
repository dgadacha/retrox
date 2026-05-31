#!/usr/bin/env bash
# scripts/dev.sh — lance le backend Go (:50000) + le frontend rsbuild
# (:50001 avec proxy /api) en parallèle, avec préfixes colorés et arrêt
# propre des deux process trees sur Ctrl-C.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

C_API=$'\033[36m'   # cyan
C_WEB=$'\033[35m'   # magenta
DIM=$'\033[2m'
R=$'\033[0m'

# Première exécution : installe les dépendances front si absentes.
if [ ! -d retrox-web/node_modules ]; then
  printf '%sPremière exécution — installation des dépendances frontend…%s\n' "$DIM" "$R"
  (cd retrox-web && npm install)
fi

printf '%sRETROX dev — api http://localhost:50000  ·  web http://localhost:50001%s\n' "$DIM" "$R"
printf '%sCtrl-C arrête les deux.%s\n\n' "$DIM" "$R"

go run . 2>&1 | awk -v p="${C_API}[api]${R}" '{ printf "%s %s\n", p, $0; fflush() }' &
API_PID=$!

(cd retrox-web && npm run dev) 2>&1 | awk -v p="${C_WEB}[web]${R}" '{ printf "%s %s\n", p, $0; fflush() }' &
WEB_PID=$!

# kill_tree tue récursivement un PID et toute sa descendance — nécessaire
# parce que `npm run dev` empile sh → npm → node (rsbuild), et `pkill -P`
# ne descend qu'à un seul niveau.
kill_tree() {
  local pid=$1 sig=${2:-TERM}
  local child
  for child in $(pgrep -P "$pid" 2>/dev/null); do
    kill_tree "$child" "$sig"
  done
  kill -"$sig" "$pid" 2>/dev/null || true
}

cleanup() {
  trap '' INT TERM   # ignore further signals during shutdown
  printf '\n%sArrêt…%s\n' "$DIM" "$R"
  kill_tree $API_PID TERM
  kill_tree $WEB_PID TERM
  wait 2>/dev/null || true
}
trap cleanup INT TERM

wait
