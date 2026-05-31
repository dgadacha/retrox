#!/usr/bin/env bash
# Fetch console logos from Wikimedia Commons (free / CC / fair-use as used
# by every retro launcher in the wild — EmulationStation, RetroArch, ARES).
# Writes one file per platform id to retrox-web/public/consoles/.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/retrox-web/public/consoles"
mkdir -p "$OUT"

UA='RETROX/0.1 (+https://github.com/dgadacha/retrox)'

# Each line: <platform_id>|<wikimedia search query>
PLATFORMS=$(cat <<'EOF'
nes|Nintendo Entertainment System logo
snes|Super Nintendo Entertainment System logo
n64|Nintendo 64 logo
gb|Game Boy logo
gbc|Game Boy Color logo
gba|Game Boy Advance logo
nds|Nintendo DS logo
gamecube|Nintendo GameCube logo
wii|Nintendo Wii logo
mastersystem|Sega Master System logo
megadrive|Sega Mega Drive logo
gamegear|Sega Game Gear logo
sega32x|Sega 32X logo
saturn|Sega Saturn logo
dreamcast|Sega Dreamcast logo
psx|PlayStation logo
ps2|PlayStation 2 logo
psp|PlayStation Portable logo
pcengine|TurboGrafx 16 logo
neogeo|Neo Geo logo
ngp|Neo Geo Pocket logo
atari2600|Atari 2600 logo
atari7800|Atari 7800 logo
lynx|Atari Lynx logo
wonderswan|Bandai WonderSwan logo
arcade|Arcade logo
EOF
)

# resolve_url <search_query> → prints best matching file URL (SVG preferred, then PNG)
resolve_url() {
  local query="$1"
  curl -s -A "$UA" --get \
    --data-urlencode "action=query" \
    --data-urlencode "list=search" \
    --data-urlencode "srsearch=$query filetype:svg|png" \
    --data-urlencode "srnamespace=6" \
    --data-urlencode "srlimit=8" \
    --data-urlencode "format=json" \
    'https://commons.wikimedia.org/w/api.php' \
    | python3 -c "
import json, sys, re
q = '''$query'''.lower()
data = json.load(sys.stdin)
hits = data.get('query', {}).get('search', [])
def score(t):
    name = t.lower()
    s = 0
    if '.svg' in name: s += 100
    elif '.png' in name: s += 50
    if 'logo' in name: s += 20
    # All keyword tokens (minus stopwords) present?
    tokens = [w for w in re.split(r'\W+', q) if len(w) > 2]
    for w in tokens:
        if w in name: s += 10
    return s
hits.sort(key=lambda h: -score(h['title']))
if hits: print(hits[0]['title'])
"
}

# fetch_actual_url <File:Title>  → prints the upload.wikimedia.org URL
fetch_actual_url() {
  local title="$1"
  curl -s -A "$UA" --get \
    --data-urlencode "action=query" \
    --data-urlencode "titles=$title" \
    --data-urlencode "prop=imageinfo" \
    --data-urlencode "iiprop=url" \
    --data-urlencode "format=json" \
    'https://commons.wikimedia.org/w/api.php' \
    | python3 -c "
import json, sys
pages = json.load(sys.stdin).get('query', {}).get('pages', {})
for p in pages.values():
    info = p.get('imageinfo')
    if info: print(info[0].get('url', '')); break
"
}

OK=0; FAIL=0
while IFS='|' read -r pid query; do
  [ -z "$pid" ] && continue
  printf '  %-14s ' "$pid"
  title=$(resolve_url "$query")
  if [ -z "$title" ]; then
    printf 'aucun résultat\n'; FAIL=$((FAIL+1)); continue
  fi
  url=$(fetch_actual_url "$title")
  if [ -z "$url" ]; then
    printf 'pas d URL\n'; FAIL=$((FAIL+1)); continue
  fi
  ext=$(printf '%s' "${url##*.}" | tr '[:upper:]' '[:lower:]')
  [ "$ext" != "svg" ] && [ "$ext" != "png" ] && ext="png"
  dest="$OUT/${pid}.${ext}"
  if curl -sfL -A "$UA" -o "$dest" "$url"; then
    size=$(wc -c < "$dest")
    printf '✓ %s (%s, %d B)\n' "${title#File:}" "$ext" "$size"
    OK=$((OK+1))
  else
    printf '✗ download\n'; FAIL=$((FAIL+1))
  fi
done <<< "$PLATFORMS"

echo
echo "Résultat : $OK ok · $FAIL échec(s)"
ls -lh "$OUT"
