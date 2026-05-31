package models

import "time"

type BaseModel struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// +---------------------+
// |      Profiles       |
// +---------------------+

// Profile scopes play history + favorites. RETROX auto-provisions a single
// default profile at boot and uses it transparently — the UI doesn't
// expose the model at all, but the rows are kept so future multi-user
// support is a frontend change rather than a migration.
type Profile struct {
	BaseModel
	UID    string `gorm:"column:uid;uniqueIndex;size:64;not null" json:"uid"`
	Name   string `gorm:"column:name;size:30;not null" json:"name"`
	Avatar string `gorm:"column:avatar;size:16" json:"avatar"`
	Color  string `gorm:"column:color;size:16" json:"color"`
}

// +---------------------+
// |        Games        |
// +---------------------+

// Game is one scanned ROM file plus the metadata the provider returned
// for it. The file path is the natural key — a rescan upserts on Path so
// re-running the scan is idempotent and cheap.
type Game struct {
	BaseModel
	Path     string `gorm:"column:path;uniqueIndex;size:1024;not null" json:"path"`
	FileName string `gorm:"column:file_name;size:512" json:"fileName"`
	FileSize int64  `gorm:"column:file_size" json:"fileSize"`

	// Hashes used to match the exact ROM in OpenVGDB.
	CRC  string `gorm:"column:crc;size:8;index" json:"crc"`
	MD5  string `gorm:"column:md5;size:32;index" json:"md5"`
	SHA1 string `gorm:"column:sha1;size:40;index" json:"sha1"`

	// Our internal platform id (see internal/platforms). Indexed so the
	// library can build one rail per platform cheaply.
	PlatformID string `gorm:"column:platform_id;size:32;index" json:"platformId"`

	// Metadata (filled by metadata.Provider; Title falls back to a cleaned
	// filename so the game is still listed before/without a match).
	Title       string `gorm:"column:title;size:512" json:"title"`
	Description string `gorm:"column:description;type:text" json:"description"`
	Genre       string `gorm:"column:genre;size:255" json:"genre"`
	Developer   string `gorm:"column:developer;size:255" json:"developer"`
	Publisher   string `gorm:"column:publisher;size:255" json:"publisher"`
	ReleaseDate string `gorm:"column:release_date;size:32" json:"releaseDate"`
	Region      string `gorm:"column:region;size:32" json:"region"`

	// Public media URLs (OpenVGDB cover URL on gamefaqs.gamespot.com,
	// libretro-thumbnails CDN). Served through /games/:id/image/:kind,
	// which disk-caches the fetched bytes.
	CoverURL      string `gorm:"column:cover_url;size:1024" json:"coverUrl"`
	ScreenshotURL string `gorm:"column:screenshot_url;size:1024" json:"screenshotUrl"`

	Scraped bool `gorm:"column:scraped;default:false" json:"scraped"`
	// Missing flags a row whose file vanished between scans — kept so play
	// history doesn't dangle, hidden from the library until the file returns.
	Missing bool `gorm:"column:missing;default:false" json:"missing"`
}

// +---------------------+
// |    Play history     |
// +---------------------+

// PlayHistory is per (profile, game): last launch + a play counter. Powers
// the "Récemment joués" rail.
type PlayHistory struct {
	BaseModel
	ProfileUID string `gorm:"column:profile_uid;size:64;not null;uniqueIndex:idx_profile_game,priority:1" json:"profileUid"`
	GameID     uint   `gorm:"column:game_id;not null;uniqueIndex:idx_profile_game,priority:2" json:"gameId"`
	PlayCount  int    `gorm:"column:play_count;default:0" json:"playCount"`
	// Denormalised so rails render without a join.
	Title      string `gorm:"column:title;size:512" json:"title"`
	PlatformID string `gorm:"column:platform_id;size:32" json:"platformId"`
	CoverURL   string `gorm:"column:cover_url;size:1024" json:"coverUrl"`
}

// Favorite — per-profile "Ma liste".
type Favorite struct {
	BaseModel
	ProfileUID string `gorm:"column:profile_uid;size:64;not null;uniqueIndex:idx_profile_fav,priority:1" json:"profileUid"`
	GameID     uint   `gorm:"column:game_id;not null;uniqueIndex:idx_profile_fav,priority:2" json:"gameId"`
	Title      string `gorm:"column:title;size:512" json:"title"`
	PlatformID string `gorm:"column:platform_id;size:32" json:"platformId"`
	CoverURL   string `gorm:"column:cover_url;size:1024" json:"coverUrl"`
}

// +---------------------+
// |   Emulator config   |
// +---------------------+

// EmulatorBinding overrides the built-in default emulator for a platform.
// Empty fields fall back to the platform catalog default (see
// internal/platforms + internal/emulator). One row per platform id.
type EmulatorBinding struct {
	PlatformID string    `gorm:"column:platform_id;primaryKey;size:32" json:"platformId"`
	Command    string    `gorm:"column:command;size:1024" json:"command"`
	Args       string    `gorm:"column:args;size:1024" json:"args"`
	Core       string    `gorm:"column:core;size:128" json:"core"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// +---------------------+
// |     Downloads       |
// +---------------------+

// Download is one queued/active/finished ROM fetch into a platform's
// library root. Progress is updated in place as bytes stream in.
type Download struct {
	BaseModel
	URL        string  `gorm:"column:url;size:2048;not null" json:"url"`
	DestPath   string  `gorm:"column:dest_path;size:1024" json:"destPath"`
	PlatformID string  `gorm:"column:platform_id;size:32" json:"platformId"`
	Title      string  `gorm:"column:title;size:512" json:"title"`
	Status     string  `gorm:"column:status;size:16;index" json:"status"` // queued|downloading|done|error|canceled
	Progress   float64 `gorm:"column:progress" json:"progress"`           // 0..1
	BytesDone  int64   `gorm:"column:bytes_done" json:"bytesDone"`
	BytesTotal int64   `gorm:"column:bytes_total" json:"bytesTotal"`
	Error      string  `gorm:"column:error;size:512" json:"error"`
}

// +---------------------+
// |      Settings       |
// +---------------------+

// Setting is a single key/value pair for admin-mutable server config
// (ROM dirs, RetroArch paths). Env vars are the boot fallback.
type Setting struct {
	Key       string    `gorm:"primarykey;size:64" json:"key"`
	Value     string    `gorm:"column:value;type:text" json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}
