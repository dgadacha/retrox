// Package libretrothumbs resolves cover / screenshot / title-screen art
// for a ROM by hitting the public libretro-thumbnails CDN.
//
// The CDN at https://thumbnails.libretro.com hosts three categories per
// system, each indexed by the No-Intro filename (without extension):
//
//	Named_Boxarts/<rom>.png  → box art (preferred for "cover")
//	Named_Snaps/<rom>.png    → in-game screenshot
//	Named_Titles/<rom>.png   → title screen
//
// Index of available systems: https://github.com/libretro-thumbnails
// — each repo there matches one folder name on the CDN. Some No-Intro
// characters (`&`, `*`) get sanitized when the repos are built, so we
// apply the same sanitization client-side before HEAD-ing the URL.
package libretrothumbs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const baseURL = "https://thumbnails.libretro.com"

// Kind picks which subfolder on the CDN to look in.
type Kind string

const (
	Boxart     Kind = "Named_Boxarts"
	Screenshot Kind = "Named_Snaps"
	Title      Kind = "Named_Titles"
)

// Client HEADs the CDN to check whether art exists before handing the
// URL back. A short timeout keeps a missing image from blocking a scan.
type Client struct {
	http *http.Client
}

func NewClient() *Client {
	return &Client{
		http: &http.Client{
			Timeout: 8 * time.Second,
			// Don't auto-follow — if the CDN ever redirects we want to
			// know rather than silently fetch from somewhere else.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// URL builds the CDN URL for a (system, kind, romName) triple without
// hitting the network. Returns "" when system or romName is empty.
func URL(libretroSystem, romFilename string, kind Kind) string {
	if libretroSystem == "" || romFilename == "" {
		return ""
	}
	base := strings.TrimSuffix(filepath.Base(romFilename), filepath.Ext(romFilename))
	if base == "" {
		return ""
	}
	clean := sanitize(base)
	return baseURL + "/" +
		url.PathEscape(libretroSystem) + "/" +
		string(kind) + "/" +
		url.PathEscape(clean) + ".png"
}

// Exists HEADs the URL and returns true on 200. Any other status (404,
// 5xx) or a network error means "no art" — the caller falls through to
// the next preference.
func (c *Client) Exists(ctx context.Context, u string) bool {
	if u == "" {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
	if err != nil {
		return false
	}
	res, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode == http.StatusOK
}

// Find returns the first (kind, URL) pair that exists on the CDN in
// preference order (Boxart → Screenshot → Title). Empty kind+URL when
// none of them is found.
func (c *Client) Find(ctx context.Context, libretroSystem, romFilename string, kinds ...Kind) (Kind, string) {
	if len(kinds) == 0 {
		kinds = []Kind{Boxart, Screenshot, Title}
	}
	for _, k := range kinds {
		u := URL(libretroSystem, romFilename, k)
		if c.Exists(ctx, u) {
			return k, u
		}
	}
	return "", ""
}

// -----------------------------------------------------------------------------
// Directory listing cache (for fuzzy-matching catalogue titles)
// -----------------------------------------------------------------------------

// listingCache memoises the file listing of one Named_Boxarts folder
// in memory + on disk (7-day TTL). Building it requires walking the
// GitHub API which is rate-limited unauthed (60/hour), so the cache
// is essential.
type listingCache struct {
	mu       sync.Mutex
	memCache map[string][]string // system → list of base filenames (no .png)
	dir      string              // disk cache dir
}

var defaultListing = &listingCache{memCache: map[string][]string{}}

// SetCacheDir tells the lazy listing cache where to persist its
// per-platform JSON files. Call once at boot from core/app.go.
func SetCacheDir(d string) {
	defaultListing.mu.Lock()
	defaultListing.dir = d
	defaultListing.mu.Unlock()
}

// ListedNames returns every base filename (no extension) in the
// Named_Boxarts folder of one system, fetching via GitHub API on first
// call. Filenames are lower-cased for caller convenience.
func (c *Client) ListedNames(ctx context.Context, libretroSystem string) ([]string, error) {
	if libretroSystem == "" {
		return nil, nil
	}
	defaultListing.mu.Lock()
	if cached, ok := defaultListing.memCache[libretroSystem]; ok {
		defaultListing.mu.Unlock()
		return cached, nil
	}
	dir := defaultListing.dir
	defaultListing.mu.Unlock()

	// Disk cache lookup.
	if dir != "" {
		if names, ok := readListingFromDisk(dir, libretroSystem); ok {
			defaultListing.mu.Lock()
			defaultListing.memCache[libretroSystem] = names
			defaultListing.mu.Unlock()
			return names, nil
		}
	}

	// Cold path: GitHub API.
	names, err := fetchListing(ctx, libretroSystem)
	if err != nil {
		return nil, err
	}
	defaultListing.mu.Lock()
	defaultListing.memCache[libretroSystem] = names
	defaultListing.mu.Unlock()
	if dir != "" {
		_ = writeListingToDisk(dir, libretroSystem, names)
	}
	return names, nil
}

// MatchBoxart finds the best file in the system's Named_Boxarts folder
// whose lowercased prefix (up to the first parenthesis) equals the
// lowercased query. Returns the canonical CDN URL or "" when no match.
func (c *Client) MatchBoxart(ctx context.Context, libretroSystem, title string) string {
	title = strings.TrimSpace(title)
	if title == "" || libretroSystem == "" {
		return ""
	}
	names, err := c.ListedNames(ctx, libretroSystem)
	if err != nil || len(names) == 0 {
		return ""
	}
	needle := canonicalize(title)
	for _, n := range names {
		if canonicalize(n) == needle {
			return baseURL + "/" + url.PathEscape(libretroSystem) +
				"/" + string(Boxart) + "/" + url.PathEscape(n) + ".png"
		}
	}
	return ""
}

// canonicalize lowercases the string and trims the trailing
// "(region)" / "[!]" tags so "Super Mario World (USA)" and
// "Super Mario World" hash to the same key.
func canonicalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for {
		i := strings.LastIndexAny(s, "([")
		if i <= 0 || len(s)-i > 30 {
			break
		}
		s = strings.TrimSpace(s[:i])
	}
	return s
}

// fetchListing walks the GitHub Contents API (1000 entries/page) and
// returns every .png base filename in the Named_Boxarts folder.
func fetchListing(ctx context.Context, libretroSystem string) ([]string, error) {
	httpc := &http.Client{Timeout: 20 * time.Second}
	var out []string
	page := 1
	for ; page <= 10; page++ {
		u := fmt.Sprintf(
			"https://api.github.com/repos/libretro-thumbnails/%s/contents/Named_Boxarts?per_page=1000&page=%d",
			url.PathEscape(libretroSystem), page,
		)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		req.Header.Set("Accept", "application/vnd.github.v3+json")
		req.Header.Set("User-Agent", "RETROX/0.1")
		res, err := httpc.Do(req)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(io.LimitReader(res.Body, 16<<20))
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("libretro-thumbnails github %d", res.StatusCode)
		}
		var entries []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal(body, &entries); err != nil {
			return nil, err
		}
		if len(entries) == 0 {
			break
		}
		for _, e := range entries {
			if e.Type == "file" && strings.HasSuffix(e.Name, ".png") {
				out = append(out, strings.TrimSuffix(e.Name, ".png"))
			}
		}
		if len(entries) < 1000 {
			break
		}
	}
	return out, nil
}

type diskEntry struct {
	Names []string  `json:"names"`
	At    time.Time `json:"at"`
}

const diskTTL = 7 * 24 * time.Hour

func diskPath(dir, libretroSystem string) string {
	// Use the system name as filename, slashes replaced.
	safe := strings.ReplaceAll(libretroSystem, "/", "_")
	return filepath.Join(dir, safe+".json")
}

func readListingFromDisk(dir, libretroSystem string) ([]string, bool) {
	b, err := os.ReadFile(diskPath(dir, libretroSystem))
	if err != nil {
		return nil, false
	}
	var e diskEntry
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, false
	}
	if time.Since(e.At) > diskTTL {
		return nil, false
	}
	return e.Names, true
}

func writeListingToDisk(dir, libretroSystem string, names []string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(diskEntry{Names: names, At: time.Now()})
	if err != nil {
		return err
	}
	return os.WriteFile(diskPath(dir, libretroSystem), b, 0o644)
}

// sanitize matches the rule the libretro-thumbnails build pipeline
// applies when materializing repos from No-Intro DATs: the chars
// `&*/:<>?\|` aren't legal on every filesystem, so they're replaced with
// `_`. The CDN files therefore use the cleaned name.
//
// Source: https://github.com/libretro-thumbnails/libretro-thumbnails (build script).
func sanitize(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch r {
		case '&', '*', '/', ':', '<', '>', '?', '\\', '|', '"':
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
