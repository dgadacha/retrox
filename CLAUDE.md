# CLAUDE.md — guidance for future Claude sessions on RETROX

## What RETROX is

A self-hosted, Netflix/Steam-style launcher for retro games. One Go binary
embeds the React SPA via `//go:embed all:web` and serves both on a single
port. Designed to live as a "portable folder" you can zip up and drop on
another Mac.

The interesting bits compared to a stock library manager:

- **Two metadata sources fused**: OpenVGDB SQLite for text + cover URLs,
  libretro-thumbnails CDN for missing art. Both account-free, no API keys.
- **Header-aware ROM hashing**: scanner strips iNES (16B) / FDS (16B) /
  SMC copier (512B) / Lynx LNX (64B) / Atari 7800 A78 (128B) headers and
  unswaps N64 v64/n64 byteswapped dumps so hashes line up with No-Intro
  DATs (OpenVGDB's source).
- **Bundled emulators**: the portable bundle ships .app bundles in
  `./emulators/` and libretro cores in `./emulators/cores/`. The launcher
  prefers those over `/Applications/` system installs.

## Repo layout

```
retrox/
├── main.go                  # entrypoint, Echo wiring, CORS, embed FS
├── Makefile                 # `make build` / `run` / `dev` / `tidy`
├── run.sh                   # portable wrapper, sets cwd, opens browser
├── RETROX.command           # macOS double-click launcher → run.sh
├── go.mod / go.sum
├── internal/
│   ├── core/
│   │   ├── app.go           # the App container; provisions OpenVGDB,
│   │   │                      libretro client, metadata.Provider,
│   │   │                      downloader, default profile at boot
│   │   └── config.go        # env vars → Config, binaryDir() helper
│   ├── platforms/           # static catalog: ID, OpenVGDBID,
│   │                          LibretroThumbsName, Core, Standalone,
│   │                          Exts, Aliases; folder-name disambiguation
│   ├── scanner/
│   │   ├── scanner.go       # walk → hash (header-aware) → metadata
│   │   │                      lookup → upsert. Live progress, one scan
│   │   │                      at a time, prunes vanished rows (Missing).
│   │   └── scanner_test.go  # hashFile normalization (10 cases)
│   ├── openvgdb/
│   │   └── openvgdb.go      # SQLite read-only, prepared stmts,
│   │                          Download() fetches + extracts the zip.
│   ├── libretrothumbs/
│   │   └── libretrothumbs.go # URL builder + HEAD check. CDN folders
│   │                           map 1:1 to libretro-thumbnails repo names.
│   ├── metadata/
│   │   └── metadata.go      # Provider that asks OpenVGDB first then
│   │                          libretro-thumbnails for missing cover/snap
│   ├── emulator/
│   │   └── emulator.go      # Resolve() picks override > standalone >
│   │                          libretro. findStandalone tries
│   │                          ./emulators/<App>.app first, then
│   │                          /Applications, then PATH.
│   ├── download/            # ROM downloader (queued, refetched 1Hz by UI)
│   ├── database/
│   │   ├── db/db.go         # GORM repo methods
│   │   └── models/models.go # Game, Profile, PlayHistory, Favorite,
│   │                          EmulatorBinding, Download, Setting
│   └── handlers/            # Echo routes under /api/v1/*
│       ├── routes.go        # route map (table at top of file)
│       ├── status.go        # /status — metadata readiness, counts, etc.
│       ├── games.go         # list/get/play (resolves emulator launcher)
│       ├── library.go       # scan + scan status
│       ├── images.go        # cover/screenshot proxy with disk cache,
│       │                      sends browser-like UA (gamefaqs Cloudflare)
│       ├── settings.go      # CRUD + OpenVGDB download trigger
│       ├── downloads.go     # ROM download queue CRUD
│       ├── emulators.go     # per-platform emulator overrides
│       └── profiles.go      # multi-profile model (UI hides it; backend
│                              still serves; one default profile auto-
│                              provisioned at boot)
├── retrox-web/              # SPA source (TS, React 19, rsbuild, Tailwind 3)
│   ├── src/
│   │   ├── components/      # App, Sidebar, Library, GameCard,
│   │   │                      GameDetail (route), PlayButton,
│   │   │                      DownloadsPage, SettingsPage, ui (Button,
│   │   │                      Spinner, Wordmark, Modal, Badge…)
│   │   ├── lib/
│   │   │   ├── api.ts       # fetch wrapper, unwraps {data}/{error}
│   │   │   ├── hooks.ts     # react-query hooks, qk = query keys
│   │   │   ├── types.ts     # mirrors Go JSON tags
│   │   │   └── format.ts    # formatBytes, year
│   │   ├── globals.css
│   │   └── main.tsx
│   ├── tailwind.config.ts   # palette (ink-* surfaces, accent violet,
│   │                          cyan2, success emerald), bg-accent-gradient
│   ├── rsbuild.config.ts    # dev :50001 proxying /api → :50000
│   ├── tsconfig.json        # paths @/* → src/*
│   └── package.json
├── scripts/
│   └── package.sh           # builds dist/retrox-portable-YYYYMMDD.zip
└── .claude/
    └── launch.json          # preview tool config (no env = relative paths)
```

Runtime-only dirs (gitignored, created on demand):

```
data/         # retrox.db, openvgdb.sqlite, imgcache/
roms/         # user ROMs, per-platform subdirs
emulators/    # bundled .app + cores/ libretro
web/          # frontend build copied here for //go:embed
dist/         # output of scripts/package.sh
```

## Conventions

- **Ports**: 50000 (Go API + prod SPA), 50001 (rsbuild dev with /api proxy).
  Don't reintroduce 43222 / 43220 anywhere — old.
- **API envelope**: every handler returns `{"data": ...}` on success,
  `{"error": "msg"}` on failure. The frontend `req<T>()` unwraps `data`,
  throws server's French error on non-2xx.
- **Errors in French**: user-visible error messages are French. Internal
  log messages can stay English.
- **Icons**: lucide-react only. NEVER use emoji in the UI — the user
  explicitly asked them removed. Unicode arrows/symbols (★ ✕ ▶ ↓ etc.)
  also count as "emoji-ish" and must use proper SVG icons.
- **Frontend stack**: rsbuild (no Webpack/Vite tweaks needed), Tailwind 3
  with custom palette, `clsx` for conditional classes, lucide-react for
  icons, react-query v5 with central `qk` keys.
- **Cover URLs**: OpenVGDB stores hotlinks to `gamefaqs.gamespot.com`
  which is behind Cloudflare. The proxy [handlers/images.go](internal/handlers/images.go)
  sends a real-browser User-Agent + Accept-Language to bypass the bot
  challenge. Don't strip those headers.
- **Hash normalization is critical**: the scanner's `hashFile(path, platformID)`
  reads platform-specific headers off before computing CRC/MD5/SHA1.
  See [scanner.go:detectROMHeader](internal/scanner/scanner.go) and
  [scanner_test.go](internal/scanner/scanner_test.go) for the format
  detection table. Adding a new system that has a ROM header? Extend
  detectROMHeader + add a test case.

## Common tasks

- **Build everything**: `make build` (frontend → web/ → embed → Go binary)
- **Dev (frontend hot-reload)**: `./retrox` in one terminal + `npm run dev`
  in `retrox-web/` for hot reload on :50001
- **Run tests**: `go test ./...` (scanner_test covers all header formats)
- **Package portable zip**: `./scripts/package.sh [--with-roms]`
- **Add a platform**: edit `internal/platforms/platforms.go` catalog;
  set `OpenVGDBID` (query `openvgdb.sqlite` SYSTEMS table) and
  `LibretroThumbsName` (folder name at github.com/libretro-thumbnails).
- **Add a ROM header format**: edit `detectROMHeader` in
  `internal/scanner/scanner.go` + add a test case.
- **Add a libretro core to portable bundle**: download from
  `https://buildbot.libretro.com/nightly/apple/osx/x86_64/latest/<core>_libretro.dylib.zip`
  (RetroArch is x86_64 from brew on macOS) and drop the .dylib into
  `./emulators/cores/`.

## Gotchas

- **RetroArch on Apple Silicon needs Rosetta** (`softwareupdate
  --install-rosetta`). The libretro cores in the bundle are also x86_64.
- **Gatekeeper on first launch of bundled .app on a new Mac**: even with
  `xattr -dr com.apple.quarantine` in scripts/package.sh, sometimes
  macOS still asks. User does clic-droit → Ouvrir once per app.
- **DuckStation is not in Homebrew anymore** (license change). If you
  need PSX you fall back to the libretro `swanstation` core (default in
  catalog). DuckStation can still be installed manually from
  https://www.duckstation.org if the user wants it.
- **MetadataCacheEntry table** existed in early versions (cached the
  ScreenScraper JSON). Removed. Existing DBs still have the column;
  harmless — GORM AutoMigrate doesn't drop tables.
- **The `web/` folder is regenerated by `make web`**. Don't edit by hand.
- **`launch.json` deliberately has no env vars** so binaryDir-relative
  defaults kick in. Don't reintroduce hardcoded /tmp paths there.
- **Profiles still exist in the DB but the UI hides them**. One profile
  is auto-provisioned at boot (`App.ensureDefaultProfile`). All
  favorites/history flow through `defaultProfileUid` exposed in /status.

## Out of scope / known limitations

- Arcade ROMs (MAME zips): not extracted on the fly. Match relies on the
  filename (extensionless) hitting OpenVGDB's MAME entries.
- Disc images > 256 MB (`maxHashBytes`): no hash computed; filename match only.
- Hacked/translated ROMs: hash unique → no OpenVGDB row → filename fallback.
- RetroArch user config lives at `~/Library/Application Support/RetroArch/`
  and is *not* in the portable bundle. Controller bindings reset on a new
  Mac unless the user copies that dir manually.

## Communication style

User speaks French in this codebase; respond in French unless the user
switches. Code comments stay English. Keep replies tight — the user has
been clear about wanting concise output, not exposition.
