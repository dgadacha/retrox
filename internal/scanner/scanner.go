// Package scanner walks the configured ROM roots, fingerprints each ROM
// (CRC32/MD5/SHA1), guesses its platform, matches it via the metadata
// provider (OpenVGDB + libretro-thumbnails), and upserts a Game row. It
// is the retro-game analogue of Notflix's library scanner: idempotent
// (keyed by path), prune-aware (vanished files are flagged Missing, not
// deleted), and it publishes a live progress snapshot the settings UI
// polls during a scan.
package scanner

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"retrox/internal/database/db"
	"retrox/internal/database/models"
	"retrox/internal/metadata"
	"retrox/internal/platforms"
)

// Files this small are almost certainly not ROMs (stub .bin sidecars,
// empty placeholders). 1 KiB is below even the tiniest NES homebrew.
const minROMBytes = 1 << 10

// ROMs larger than this aren't fully hashed — disc images (PS2/Wii .iso
// up to ~8 GB) would take minutes to hash and OpenVGDB matches them by
// name anyway. Cartridge systems all sit well under this.
const maxHashBytes = 256 << 20

// Source is the metadata facade the scanner depends on; declaring it as
// an interface keeps dependencies one-directional and makes scanner
// unit-testable with a fake.
type Source interface {
	Ready() bool
	Lookup(ctx context.Context, l metadata.Lookup) (*metadata.GameInfo, error)
}

// ScanReport is the post-scan summary surfaced to the settings UI.
type ScanReport struct {
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	Roots      []string  `json:"roots"`
	FilesSeen  int       `json:"filesSeen"`
	Scraped    int       `json:"scraped"`
	Unscraped  int       `json:"unscraped"`
	Removed    int       `json:"removed"`
	DurationMs int64     `json:"durationMs"`
	Error      string    `json:"error,omitempty"`
}

// -----------------------------------------------------------------------------
// Live progress (package-global; one scan at a time)
// -----------------------------------------------------------------------------

type ProgressSnapshot struct {
	Running     bool   `json:"running"`
	StartedAt   string `json:"startedAt,omitempty"`
	Total       int    `json:"total"`
	Current     int    `json:"current"`
	CurrentFile string `json:"currentFile,omitempty"`
	Scraped     int    `json:"scraped"`
	Unscraped   int    `json:"unscraped"`
}

type progressState struct {
	mu          sync.RWMutex
	running     bool
	startedAt   time.Time
	total       int
	current     int
	currentFile string
	scraped     int
	unscraped   int
}

var globalProgress = &progressState{}

// Running reports whether a scan is in flight (used to reject concurrent
// scan requests at the handler layer).
func Running() bool {
	globalProgress.mu.RLock()
	defer globalProgress.mu.RUnlock()
	return globalProgress.running
}

func Progress() ProgressSnapshot {
	globalProgress.mu.RLock()
	defer globalProgress.mu.RUnlock()
	startedAt := ""
	if !globalProgress.startedAt.IsZero() {
		startedAt = globalProgress.startedAt.Format(time.RFC3339)
	}
	return ProgressSnapshot{
		Running:     globalProgress.running,
		StartedAt:   startedAt,
		Total:       globalProgress.total,
		Current:     globalProgress.current,
		CurrentFile: globalProgress.currentFile,
		Scraped:     globalProgress.scraped,
		Unscraped:   globalProgress.unscraped,
	}
}

func (p *progressState) reset(total int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.running, p.startedAt = true, time.Now()
	p.total, p.current = total, 0
	p.currentFile, p.scraped, p.unscraped = "", 0, 0
}

func (p *progressState) setTotal(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.total = n
}

func (p *progressState) tick(file string, scraped bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current++
	p.currentFile = file
	if scraped {
		p.scraped++
	} else {
		p.unscraped++
	}
}

func (p *progressState) finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.running, p.currentFile = false, ""
}

// -----------------------------------------------------------------------------
// Scan
// -----------------------------------------------------------------------------

