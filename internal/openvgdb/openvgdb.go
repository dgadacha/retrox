// Package openvgdb is a thin reader over the OpenVGDB SQLite database
// (https://github.com/OpenVGDB/OpenVGDB), used by RETROX as a fully
// offline, account-free source of retro-game metadata.
//
// The store is downloaded once on first boot (~9 MB zipped, ~42 MB on
// disk) into <datadir>/openvgdb.sqlite. Lookups are by ROM hash —
// preferring CRC32 (No-Intro convention), then MD5, then SHA1, then the
// extension-less filename as a last resort. The result is flattened
// into a UI-friendly GameInfo that the scanner can apply directly.
package openvgdb

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DefaultZipURL is the upstream zipped SQLite release.
const DefaultZipURL = "https://github.com/OpenVGDB/OpenVGDB/releases/download/v29.0/openvgdb.zip"

// ErrNotReady is returned when Lookup is called before the SQLite file
// has been downloaded. Callers treat it as "metadata unavailable", not
// as a hard failure.
var ErrNotReady = errors.New("openvgdb: database not downloaded yet")

// GameInfo is the flattened metadata row the scanner consumes.
type GameInfo struct {
	OpenVGDBID  int
	Title       string
	Description string
	Genre       string
	Developer   string
	Publisher   string
	ReleaseDate string // free-form ("July 1993", "Mar 22, 1996", "1996")
	Region      string
	CoverURL    string // gamefaqs.gamespot.com hotlink, no auth needed
	BackCoverURL string
}

// Lookup carries everything we have on the ROM. CRC > MD5 > SHA1 > name.
type Lookup struct {
	OpenVGDBSystemID int
	CRC32            string // 8 uppercase hex chars
	MD5              string // 32 lowercase hex chars
	SHA1             string // 40 lowercase hex chars
	FileName         string // basename with extension
}

// Store wraps an opened openvgdb.sqlite. Safe for concurrent reads.
type Store struct {
	mu   sync.RWMutex
	path string
	db   *sql.DB
	// Pre-prepared statements for the hot path.
	stmtCRC, stmtMD5, stmtSHA1, stmtName *sql.Stmt
}

// Open returns a *Store for the SQLite at `path`. If the file doesn't
// exist yet, ready=false is returned with a nil error so the caller can
// surface "database not downloaded" without treating it as a failure.
func Open(path string) (*Store, bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return &Store{path: path}, false, nil
		}
		return nil, false, err
	}
	s := &Store{path: path}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.openLocked(); err != nil {
		return nil, false, err
	}
	return s, true, nil
}

// Ready reports whether the SQLite file is open and queryable.
func (s *Store) Ready() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.db != nil
}

// Path returns the configured SQLite path (whether or not it exists).
func (s *Store) Path() string { return s.path }

// Counts returns (roms, releases) row counts for the Settings UI. Zero
// values when the store isn't ready.
func (s *Store) Counts() (roms, releases int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return 0, 0
	}
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM ROMs`).Scan(&roms)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM RELEASES`).Scan(&releases)
	return
}

// Close releases the SQLite handle.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

