// Package db is the GORM/SQLite persistence layer. One Database wraps a
// *gorm.DB and exposes intention-revealing methods (UpsertGameFile,
// IncrementPlay…) so the rest of the app never touches GORM directly.
package db

import (
	"errors"
	"time"

	"retrox/internal/database/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type Database struct {
	gormdb *gorm.DB
}

func Open(path string) (*Database, error) {
	g, err := gorm.Open(sqlite.Open(path+"?_journal_mode=WAL&_foreign_keys=on"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	if err := g.AutoMigrate(
		&models.Setting{},
		&models.Profile{},
		&models.Game{},
		&models.PlayHistory{},
		&models.Favorite{},
		&models.EmulatorBinding{},
		&models.Download{},
	); err != nil {
		return nil, err
	}
	return &Database{gormdb: g}, nil
}

func (db *Database) Close() error {
	sqlDB, err := db.gormdb.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// -----------------------------------------------------------------------------
// Settings (admin-mutable key/value; env vars are the boot fallback)
// -----------------------------------------------------------------------------

func (db *Database) GetSetting(key string) (string, error) {
	var s models.Setting
	err := db.gormdb.Where("key = ?", key).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return s.Value, nil
}

func (db *Database) SetSetting(key, value string) error {
	return db.gormdb.
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
		}).
		Create(&models.Setting{Key: key, Value: value, UpdatedAt: time.Now()}).Error
}

// GetSettings reads several keys at once. Missing keys come back as ""
// so callers can iterate without nil checks.
func (db *Database) GetSettings(keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = ""
	}
	if len(keys) == 0 {
		return out, nil
	}
	var rows []models.Setting
	if err := db.gormdb.Where("key IN ?", keys).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.Key] = r.Value
	}
	return out, nil
}

// -----------------------------------------------------------------------------
// Games
// -----------------------------------------------------------------------------

// UpsertGameFile records the file-level facts of a scanned ROM (path,
// name, size, hashes, platform) and clears the Missing flag. It does NOT
// touch scrape metadata — a plain rescan must never wipe a good scrape.
// Keyed by Path so re-scanning is idempotent and keeps stable ids.
func (db *Database) UpsertGameFile(g *models.Game) (*models.Game, error) {
	g.Missing = false
	err := db.gormdb.
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "path"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"file_name", "file_size", "crc", "md5", "sha1",
				"platform_id", "missing", "updated_at",
			}),
		}).
		Create(g).Error
	if err != nil {
		return nil, err
	}
	var refreshed models.Game
	if err := db.gormdb.Where("path = ?", g.Path).First(&refreshed).Error; err != nil {
		return nil, err
	}
	return &refreshed, nil
}

// UpdateGameMetadata patches the scraped fields on an existing row. The
// `fields` map is applied as a partial update so the scanner can write
// only what ScreenScraper actually returned.
func (db *Database) UpdateGameMetadata(id uint, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	fields["updated_at"] = time.Now()
	return db.gormdb.Model(&models.Game{}).Where("id = ?", id).Updates(fields).Error
}

func (db *Database) GetGame(id uint) (*models.Game, error) {
	var g models.Game
	if err := db.gormdb.First(&g, id).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

// ListGames returns every present (non-missing) game, newest first.
func (db *Database) ListGames() ([]*models.Game, error) {
	var res []*models.Game
	err := db.gormdb.Where("missing = ?", false).Order("title ASC, file_name ASC").Find(&res).Error
	return res, err
}

func (db *Database) ListGamesByPlatform(platformID string) ([]*models.Game, error) {
	var res []*models.Game
	err := db.gormdb.Where("platform_id = ? AND missing = ?", platformID, false).
		Order("title ASC, file_name ASC").Find(&res).Error
	return res, err
}

// MarkMissingNotIn flags rows whose path wasn't seen on the last scan.
// We flag rather than delete so play history / favorites don't dangle;
// the row reappears (Missing=false) if the file comes back.
func (db *Database) MarkMissingNotIn(paths []string) (int64, error) {
	if len(paths) == 0 {
		res := db.gormdb.Model(&models.Game{}).Where("missing = ?", false).
			Update("missing", true)
		return res.RowsAffected, res.Error
	}
	res := db.gormdb.Model(&models.Game{}).
		Where("path NOT IN ? AND missing = ?", paths, false).
		Update("missing", true)
	return res.RowsAffected, res.Error
}

// CountGames returns (scraped, total) over present games — powers the
// settings "N jeux, M scrappés" line.
func (db *Database) CountGames() (scraped, total int64, err error) {
	if err = db.gormdb.Model(&models.Game{}).Where("missing = ?", false).Count(&total).Error; err != nil {
		return
	}
	err = db.gormdb.Model(&models.Game{}).Where("missing = ? AND scraped = ?", false, true).Count(&scraped).Error
	return
}

// -----------------------------------------------------------------------------
// Profiles
// -----------------------------------------------------------------------------

func (db *Database) ListProfiles() ([]*models.Profile, error) {
	var res []*models.Profile
	err := db.gormdb.Order("created_at ASC").Find(&res).Error
	return res, err
}

func (db *Database) GetProfile(uid string) (*models.Profile, error) {
	if uid == "" {
		return nil, errors.New("profile uid required")
	}
	var p models.Profile
	if err := db.gormdb.Where("uid = ?", uid).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (db *Database) CreateProfile(p *models.Profile) (*models.Profile, error) {
	if err := db.gormdb.Create(p).Error; err != nil {
		return nil, err
	}
	return p, nil
}

func (db *Database) UpdateProfile(uid, name, avatar, color string) (*models.Profile, error) {
	p, err := db.GetProfile(uid)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if name != "" {
		updates["name"] = name
	}
	if avatar != "" {
		updates["avatar"] = avatar
	}
	if color != "" {
		updates["color"] = color
	}
	if len(updates) == 0 {
		return p, nil
	}
	if err := db.gormdb.Model(p).Updates(updates).Error; err != nil {
		return nil, err
	}
	return db.GetProfile(uid)
}

func (db *Database) DeleteProfile(uid string) error {
	return db.gormdb.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("profile_uid = ?", uid).Delete(&models.PlayHistory{}).Error; err != nil {
			return err
		}
		if err := tx.Where("profile_uid = ?", uid).Delete(&models.Favorite{}).Error; err != nil {
			return err
		}
		return tx.Where("uid = ?", uid).Delete(&models.Profile{}).Error
	})
}