// Scan walks every root, processes each ROM file, and flags vanished
// rows as Missing. Safe to call only one at a time — it drives the
// package-global progress state. Honors ctx cancellation between files.
func Scan(ctx context.Context, roots []string, src Source, store *db.Database) (*ScanReport, error) {
	report := &ScanReport{StartedAt: time.Now(), Roots: roots}

	globalProgress.reset(0)
	defer globalProgress.finish()

	// Phase 1: cheap pre-walk to count ROM files for the progress bar.
	total := 0
	for _, root := range roots {
		total += countROMs(root)
	}
	globalProgress.setTotal(total)

	// Phase 2: process.
	seen := make([]string, 0, total)
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			report.Error = err.Error()
			break
		}
		walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // unreadable entry — skip, don't abort the walk
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if d.IsDir() {
				if skipDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if !platforms.IsROMExtension(path) {
				return nil
			}
			info, ierr := d.Info()
			if ierr != nil || info.Size() < minROMBytes {
				return nil
			}
			scraped := processFile(ctx, src, store, path, info)
			seen = append(seen, path)
			report.FilesSeen++
			if scraped {
				report.Scraped++
			} else {
				report.Unscraped++
			}
			globalProgress.tick(filepath.Base(path), scraped)
			return nil
		})
		if walkErr != nil && report.Error == "" {
			report.Error = walkErr.Error()
		}
	}

	// Phase 3: prune. Only when the walk wasn't cut short by an error or
	// cancellation — a transient failure shouldn't flag the whole library
	// as missing.
	if report.Error == "" {
		if removed, derr := store.MarkMissingNotIn(seen); derr == nil {
			report.Removed = int(removed)
		}
	}

	report.FinishedAt = time.Now()
	report.DurationMs = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
	return report, nil
}

// processFile fingerprints + upserts one ROM, then matches it against
// the metadata source when possible. Returns whether the row ended up
// scraped.
func processFile(ctx context.Context, src Source, store *db.Database, path string, info os.FileInfo) bool {
	platformID, _ := platforms.Guess(path)

	g := &models.Game{
		Path:       path,
		FileName:   filepath.Base(path),
		FileSize:   info.Size(),
		PlatformID: platformID,
	}
	// Hash small ROMs for exact metadata matching; skip huge disc
	// images (matched by name instead). hashFile knows how to strip
	// iNES / SMC / LNX / A78 headers and unswap N64 byte orders so
	// the digest lines up with OpenVGDB's No-Intro hashes.
	if info.Size() <= maxHashBytes {
		if crc, md5sum, sha, err := hashFile(path, platformID); err == nil {
			g.CRC, g.MD5, g.SHA1 = crc, md5sum, sha
		}
	}

	row, err := store.UpsertGameFile(g)
	if err != nil {
		log.Printf("scanner: upsert %s: %v", path, err)
		return false
	}

	// Always at least give the row a human title (cleaned filename) so it
	// lists nicely before/without scraping.
	meta := map[string]any{"title": cleanName(g.FileName)}

	scraped := false
	if src != nil && src.Ready() {
		if gi, serr := lookup(ctx, src, g); serr != nil {
			log.Printf("scanner: lookup %s: %v", g.FileName, serr)
		} else if gi != nil {
			applyMeta(meta, gi)
			scraped = true
		}
	}

	if err := store.UpdateGameMetadata(row.ID, meta); err != nil {
		log.Printf("scanner: metadata %s: %v", path, err)
	}
	return scraped
}

func lookup(ctx context.Context, src Source, g *models.Game) (*metadata.GameInfo, error) {
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return src.Lookup(cctx, metadata.Lookup{
		PlatformID:   g.PlatformID,
		CRC32:        g.CRC,
		MD5:          g.MD5,
		SHA1:         g.SHA1,
		FileName:     g.FileName,
		ROMSizeBytes: g.FileSize,
	})
}

// applyMeta copies non-empty metadata fields into the update map. Empty
// fields are skipped so a partial match never blanks out earlier data.
func applyMeta(meta map[string]any, gi *metadata.GameInfo) {
	meta["scraped"] = true
	setIf(meta, "title", gi.Title)
	setIf(meta, "description", gi.Description)
	setIf(meta, "genre", gi.Genre)
	setIf(meta, "developer", gi.Developer)
	setIf(meta, "publisher", gi.Publisher)
	setIf(meta, "release_date", gi.ReleaseDate)
	setIf(meta, "region", gi.Region)
	setIf(meta, "cover_url", gi.CoverURL)
	setIf(meta, "screenshot_url", gi.ScreenshotURL)
}

func setIf(m map[string]any, k, v string) {
	if v != "" {
		m[k] = v
	}
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

var skipFolders = map[string]bool{
	".trash": true, "@eadir": true, "bios": true, "system": true,
	"saves": true, "states": true, "screenshots": true, "media": true,
}

func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") && name != "." || skipFolders[strings.ToLower(name)]
}

func countROMs(root string) int {
	n := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !platforms.IsROMExtension(path) {
			return nil
		}
		if info, ierr := d.Info(); ierr == nil && info.Size() >= minROMBytes {
			n++
		}
		return nil
	})
	return n
}

