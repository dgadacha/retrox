package core

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// binaryDir returns the directory containing the running executable —
// the natural anchor for a portable, ship-everything-in-one-folder
// install. When the program is launched via `go run` (the binary lives
// in a /tmp/go-build* dir) it falls back to the current working dir,
// which is the project root in dev.
func binaryDir() string {
	exe, err := os.Executable()
	if err != nil {
		cwd, _ := os.Getwd()
		return cwd
	}
	if real, lerr := filepath.EvalSymlinks(exe); lerr == nil {
		exe = real
	}
	dir := filepath.Dir(exe)
	if strings.Contains(dir, "/go-build") || strings.HasPrefix(dir, "/var/folders") {
		cwd, _ := os.Getwd()
		return cwd
	}
	return dir
}

// Config — runtime config loaded once at startup. Mirrors Notflix's shape:
// env vars (RETROX_*) override the defaults, an admin UI can persist a
// subset into the settings table afterwards.
type Config struct {
	Server struct {
		Host string
		Port int
	}
	Metadata struct {
		// Path is where openvgdb.sqlite lives. Defaults to
		// <datadir>/openvgdb.sqlite; override with RETROX_OPENVGDB_PATH
		// if you keep the DB on a shared volume.
		Path string

		// IGDBClientID + IGDBClientSecret are the Twitch developer
		// credentials used to authenticate against the IGDB REST API.
		// Both empty → IGDB is "not configured" and the catalogue
		// falls back to OpenVGDB / libretro-thumbnails only.
		IGDBClientID     string
		IGDBClientSecret string
	}
	Library struct {
		// Colon-separated list of root folders to scan for ROMs. Each root
		// is scanned recursively; the platform is guessed from the file
		// extension (and, as a hint, the immediate parent folder name).
		Roots []string
	}
	Emulator struct {
		// Optional overrides for where RetroArch + its libretro cores
		// live. Empty → the emulator package auto-detects per-OS.
		RetroArchBin   string
		RetroArchCores string
		// EmulatorsDir is the project-bundled emulators folder (e.g.
		// <binDir>/emulators). When set, the launcher prefers .app
		// bundles found there over /Applications system installs, so
		// the same project folder zipped + moved to another Mac is
		// fully self-contained.
		EmulatorsDir string
	}
	Data struct {
		Dir string // resolved at boot
	}
}

func (c *Config) PortStr() string { return strconv.Itoa(c.Server.Port) }

func loadConfig() (*Config, error) {
	cfg := &Config{}

	// Datadir — defaults to <binaryDir>/data so a portable RETROX
	// folder ships its DB, OpenVGDB and image cache next to the
	// binary. Override with RETROX_DATA_DIR for a non-portable
	// install (e.g. a system-wide service).
	cfg.Data.Dir = firstNonEmpty(
		os.Getenv("RETROX_DATA_DIR"),
		os.Getenv("RETROX_DATADIR"),
		filepath.Join(binaryDir(), "data"),
	)
	if err := os.MkdirAll(cfg.Data.Dir, 0o755); err != nil {
		return nil, err
	}

	// Server bind. 50000 is the retrox default.
	cfg.Server.Host = firstNonEmpty(os.Getenv("RETROX_SERVER_HOST"), "127.0.0.1")
	cfg.Server.Port = intOrDefault(os.Getenv("RETROX_SERVER_PORT"), 50000)

	// OpenVGDB SQLite path — auto-downloaded on demand from the settings
	// UI. Defaults to <datadir>/openvgdb.sqlite so a fresh install is
	// fully self-contained.
	cfg.Metadata.Path = firstNonEmpty(
		os.Getenv("RETROX_OPENVGDB_PATH"),
		filepath.Join(cfg.Data.Dir, "openvgdb.sqlite"),
	)

	// IGDB credentials — env-var seeded, overridable from the settings
	// UI for first-time setup. Both empty = IGDB disabled.
	cfg.Metadata.IGDBClientID = os.Getenv("RETROX_IGDB_CLIENT_ID")
	cfg.Metadata.IGDBClientSecret = os.Getenv("RETROX_IGDB_CLIENT_SECRET")

	// Library roots — RETROX_ROM_DIRS is a ':'-separated list. Default
	// to <binaryDir>/roms so dropping a ROM in the project folder
	// "just works" with no setup.
	cfg.Library.Roots = splitPaths(os.Getenv("RETROX_ROM_DIRS"))
	if len(cfg.Library.Roots) == 0 {
		cfg.Library.Roots = []string{filepath.Join(binaryDir(), "roms")}
	}
	for _, r := range cfg.Library.Roots {
		_ = os.MkdirAll(r, 0o755)
	}

	// Emulator overrides — all three optional.
	cfg.Emulator.RetroArchBin = os.Getenv("RETROX_RETROARCH_BIN")
	cfg.Emulator.RetroArchCores = os.Getenv("RETROX_RETROARCH_CORES")
	cfg.Emulator.EmulatorsDir = firstNonEmpty(
		os.Getenv("RETROX_EMULATORS_DIR"),
		filepath.Join(binaryDir(), "emulators"),
	)
	// If the cores default isn't set, fall back to the bundled folder
	// inside the project's emulators dir.
	if cfg.Emulator.RetroArchCores == "" {
		cfg.Emulator.RetroArchCores = filepath.Join(cfg.Emulator.EmulatorsDir, "cores")
	}

	return cfg, nil
}

func ensureDir(p string) error { return os.MkdirAll(p, 0o755) }

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func intOrDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// splitPaths splits a ':'-separated (or ';' on the off-chance) path list,
// trims blanks, and returns the cleaned absolute-ish roots.
func splitPaths(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	sep := ":"
	if strings.Contains(s, ";") {
		sep = ";"
	}
	var out []string
	for _, part := range strings.Split(s, sep) {
		p := strings.TrimSpace(part)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
