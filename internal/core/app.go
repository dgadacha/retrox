package core

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"strings"

	"retrox/internal/database/db"
	"retrox/internal/database/models"
	"retrox/internal/download"
	"retrox/internal/emulator"
	"retrox/internal/libretrothumbs"
	"retrox/internal/metadata"
	"retrox/internal/openvgdb"
	"retrox/internal/scanner"
	"retrox/internal/sources"
)

// App is the singleton wiring container handed to every HTTP handler so
// they can reach the DB, the metadata source and the download manager
// without threading a dozen arguments around.
type App struct {
	Config    *Config
	Database  *db.Database
	OpenVGDB  *openvgdb.Store
	Thumbs    *libretrothumbs.Client
	Metadata  *metadata.Provider
	Downloads *download.Manager
	// Sources is the registry of remote ROM catalogs the UI can browse.
	// Order matters only for the picker in the sidebar.
	Sources []sources.Source

	// defaultProfileUID is provisioned at boot and used transparently by
	// the UI: favorites and history belong to this single instance-wide
	// profile (the multi-profile model survives in the DB but is unused).
	defaultProfileUID string
}

// Setting keys — the admin UI overrides the env-var defaults by writing
// these into the settings table. Env vars remain the boot fallback.
const (
	SettingROMDirs        = "rom_dirs" // ':'-separated
	SettingRetroArchBin   = "retroarch_bin"
	SettingRetroArchCores = "retroarch_cores"
	SettingOpenVGDBPath   = "openvgdb_path"
)

func New() (*App, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}

	database, err := db.Open(filepath.Join(cfg.Data.Dir, "retrox.db"))
	if err != nil {
		return nil, err
	}

	// DB-persisted settings win over env-var defaults so the UI is the
	// canonical place to manage config after first boot.
	overlaySettingsOnto(database, cfg)

	// Open OpenVGDB if the file already exists; otherwise leave the store
	// in a "not ready" state so the UI can offer a Download button.
	store, ready, err := openvgdb.Open(cfg.Metadata.Path)
	if err != nil {
		log.Printf("openvgdb: %v", err)
	} else if ready {
		roms, releases := store.Counts()
		log.Printf("openvgdb: ready (%d ROMs, %d releases)", roms, releases)
	} else {
		log.Printf("openvgdb: not downloaded yet (%s)", cfg.Metadata.Path)
	}

	thumbs := libretrothumbs.NewClient()

	app := &App{
		Config:   cfg,
		Database: database,
		OpenVGDB: store,
		Thumbs:   thumbs,
		Metadata: metadata.New(store, thumbs),
		Sources: []sources.Source{
			sources.NewArchiveOrg(),
			sources.NewPDRoms(),
		},
	}
	app.Downloads = download.New(database, app.destDir)

	// A finished download chains into a background scan so the new ROM
	// shows up without the user clicking "rescan" (no-op if a scan is
	// already running).
	app.Downloads.SetOnComplete(func(d *models.Download) {
		app.ScanAsync()
	})

	// Anything left mid-flight by a previous process can't resume.
	if err := database.FailInterruptedDownloads(); err != nil {
		log.Printf("downloads: marking interrupted: %v", err)
	}

	if err := app.ensureDefaultProfile(); err != nil {
		log.Printf("default profile: %v", err)
	}

	return app, nil
}

func (a *App) Close() {
	if a.OpenVGDB != nil {
		_ = a.OpenVGDB.Close()
	}
	if a.Database != nil {
		_ = a.Database.Close()
	}
}

// ScanAsync kicks a library scan in the background unless one is already
// running. Returns immediately.
func (a *App) ScanAsync() {
	if scanner.Running() {
		return
	}
	go func() {
		if _, err := scanner.Scan(context.Background(), a.Config.Library.Roots, a.Metadata, a.Database); err != nil {
			log.Printf("scan: %v", err)
		}
	}()
}