// hashFile streams the file once and computes CRC32 (IEEE), MD5 and
// SHA1 against the *No-Intro normalized* bytes:
//
//   - Strip the iNES (16B) / FDS (16B) / Lynx LNX (64B) / Atari 7800
//     A78 (128B) magic-based headers when present.
//   - Strip the legacy SNES SMC copier header (512B) when the file
//     size mod 1024 == 512 on a .smc/.sfc.
//   - Unswap N64 ROMs delivered as .v64 (2-byte swap) or .n64 (4-byte
//     swap) so we always hash the canonical .z64 byte order.
//
// OpenVGDB's hashes come from No-Intro DATs which assume this exact
// normalization, so without this step CRC/MD5/SHA1 lookups fail on a
// majority of cartridge dumps in the wild.
func hashFile(path, platformID string) (crc, md5sum, sha string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", "", err
	}
	defer f.Close()

	skip, swap, err := detectROMHeader(f, platformID)
	if err != nil {
		return "", "", "", err
	}
	if _, err := f.Seek(int64(skip), io.SeekStart); err != nil {
		return "", "", "", err
	}

	crcH := crc32.NewIEEE()
	md5H := md5.New()
	sha1H := sha1.New()
	mw := io.MultiWriter(crcH, md5H, sha1H)

	if swap > 0 {
		// Read + swap in memory. The systems that need byteswapping
		// (N64) cap at 64 MiB so the buffer cost is bounded.
		body, rerr := io.ReadAll(f)
		if rerr != nil {
			return "", "", "", rerr
		}
		byteswap(body, swap)
		_, _ = mw.Write(body)
	} else if _, err := io.Copy(mw, f); err != nil {
		return "", "", "", err
	}

	crc = fmt.Sprintf("%08X", crcH.Sum32())
	md5sum = hex.EncodeToString(md5H.Sum(nil))
	sha = hex.EncodeToString(sha1H.Sum(nil))
	return crc, md5sum, sha, nil
}

// detectROMHeader peeks the first bytes and returns how much to skip
// (header length) plus the byteswap width (0/2/4) needed to reach the
// canonical No-Intro byte order. Returns (0, 0, nil) when no special
// handling is required. The file's read offset may be moved; the
// caller must Seek before reading the actual body.
func detectROMHeader(f *os.File, platformID string) (skip, swap int, err error) {
	var head [16]byte
	n, _ := io.ReadFull(f, head[:])
	if n < 4 {
		return 0, 0, nil
	}

	switch {
	case bytes.Equal(head[:4], []byte{'N', 'E', 'S', 0x1A}):
		return 16, 0, nil // iNES / NES 2.0
	case bytes.Equal(head[:4], []byte{'F', 'D', 'S', 0x1A}):
		return 16, 0, nil // Famicom Disk System
	case bytes.Equal(head[:4], []byte{0x80, 0x37, 0x12, 0x40}):
		return 0, 0, nil // N64 .z64 (canonical big-endian)
	case bytes.Equal(head[:4], []byte{0x37, 0x80, 0x40, 0x12}):
		return 0, 2, nil // N64 .v64 (byteswapped)
	case bytes.Equal(head[:4], []byte{0x40, 0x12, 0x37, 0x80}):
		return 0, 4, nil // N64 .n64 (little-endian)
	case bytes.Equal(head[:4], []byte("LYNX")):
		return 64, 0, nil // Atari Lynx LNX header
	case n >= 10 && bytes.Equal(head[1:10], []byte("ATARI7800")):
		return 128, 0, nil // Atari 7800 A78 header
	}

	// SNES SMC copier header — no magic, detected by file-size remainder.
	// A canonical SNES ROM is a multiple of 1024 bytes; a 512-byte tail
	// means there's a copier header glued on the front.
	if platformID == "snes" {
		if info, statErr := f.Stat(); statErr == nil && info.Size()%1024 == 512 {
			return 512, 0, nil
		}
	}

	return 0, 0, nil
}

// byteswap reorders bytes in place. width=2 flips every adjacent pair
// (V64→Z64), width=4 reverses every 4-byte word (N64→Z64). Bytes that
// don't align (i.e. a trailing partial chunk) are left untouched —
// real ROMs are always a multiple of the swap width.
func byteswap(b []byte, width int) {
	switch width {
	case 2:
		for i := 0; i+1 < len(b); i += 2 {
			b[i], b[i+1] = b[i+1], b[i]
		}
	case 4:
		for i := 0; i+3 < len(b); i += 4 {
			b[i], b[i+1], b[i+2], b[i+3] = b[i+3], b[i+2], b[i+1], b[i]
		}
	}
}

// tagPattern strips trailing region/dump tags: "(USA)", "[!]", "(Rev A)".
var tagPattern = regexp.MustCompile(`\s*[\(\[][^\)\]]*[\)\]]`)

// cleanName turns "Super Mario World (USA) [!].sfc" into "Super Mario
// World" for a presentable fallback title.
func cleanName(filename string) string {
	s := strings.TrimSuffix(filename, filepath.Ext(filename))
	s = tagPattern.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, ".", " ")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}
