// Package metadata is the scanner-facing facade that fuses the OpenVGDB
// text metadata with libretro-thumbnails cover art.
//
// Lookup order for a ROM:
//
//  1. OpenVGDB → title, description, genre, developer, publisher, date,
//     region, and (when present) a cover URL from gamefaqs.gamespot.com.
//  2. libretro-thumbnails → cover / screenshot / title-screen, used as
//     the cover when OpenVGDB had none, and always as the screenshot.
//
// Both sources are public and credential-free. OpenVGDB ships as a
// single SQLite file (downloaded once), libretro-thumbnails is a CDN.
package metadata

import (
	"context"

	"retrox/internal/libretrothumbs"
	"retrox/internal/openvgdb"
	"retrox/internal/platforms"
)

// GameInfo is the flattened, scanner-friendly result. CoverURL and
// ScreenshotURL are public http(s) URLs the image proxy can fetch
// without authentication.
type GameInfo struct {
	Title       string
	Description string
	Genre       string
	Developer   string
	Publisher   string
	ReleaseDate string
	Region      string
	CoverURL    string
	ScreenshotURL string
}

// Lookup carries everything the scanner knows about a ROM. The Provider
// fans the right fields out to each source.
type Lookup struct {
	PlatformID   string
	CRC32        string
	MD5          string
	SHA1         string
	FileName     string
	ROMSizeBytes int64
}

// Provider is the combined OpenVGDB + libretro-thumbnails metadata
// source the scanner depends on. Either piece may be nil — the Provider
// degrades gracefully (text-only or art-only) instead of erroring.
type Provider struct {
	openvgdb *openvgdb.Store
	thumbs   *libretrothumbs.Client
}

func New(ovgdb *openvgdb.Store, thumbs *libretrothumbs.Client) *Provider {
	return &Provider{openvgdb: ovgdb, thumbs: thumbs}
}

// Ready reports whether at least one source is usable. The handlers use
// this to expose a "metadata configured" flag on /status.
func (p *Provider) Ready() bool {
	if p == nil {
		return false
	}
	if p.openvgdb != nil && p.openvgdb.Ready() {
		return true
	}
	return p.thumbs != nil
}

// Lookup queries each source in priority order and merges the results.
// Returns (nil, nil) when nothing matched — caller records "tried, no
// match" without erroring.
func (p *Provider) Lookup(ctx context.Context, l Lookup) (*GameInfo, error) {
	if p == nil {
		return nil, nil
	}
	plat, ok := platforms.ByID(l.PlatformID)
	if !ok {
		return nil, nil
	}

	out := &GameInfo{}
	matched := false

	// 1. OpenVGDB
	if p.openvgdb != nil && p.openvgdb.Ready() {
		gi, err := p.openvgdb.Lookup(ctx, openvgdb.Lookup{
			OpenVGDBSystemID: plat.OpenVGDBID,
			CRC32:            l.CRC32,
			MD5:              l.MD5,
			SHA1:             l.SHA1,
			FileName:         l.FileName,
		})
		if err != nil {
			return nil, err
		}
		if gi != nil {
			matched = true
			out.Title = gi.Title
			out.Description = gi.Description
			out.Genre = gi.Genre
			out.Developer = gi.Developer
			out.Publisher = gi.Publisher
			out.ReleaseDate = gi.ReleaseDate
			out.Region = gi.Region
			out.CoverURL = gi.CoverURL
		}
	}

	// 2. libretro-thumbnails — always try for a screenshot, and as a
	// fallback when OpenVGDB had no cover (or no row at all).
	if p.thumbs != nil && plat.LibretroThumbsName != "" {
		if out.CoverURL == "" {
			if u := libretrothumbs.URL(plat.LibretroThumbsName, l.FileName, libretrothumbs.Boxart); u != "" && p.thumbs.Exists(ctx, u) {
				out.CoverURL = u
				matched = true
			}
		}
		if out.ScreenshotURL == "" {
			if u := libretrothumbs.URL(plat.LibretroThumbsName, l.FileName, libretrothumbs.Screenshot); u != "" && p.thumbs.Exists(ctx, u) {
				out.ScreenshotURL = u
				matched = true
			}
		}
	}

	if !matched {
		return nil, nil
	}
	return out, nil
}
