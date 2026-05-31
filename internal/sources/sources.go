// Package sources exposes a uniform interface over remote ROM catalogs
// that RETROX can browse from inside the UI. Concrete adapters live in
// sibling files (archiveorg.go, pdroms.go). The package contains no I/O
// of its own — adapters do their own HTTP — so registering a new source
// is just: write an adapter, append it to App.Sources in core/app.go.
//
// Capabilities:
//   - Browseable adapters (Archive.org, PDRoms) implement Browse().
//   - Downloadable adapters (Archive.org) also implement Resolve(),
//     returning a direct URL the download manager can fetch. Link-only
//     adapters (PDRoms, where articles no longer expose the file)
//     return Downloadable() == false; the UI shows an "Open in source"
//     button instead.
package sources

import "context"

// ROM is one entry surfaced by a Source. PlatformID is mapped to our
// internal platform catalog so the UI can route the download into the
// right ./roms/<platform>/ folder.
type ROM struct {
	SourceID     string `json:"sourceId"`
	ID           string `json:"id"`
	Title        string `json:"title"`
	PlatformID   string `json:"platformId"`
	Description  string `json:"description,omitempty"`
	CoverURL     string `json:"coverUrl,omitempty"`
	SizeBytes    int64  `json:"sizeBytes,omitempty"`
	Downloadable bool   `json:"downloadable"`
	ExternalURL  string `json:"externalUrl"`
}

// Page is one slice of results from Browse(). HasMore + NextPage drive
// the UI's "load more" pagination.
type Page struct {
	Items    []ROM `json:"items"`
	HasMore  bool  `json:"hasMore"`
	NextPage int   `json:"nextPage"`
}

// BrowseOptions filters a Browse() call. Empty Query means "list",
// non-empty Query means "search". Page is 1-based.
type BrowseOptions struct {
	PlatformID string
	Query      string
	Page       int
}

// Source is one concrete remote catalog. Implementations must be
// goroutine-safe (handlers may call Browse + Resolve concurrently from
// different requests).
type Source interface {
	ID() string
	Name() string
	Description() string
	Downloadable() bool
	SupportedPlatforms() []string // our platform IDs

	Browse(ctx context.Context, opts BrowseOptions) (*Page, error)
	// Resolve returns a direct download URL for romID. Returns
	// (string, ErrNotDownloadable) when the source can't hand off a
	// URL (the UI should already hide the Download button in that
	// case; this is the belt-and-braces check).
	Resolve(ctx context.Context, romID string) (string, error)
}

// ErrNotDownloadable signals that the source surfaces ROMs but can't
// hand them off as direct URLs (e.g. PDRoms RSS, where the user has
// to follow the article link to find the actual file).
type errNotDownloadable struct{}

func (errNotDownloadable) Error() string { return "source: not downloadable" }

var ErrNotDownloadable error = errNotDownloadable{}

// Info is the descriptor returned by the /sources list endpoint so the
// UI knows what's available without having to introspect each one.
type Info struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Downloadable       bool     `json:"downloadable"`
	SupportedPlatforms []string `json:"supportedPlatforms"`
}

func InfoFrom(s Source) Info {
	return Info{
		ID:                 s.ID(),
		Name:               s.Name(),
		Description:        s.Description(),
		Downloadable:       s.Downloadable(),
		SupportedPlatforms: s.SupportedPlatforms(),
	}
}
