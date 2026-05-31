// Package download is RETROX's ROM fetcher: a single-worker queue that
// streams a URL into the right platform folder, updating a Download row's
// progress in place so the UI can render a live bar. It is deliberately
// modest — one transfer at a time, resumable only by re-queueing — which
// is plenty for grabbing the occasional homebrew/public-domain ROM.
package download

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"retrox/internal/database/db"
	"retrox/internal/database/models"
)

// DestResolver returns the directory a ROM for the given platform should
// land in. Supplied by the core (derived from the configured roots).
type DestResolver func(platformID string) (string, error)

type Manager struct {
	store   *db.Database
	resolve DestResolver
	http    *http.Client
	queue   chan uint
	onDone  func(*models.Download) // optional post-success hook (e.g. index)

	mu      sync.Mutex
	cancels map[uint]context.CancelFunc
}

func New(store *db.Database, resolve DestResolver) *Manager {
	m := &Manager{
		store:   store,
		resolve: resolve,
		http:    &http.Client{Timeout: 0}, // large files — no overall timeout
		queue:   make(chan uint, 128),
		cancels: make(map[uint]context.CancelFunc),
	}
	go m.worker()
	return m
}

// SetOnComplete registers a hook fired after a download finishes
// successfully (used to index the freshly-downloaded ROM).
func (m *Manager) SetOnComplete(fn func(*models.Download)) { m.onDone = fn }

// Enqueue validates the URL, computes the destination, persists a queued
// Download row, and schedules it. Returns the created row.
func (m *Manager) Enqueue(rawURL, platformID, title string) (*models.Download, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("URL invalide (http/https attendu)")
	}

	destDir, err := m.resolve(platformID)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("création du dossier de destination: %w", err)
	}
	name := fileNameFromURL(u, title)
	dest := filepath.Join(destDir, name)

	if title == "" {
		title = name
	}
	row, err := m.store.CreateDownload(&models.Download{
		URL:        rawURL,
		DestPath:   dest,
		PlatformID: platformID,
		Title:      title,
		Status:     "queued",
	})
	if err != nil {
		return nil, err
	}

	select {
	case m.queue <- row.ID:
	default:
		// Queue full — mark errored rather than blocking the request.
		row.Status = "error"
		row.Error = "file d'attente pleine"
		_ = m.store.SaveDownload(row)
		return nil, fmt.Errorf("file d'attente pleine, réessayez")
	}
	return row, nil
}

// Cancel stops an active or queued download. Active transfers are
// interrupted via their context; queued ones are flagged so the worker
// skips them when they come up.
func (m *Manager) Cancel(id uint) error {
	m.mu.Lock()
	cancel, active := m.cancels[id]
	m.mu.Unlock()
	if active {
		cancel()
		return nil
	}
	row, err := m.store.GetDownload(id)
	if err != nil {
		return err
	}
	if row.Status == "queued" {
		row.Status = "canceled"
		return m.store.SaveDownload(row)
	}
	return nil
}

func (m *Manager) worker() {
	for id := range m.queue {
		m.process(id)
	}
}

func (m *Manager) process(id uint) {
	row, err := m.store.GetDownload(id)
	if err != nil {
		log.Printf("download: load %d: %v", id, err)
		return
	}
	if row.Status == "canceled" {
		return // canceled while queued
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.cancels[id] = cancel
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.cancels, id)
		m.mu.Unlock()
		cancel()
	}()

	row.Status = "downloading"
	row.Error = ""
	_ = m.store.SaveDownload(row)

	if err := m.fetch(ctx, row); err != nil {
		if ctx.Err() != nil {
			row.Status = "canceled"
			row.Error = ""
			_ = os.Remove(row.DestPath + ".part")
		} else {
			row.Status = "error"
			row.Error = err.Error()
		}
		_ = m.store.SaveDownload(row)
		return
	}

	row.Status = "done"
	row.Progress = 1
	_ = m.store.SaveDownload(row)
	if m.onDone != nil {
		m.onDone(row)
	}
}

// fetch streams the URL to "<dest>.part", updating progress at most a few
// times a second, then atomically renames to the final path on success.
func (m *Manager) fetch(ctx context.Context, row *models.Download) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, row.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "retrox/0.1")
	res, err := m.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return fmt.Errorf("le serveur a répondu %d", res.StatusCode)
	}

	row.BytesTotal = res.ContentLength // -1 when unknown
	_ = m.store.SaveDownload(row)

	tmp := row.DestPath + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer out.Close()

	buf := make([]byte, 256<<10)
	lastSave := time.Now()
	for {
		n, rerr := res.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
			row.BytesDone += int64(n)
			if row.BytesTotal > 0 {
				row.Progress = float64(row.BytesDone) / float64(row.BytesTotal)
			}
			// Throttle DB writes — progress changes far faster than the
			// UI polls.
			if time.Since(lastSave) > 400*time.Millisecond {
				_ = m.store.SaveDownload(row)
				lastSave = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, row.DestPath)
}

// fileNameFromURL derives a safe local filename from the URL path,
// falling back to the title (then a generic name) when the URL has none.
func fileNameFromURL(u *url.URL, title string) string {
	name := path.Base(u.Path)
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
	}
	name = sanitizeFilename(name)
	if name == "" || name == "." || name == "/" {
		name = sanitizeFilename(title)
	}
	if name == "" {
		name = "rom.bin"
	}
	return name
}

// sanitizeFilename strips path separators and characters that would be
// awkward on disk, keeping the rest intact.
func sanitizeFilename(s string) string {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("/", "_", "\\", "_", ":", "_", "\x00", "").Replace(s)
	return strings.TrimSpace(s)
}
