// Package sources exposes a uniform interface over remote ROM catalogs
// RETROX can search for a specific game and hand-off the download to
// the existing manager. Concrete adapters live in sibling files
// (archiveorg.go, pdroms.go); registering a new one is just: write an
// adapter, append it to App.Sources in core/app.go.
//
// The user-facing flow is catalogue-driven: the UI browses OpenVGDB's
// 53k releases, the user picks one, then the backend asks every Source
// "do you have <title> for <platform>?" and surfaces the answers as a
// pick-list of candidates. Downloadable sources return a direct URL
// from Resolve(); link-only sources (RSS, paywall, etc.) return
// ErrNotDownloadable and the UI shows an "open external" button.
package sources

import "context"

// ROM is one candidate returned by Search. PlatformID is the caller's
// platform id passed back unchanged so the UI can route the download.
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

// Source is one concrete remote catalog. Implementations must be safe
// for concurrent calls (the aggregation endpoint searches every source
// in parallel from each request).
type Source interface {
	ID() string
	Name() string
	Description() string
	Downloadable() bool
	SupportedPlatforms() []string // our platform IDs

	// Search returns up to a handful of candidates matching the given
	// game title for the given platform. Empty title or unsupported
	// platform should return (nil, nil) — not an error.
	Search(ctx context.Context, title, platformID string) ([]ROM, error)

	// Resolve returns a direct download URL for romID. Returns
	// ErrNotDownloadable when the source can't hand off a URL (the UI
	// hides the Download button in that case; this is the safety net).
	Resolve(ctx context.Context, romID string) (string, error)
}

// ErrNotDownloadable signals that the source surfaces ROMs but can't
// hand them off as direct URLs (e.g. PDRoms, where the user has to
// follow the article link to find the actual file).
type errNotDownloadable struct{}

func (errNotDownloadable) Error() string { return "source: not downloadable" }

var ErrNotDownloadable error = errNotDownloadable{}

// Info is the descriptor returned by the /sources list endpoint so the
// UI knows what's available without introspecting each one.
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

// AggregatedResult is one source's slice of the response when the
// catalog handler fans out a Search across every registered source.
type AggregatedResult struct {
	Source Info  `json:"source"`
	Items  []ROM `json:"items"`
	Error  string `json:"error,omitempty"`
}