// EmulatorConfig snapshots the emulator-related config for the launcher.
func (a *App) EmulatorConfig() emulator.Config {
	return emulator.Config{
		RetroArchBin:   a.Config.Emulator.RetroArchBin,
		RetroArchCores: a.Config.Emulator.RetroArchCores,
		EmulatorsDir:   a.Config.Emulator.EmulatorsDir,
	}
}

// DefaultProfileUID returns the UID of the auto-provisioned profile that
// owns favorites and play history for this instance.
func (a *App) DefaultProfileUID() string { return a.defaultProfileUID }

// destDir resolves where a downloaded ROM for `platformID` should land:
// the first configured root, in a per-platform subfolder.
func (a *App) destDir(platformID string) (string, error) {
	roots := a.Config.Library.Roots
	if len(roots) == 0 {
		return "", errors.New("aucun dossier ROM configuré")
	}
	base := roots[0]
	if platformID != "" {
		base = filepath.Join(base, platformID)
	}
	return base, nil
}

// ApplyServerConfig persists the admin-mutable keys and updates the
// in-memory Config. Empty strings clear the corresponding setting (next
// boot falls back to env vars).
func (a *App) ApplyServerConfig(romDirs []string, raBin, raCores string) error {
	pairs := []struct{ key, val string }{
		{SettingROMDirs, strings.Join(romDirs, ":")},
		{SettingRetroArchBin, raBin},
		{SettingRetroArchCores, raCores},
	}
	for _, p := range pairs {
		if err := a.Database.SetSetting(p.key, p.val); err != nil {
			return err
		}
	}

	if len(romDirs) > 0 {
		a.Config.Library.Roots = romDirs
		for _, r := range romDirs {
			_ = ensureDir(r)
		}
	}
	a.Config.Emulator.RetroArchBin = raBin
	a.Config.Emulator.RetroArchCores = raCores
	return nil
}

// DownloadOpenVGDB fetches the upstream SQLite zip, extracts it, and
// re-opens the store. Triggered from the settings UI; safe to call
// repeatedly to refresh.
func (a *App) DownloadOpenVGDB(ctx context.Context) error {
	if a.OpenVGDB == nil {
		return errors.New("metadata store not initialised")
	}
	return a.OpenVGDB.Download(ctx)
}

// ensureDefaultProfile reuses the first existing profile, or creates one
// named "Joueur" if the DB is empty. Idempotent across restarts.
func (a *App) ensureDefaultProfile() error {
	profiles, err := a.Database.ListProfiles()
	if err != nil {
		return err
	}
	if len(profiles) > 0 {
		a.defaultProfileUID = profiles[0].UID
		return nil
	}
	p, err := a.Database.CreateProfile(&models.Profile{
		UID:    "default",
		Name:   "Joueur",
		Avatar: "🎮",
		Color:  "#8b5cf6",
	})
	if err != nil {
		return err
	}
	a.defaultProfileUID = p.UID
	return nil
}

// overlaySettingsOnto patches DB-stored settings over the env defaults.
// Silent on errors — env values stay a working fallback.
func overlaySettingsOnto(database *db.Database, cfg *Config) {
	rows, err := database.GetSettings([]string{
		SettingROMDirs, SettingRetroArchBin, SettingRetroArchCores,
		SettingOpenVGDBPath,
	})
	if err != nil {
		return
	}
	if v := rows[SettingROMDirs]; v != "" {
		roots := splitPaths(v)
		if len(roots) > 0 {
			cfg.Library.Roots = roots
			for _, r := range roots {
				_ = ensureDir(r)
			}
		}
	}
	if v := rows[SettingRetroArchBin]; v != "" {
		cfg.Emulator.RetroArchBin = v
	}
	if v := rows[SettingRetroArchCores]; v != "" {
		cfg.Emulator.RetroArchCores = v
	}
	if v := rows[SettingOpenVGDBPath]; v != "" {
		cfg.Metadata.Path = v
	}
}
