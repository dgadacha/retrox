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
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
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
