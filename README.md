# RETROX

Lanceur de jeux rétro auto-hébergé. Un seul binaire Go qui sert une SPA React
embarquée, scanne tes ROMs, récupère leurs jaquettes hors-ligne et lance le bon
émulateur d'un clic. Inspiré par l'archi de [Notflix](#) (single binary +
React embedded), look-and-feel à la Steam moderne.

## Démarrage rapide (deux minutes)

Pré-requis : macOS + [Homebrew](https://brew.sh/).

```bash
# 1. Cloner et compiler
git clone https://github.com/dgadacha/retrox && cd retrox
brew install --cask retroarch dolphin pcsx2 ppsspp
make build

# 2. Lancer
./run.sh    # ou double-clic RETROX.command depuis le Finder
```

Ouvre `http://localhost:50000` → **Réglages → Source de métadonnées →
Télécharger** (récupère OpenVGDB, ~9 MB) → pose tes ROMs dans
`./roms/<plateforme>/` → **Lancer une analyse**.

## Ce que ça fait

- **Scan ROMs** : détection du système par extension + nom de dossier, normalise
  les headers (iNES 16B, SMC 512B, byteswap N64 v64/n64) pour matcher les hashes
  No-Intro.
- **Métadonnées hors-ligne** : [OpenVGDB](https://github.com/OpenVGDB/OpenVGDB)
  (SQLite local, 51 742 ROMs) + [libretro-thumbnails](https://thumbnails.libretro.com)
  en fallback. Aucun compte, aucune clé API.
- **Téléchargement de ROMs** : colle une URL http(s) directe, le fichier
  atterrit dans la plateforme appropriée, rescan automatique.
- **Émulateurs** : RetroArch + cœur libretro par défaut (catalogue intégré pour
  ~25 systèmes), surcharge possible par plateforme avec placeholder `{rom}` et
  `{core}`. Dolphin, PCSX2, PPSSPP en standalone si installés.
- **Bibliothèque Steam-style** : sidebar gauche (plateformes + favoris +
  récents), grille de capsules ou liste dense, recherche + tri.
- **Favoris + historique de jeu** persistés localement.

## Plateformes couvertes

Nintendo (NES, SNES, N64, GB/GBC/GBA, NDS, GameCube, Wii), Sega (Master System,
Mega Drive, Game Gear, 32X, Saturn, Dreamcast), Sony (PSX, PS2, PSP), NEC
(PC Engine), SNK (Neo Geo, Neo Geo Pocket), Atari (2600, 7800, Lynx), Bandai
WonderSwan, Arcade (MAME).

OpenVGDB couvre la plupart pour les métadonnées texte ; les disc-systems lourds
(PS2, Dreamcast, Wii…) tombent sur libretro-thumbnails pour la jaquette
uniquement.

## Bundle portable

Tu peux empaqueter RETROX en un seul `.zip` qui inclut le binaire, les
émulateurs (.app + cœurs), les données et la base OpenVGDB :

```bash
./scripts/package.sh                 # sans les ROMs (~370 MB)
./scripts/package.sh --with-roms     # avec les ROMs (taille variable)
```

Sur un autre Mac : décompresser → double-clic `RETROX.command` → ça démarre.
Premier lancement : si Gatekeeper bloque RetroArch, clic-droit dessus →
« Ouvrir ». RetroArch est compilé Intel, Apple Silicon a besoin de Rosetta
(`softwareupdate --install-rosetta --agree-to-license`).

## Architecture

```
retrox/
├── retrox                  # binaire Go (~20 MB, SPA embarquée)
├── run.sh / RETROX.command # wrappers de lancement
├── main.go                 # entrypoint + CORS + static
├── internal/
│   ├── core/               # App wiring + Config + env vars
│   ├── platforms/          # catalogue des systèmes (extensions, cores, OpenVGDB id…)
│   ├── scanner/            # walk + hash (header-aware) + match
│   ├── openvgdb/           # SQLite reader + downloader
│   ├── libretrothumbs/     # client CDN libretro
│   ├── metadata/           # façade OpenVGDB + libretro
│   ├── emulator/           # détection .app + cœurs, spawn detached
│   ├── download/           # queue de téléchargement ROM
│   ├── database/           # GORM + SQLite (db, models)
│   └── handlers/           # routes Echo /api/v1/*
├── retrox-web/             # SPA React (TS, Tailwind, lucide-react, rsbuild)
└── scripts/package.sh      # création du zip portable
```

Au runtime, tout est résolu en relatif au binaire :

```
data/              # SQLite RETROX + openvgdb.sqlite + cache images
roms/              # tes ROMs (sous-dossiers par plateforme)
emulators/
├── RetroArch.app  # bundles .app
├── Dolphin.app
├── PCSX2.app
├── PPSSPPSDL.app
└── cores/         # cœurs libretro (.dylib)
```

Ports par défaut : **50000** (API + SPA prod), **50001** (rsbuild dev,
proxy `/api` vers 50000).

## Compilation depuis les sources

```bash
# Première install
brew install go node
cd retrox-web && npm install

# Build complet (frontend → ./web/ puis embed dans le binaire Go)
make build

# Dev avec hot-reload du frontend
./retrox &                       # backend sur :50000
cd retrox-web && npm run dev     # frontend hot-reload sur :50001
```

## Variables d'environnement

| Variable | Défaut | Rôle |
|---|---|---|
| `RETROX_SERVER_PORT` | `50000` | Port d'écoute du serveur Go |
| `RETROX_SERVER_HOST` | `127.0.0.1` | Interface bind |
| `RETROX_DATA_DIR` | `<binDir>/data` | DB, OpenVGDB, cache jaquettes |
| `RETROX_ROM_DIRS` | `<binDir>/roms` | Dossiers ROMs (`:`-séparés) |
| `RETROX_EMULATORS_DIR` | `<binDir>/emulators` | .app bundles + cores libretro |
| `RETROX_RETROARCH_BIN` | auto | Override chemin binaire RetroArch |
| `RETROX_RETROARCH_CORES` | `<emulatorsDir>/cores` | Override dossier cores |
| `RETROX_OPENVGDB_PATH` | `<dataDir>/openvgdb.sqlite` | Override chemin SQLite |

L'UI Réglages persiste les overrides dans la base, qui ont priorité sur l'env
au boot suivant. Une variable à blanc dans la DB tombe sur l'env (puis sur le
défaut).

## Stack

- **Backend** : Go 1.21+, [Echo v4](https://echo.labstack.com),
  [GORM](https://gorm.io) + SQLite, `//go:embed all:web`
- **Frontend** : React 19, TypeScript, [rsbuild](https://rsbuild.dev),
  Tailwind 3, [@tanstack/react-query](https://tanstack.com/query), `react-router-dom` 7,
  [lucide-react](https://lucide.dev), `clsx`
- **Métadonnées** : OpenVGDB (SQLite local), libretro-thumbnails (CDN public)

## Licence et crédits

RETROX est un projet personnel. Les métadonnées proviennent d'OpenVGDB
(communauté OpenEmu, données No-Intro/Redump) et de libretro-thumbnails
(projet libretro). Les .app embarquées dans le bundle portable restent sous
leurs licences respectives (RetroArch GPL, Dolphin GPL, PCSX2 GPL, PPSSPP GPL).

Aucune ROM commerciale n'est distribuée avec ce projet. À toi de fournir tes
propres dumps de cartouches ou des ROMs homebrew/public domain.