// Lookup matches a ROM in priority order (CRC > MD5 > SHA1 > filename)
// and returns the flattened metadata. (nil, nil) when no row matched —
// the scanner records "tried, no match" without erroring.
func (s *Store) Lookup(ctx context.Context, l Lookup) (*GameInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, ErrNotReady
	}

	sysID := l.OpenVGDBSystemID
	queries := []struct {
		stmt *sql.Stmt
		key  string
	}{
		{s.stmtCRC, strings.ToUpper(strings.TrimSpace(l.CRC32))},
		{s.stmtMD5, strings.TrimSpace(l.MD5)},
		{s.stmtSHA1, strings.TrimSpace(l.SHA1)},
		{s.stmtName, extensionlessName(l.FileName)},
	}
	for _, q := range queries {
		if q.key == "" {
			continue
		}
		row := q.stmt.QueryRowContext(ctx, sysID, sysID, q.key)
		gi, err := scanGame(row, sysID)
		if err == nil {
			return gi, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	return nil, nil
}

func scanGame(row *sql.Row, sysID int) (*GameInfo, error) {
	var gi GameInfo
	var (
		title, desc, dev, pub, genre, date sql.NullString
		coverF, coverB, region             sql.NullString
	)
	if err := row.Scan(&title, &desc, &dev, &pub, &genre, &date, &coverF, &coverB, &region); err != nil {
		return nil, err
	}
	gi.OpenVGDBID = sysID
	gi.Title = title.String
	gi.Description = desc.String
	gi.Developer = dev.String
	gi.Publisher = pub.String
	gi.Genre = genre.String
	gi.ReleaseDate = date.String
	gi.CoverURL = coverF.String
	gi.BackCoverURL = coverB.String
	gi.Region = region.String
	return &gi, nil
}

// extensionlessName matches OpenVGDB's `romExtensionlessFileName` column.
func extensionlessName(filename string) string {
	if filename == "" {
		return ""
	}
	base := filepath.Base(filename)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// -----------------------------------------------------------------------------
// Download / install
// -----------------------------------------------------------------------------

// Download fetches the upstream zip, extracts openvgdb.sqlite into the
// store's target path, and re-opens it. Atomically replaces the existing
// file (rename) so a failed download never leaves a half-baked DB.
func (s *Store) Download(ctx context.Context) error {
	return s.DownloadFrom(ctx, DefaultZipURL)
}

func (s *Store) DownloadFrom(ctx context.Context, zipURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, zipURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("openvgdb: GET %s → %s", zipURL, res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 200<<20))
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return fmt.Errorf("openvgdb: unzip: %w", err)
	}
	var sqlite *zip.File
	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".sqlite") &&
			!strings.HasPrefix(filepath.Base(f.Name), "._") {
			sqlite = f
			break
		}
	}
	if sqlite == nil {
		return errors.New("openvgdb: no .sqlite file in archive")
	}
	rc, err := sqlite.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}

	// Close the old handle (if any), swap the file, and re-open while
	// holding the write lock so concurrent Lookups can't catch the store
	// mid-swap with a nil db.
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		_ = s.db.Close()
		s.db = nil
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return err
	}
	return s.openLocked()
}

// openLocked opens the SQLite file and prepares statements. Caller must
// hold s.mu (write lock). Open() takes the lock before calling this.
func (s *Store) openLocked() error {
	dsn := fmt.Sprintf("file:%s?mode=ro&immutable=1&_query_only=true", s.path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return err
	}
	s.db = db
	prep := func(q string) (*sql.Stmt, error) { return db.Prepare(q) }
	const selectCols = `
		R.releaseTitleName, R.releaseDescription, R.releaseDeveloper,
		R.releasePublisher, R.releaseGenre, R.releaseDate,
		R.releaseCoverFront, R.releaseCoverBack,
		COALESCE(Reg.regionName, '')
	`
	if s.stmtCRC, err = prep(`
		SELECT ` + selectCols + ` FROM ROMs Ro
		JOIN RELEASES R ON R.romID = Ro.romID
		LEFT JOIN REGIONS Reg ON Reg.regionID = Ro.regionID
		WHERE (? = 0 OR Ro.systemID = ?) AND Ro.romHashCRC = ?
		LIMIT 1
	`); err != nil {
		return err
	}
	if s.stmtMD5, err = prep(`
		SELECT ` + selectCols + ` FROM ROMs Ro
		JOIN RELEASES R ON R.romID = Ro.romID
		LEFT JOIN REGIONS Reg ON Reg.regionID = Ro.regionID
		WHERE (? = 0 OR Ro.systemID = ?) AND UPPER(Ro.romHashMD5) = UPPER(?)
		LIMIT 1
	`); err != nil {
		return err
	}
	if s.stmtSHA1, err = prep(`
		SELECT ` + selectCols + ` FROM ROMs Ro
		JOIN RELEASES R ON R.romID = Ro.romID
		LEFT JOIN REGIONS Reg ON Reg.regionID = Ro.regionID
		WHERE (? = 0 OR Ro.systemID = ?) AND UPPER(Ro.romHashSHA1) = UPPER(?)
		LIMIT 1
	`); err != nil {
		return err
	}
	if s.stmtName, err = prep(`
		SELECT ` + selectCols + ` FROM ROMs Ro
		JOIN RELEASES R ON R.romID = Ro.romID
		LEFT JOIN REGIONS Reg ON Reg.regionID = Ro.regionID
		WHERE (? = 0 OR Ro.systemID = ?) AND Ro.romExtensionlessFileName = ?
		LIMIT 1
	`); err != nil {
		return err
	}
	return nil
}