// -----------------------------------------------------------------------------
// Play history (per profile/game)
// -----------------------------------------------------------------------------

func (db *Database) ListPlayHistory(profileUID string) ([]*models.PlayHistory, error) {
	var res []*models.PlayHistory
	err := db.gormdb.Where("profile_uid = ?", profileUID).Order("updated_at DESC").Find(&res).Error
	return res, err
}

// IncrementPlay bumps the play counter for (profile, game), refreshing
// the denormalised title/platform/cover so the "Reprendre" rail renders
// without a join. Upserts the row on first play.
func (db *Database) IncrementPlay(profileUID string, g *models.Game) error {
	return db.gormdb.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "profile_uid"}, {Name: "game_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"play_count":  gorm.Expr("play_count + 1"),
			"title":       g.Title,
			"platform_id": g.PlatformID,
			"cover_url":   g.CoverURL,
			"updated_at":  time.Now(),
		}),
	}).Create(&models.PlayHistory{
		ProfileUID: profileUID,
		GameID:     g.ID,
		PlayCount:  1,
		Title:      g.Title,
		PlatformID: g.PlatformID,
		CoverURL:   g.CoverURL,
	}).Error
}

// -----------------------------------------------------------------------------
// Favorites (per profile "Ma liste")
// -----------------------------------------------------------------------------

func (db *Database) ListFavorites(profileUID string) ([]*models.Favorite, error) {
	var res []*models.Favorite
	err := db.gormdb.Where("profile_uid = ?", profileUID).Order("created_at DESC").Find(&res).Error
	return res, err
}

func (db *Database) AddFavorite(profileUID string, g *models.Game) error {
	return db.gormdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "profile_uid"}, {Name: "game_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"title", "platform_id", "cover_url"}),
	}).Create(&models.Favorite{
		ProfileUID: profileUID,
		GameID:     g.ID,
		Title:      g.Title,
		PlatformID: g.PlatformID,
		CoverURL:   g.CoverURL,
	}).Error
}

func (db *Database) RemoveFavorite(profileUID string, gameID uint) error {
	return db.gormdb.Where("profile_uid = ? AND game_id = ?", profileUID, gameID).
		Delete(&models.Favorite{}).Error
}

// -----------------------------------------------------------------------------
// Emulator bindings (per-platform launch overrides)
// -----------------------------------------------------------------------------

func (db *Database) ListEmulatorBindings() ([]*models.EmulatorBinding, error) {
	var res []*models.EmulatorBinding
	err := db.gormdb.Find(&res).Error
	return res, err
}

func (db *Database) GetEmulatorBinding(platformID string) (*models.EmulatorBinding, error) {
	var b models.EmulatorBinding
	err := db.gormdb.Where("platform_id = ?", platformID).First(&b).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (db *Database) UpsertEmulatorBinding(b *models.EmulatorBinding) error {
	b.UpdatedAt = time.Now()
	return db.gormdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "platform_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"command", "args", "core", "updated_at"}),
	}).Create(b).Error
}

func (db *Database) DeleteEmulatorBinding(platformID string) error {
	return db.gormdb.Where("platform_id = ?", platformID).Delete(&models.EmulatorBinding{}).Error
}

// -----------------------------------------------------------------------------
// Downloads
// -----------------------------------------------------------------------------

func (db *Database) CreateDownload(d *models.Download) (*models.Download, error) {
	if err := db.gormdb.Create(d).Error; err != nil {
		return nil, err
	}
	return d, nil
}

func (db *Database) GetDownload(id uint) (*models.Download, error) {
	var d models.Download
	if err := db.gormdb.First(&d, id).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (db *Database) ListDownloads() ([]*models.Download, error) {
	var res []*models.Download
	err := db.gormdb.Order("created_at DESC").Find(&res).Error
	return res, err
}

// SaveDownload persists the whole row. The downloader throttles how
// often it calls this (progress changes constantly).
func (db *Database) SaveDownload(d *models.Download) error {
	return db.gormdb.Save(d).Error
}

// FailInterruptedDownloads runs at boot: anything still "downloading" or
// "queued" from a previous process can't resume, so mark it errored.
func (db *Database) FailInterruptedDownloads() error {
	return db.gormdb.Model(&models.Download{}).
		Where("status IN ?", []string{"downloading", "queued"}).
		Updates(map[string]any{"status": "error", "error": "interrompu par un redémarrage"}).Error
}

