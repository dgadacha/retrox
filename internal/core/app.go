package core

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync/atomic"

	"retrox/internal/database/db"
	"retrox/internal/database/models"
	"retrox/internal/download"
	"retrox/internal/emulator"
	"retrox/internal/igdb"
	"retrox/internal/libretrothumbs"
	"retrox/internal/metadata"
	"retrox/internal/openvgdb"
	"retrox/internal/rawg"
	"retrox/internal/scanner"
	"retrox/internal/sources"
	"retrox/internal/tgdb"
)

// App is the singleton wiring container handed to every HTTP handler so
// they can reach the DB, the metadata source and the download manager
// without threading a dozen arguments around.
type App struct {
	Config    *Config
	Database  *db.Database
	OpenVGDB  *openvgdb.Store
	IGDB      *igdb.Client
	TGDB      *tgdb.Client
	RAWG      *rawg.Client
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

	// settingsVersion bumps every time a metadata source's credentials
	// or the user preference changes. The catalogue platforms cache
	// uses it as part of its key, so a settings change invalidates the
	// cache without core having to know handlers exists.
	settingsVersion atomic.Int64
}

// SettingsVersion returns the monotonic version counter that bumps on
// every credential / preference change. Handlers that cache
// preference-dependent state use it as a cache key component.
func (a *App) SettingsVersion() int64 { return a.settingsVersion.Load() }

func (a *App) bumpSettings() { a.settingsVersion.Add(1) }

// Setting keys — the admin UI overrides the env-var defaults by writing
// these into the settings table. Env vars remain the boot fallback.
const (
	SettingROMDirs            = "rom_dirs" // ':'-separated
	SettingRetroArchBin       = "retroarch_bin"
	SettingRetroArchCores     = "retroarch_cores"
	SettingOpenVGDBPath       = "openvgdb_path"
	SettingIGDBClientID       = "igdb_client_id"
	SettingIGDBClientSecret   = "igdb_client_secret"
	SettingTGDBKey            = "tgdb_key"
	SettingRAWGKey            = "rawg_key"
	SettingMetadataPreference = "metadata_preference"
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
	libretrothumbs.SetCacheDir(filepath.Join(cfg.Data.Dir, "libretroCache"))
	igdbClient := igdb.New()
	if cfg.Metadata.IGDBClientID != "" && cfg.Metadata.IGDBClientSecret != "" {
		igdbClient.SetCredentials(cfg.Metadata.IGDBClientID, cfg.Metadata.IGDBClientSecret)
	}
	tgdbClient := tgdb.New()
	if cfg.Metadata.TGDBKey != "" {
		tgdbClient.SetCredentials(cfg.Metadata.TGDBKey)
	}
	rawgClient := rawg.New()
	if cfg.Metadata.RAWGKey != "" {
		rawgClient.SetCredentials(cfg.Metadata.RAWGKey)
	}

	app := &App{
		Config:   cfg,
		Database: database,
		OpenVGDB: store,
		IGDB:     igdbClient,
		TGDB:     tgdbClient,
		RAWG:     rawgClient,
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

// ApplyIGDBCredentials persists + hot-swaps the IGDB OAuth keys.
// Empty strings disable IGDB until the user pastes new ones.
func (a *App) ApplyIGDBCredentials(clientID, clientSecret string) error {
	if err := a.Database.SetSetting(SettingIGDBClientID, clientID); err != nil {
		return err
	}
	if err := a.Database.SetSetting(SettingIGDBClientSecret, clientSecret); err != nil {
		return err
	}
	a.Config.Metadata.IGDBClientID = clientID
	a.Config.Metadata.IGDBClientSecret = clientSecret
	a.IGDB.SetCredentials(clientID, clientSecret)
	a.bumpSettings()
	return nil
}

// ApplyTGDBKey persists + hot-swaps the TheGamesDB API key.
func (a *App) ApplyTGDBKey(key string) error {
	if err := a.Database.SetSetting(SettingTGDBKey, key); err != nil {
		return err
	}
	a.Config.Metadata.TGDBKey = key
	a.TGDB.SetCredentials(key)
	a.bumpSettings()
	return nil
}

// ApplyRAWGKey persists + hot-swaps the RAWG.io API key.
func (a *App) ApplyRAWGKey(key string) error {
	if err := a.Database.SetSetting(SettingRAWGKey, key); err != nil {
		return err
	}
	a.Config.Metadata.RAWGKey = key
	a.RAWG.SetCredentials(key)
	a.bumpSettings()
	return nil
}

// ApplyMetadataPreference picks which catalogue backend wins when more
// than one is configured. Values: "auto" | "openvgdb" | "igdb" | "tgdb" | "rawg".
func (a *App) ApplyMetadataPreference(pref string) error {
	switch pref {
	case "", "auto", "openvgdb", "igdb", "tgdb", "rawg":
	default:
		return fmt.Errorf("préférence invalide %q", pref)
	}
	if pref == "" {
		pref = "auto"
	}
	if err := a.Database.SetSetting(SettingMetadataPreference, pref); err != nil {
		return err
	}
	a.Config.Metadata.Preference = pref
	a.bumpSettings()
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
		SettingOpenVGDBPath, SettingIGDBClientID, SettingIGDBClientSecret,
		SettingTGDBKey, SettingRAWGKey, SettingMetadataPreference,
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
	if v := rows[SettingIGDBClientID]; v != "" {
		cfg.Metadata.IGDBClientID = v
	}
	if v := rows[SettingIGDBClientSecret]; v != "" {
		cfg.Metadata.IGDBClientSecret = v
	}
	if v := rows[SettingTGDBKey]; v != "" {
		cfg.Metadata.TGDBKey = v
	}
	if v := rows[SettingRAWGKey]; v != "" {
		cfg.Metadata.RAWGKey = v
	}
	if v := rows[SettingMetadataPreference]; v != "" {
		cfg.Metadata.Preference = v
	}
}
