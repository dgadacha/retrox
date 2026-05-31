// Package igdb is a thin client for IGDB.com's REST API. It exists to
// fill the gaps OpenVGDB leaves: PS2, Dreamcast, Wii, Neo Geo, modern
// systems — anything No-Intro/Redump didn't ship a DAT for.
//
// IGDB auth piggybacks on Twitch: the app exchanges a client_id +
// client_secret for an OAuth token that's valid ~60 days, then attaches
// it (plus the Client-ID header) to every request. The token is cached
// in-memory and refreshed automatically when a request returns 401.
//
// The query language is Apicalypse — a Lucene-ish DSL the server reads
// from the POST body as text/plain. See https://api-docs.igdb.com.
package igdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	twitchTokenURL = "https://id.twitch.tv/oauth2/token"
	igdbBaseURL    = "https://api.igdb.com/v4"
	coverBase      = "https://images.igdb.com/igdb/image/upload"
)

// ErrNotConfigured signals that the user hasn't pasted their Twitch app
// credentials yet — callers treat it like "metadata unavailable".
var ErrNotConfigured = errors.New("igdb: client_id and client_secret are required")

// Game is the flattened IGDB row our catalogue consumes.
type Game struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Summary     string  `json:"summary,omitempty"`
	CoverURL    string  `json:"coverUrl,omitempty"`
	FirstYear   int     `json:"firstYear,omitempty"`
	Rating      float64 `json:"rating,omitempty"`     // 0..100
	Genre       string  `json:"genre,omitempty"`
	Company     string  `json:"company,omitempty"`
	PlatformID  int     `json:"platformId,omitempty"` // IGDB platform id
}

// Page is the paginated response shape.
type Page struct {
	Items   []Game `json:"items"`
	Total   int    `json:"total"`
	Page    int    `json:"page"`
	HasMore bool   `json:"hasMore"`
}

// Client wraps the OAuth bookkeeping and the REST calls.
type Client struct {
	http *http.Client

	mu          sync.RWMutex
	clientID    string
	clientSecret string

	tokenMu     sync.Mutex
	token       string
	tokenExpiry time.Time
}

func New() *Client {
	return &Client{http: &http.Client{Timeout: 20 * time.Second}}
}

// SetCredentials hot-swaps the OAuth keys from the settings UI. Clears
// the cached token so the next call re-authenticates.
func (c *Client) SetCredentials(clientID, clientSecret string) {
	c.mu.Lock()
	c.clientID = clientID
	c.clientSecret = clientSecret
	c.mu.Unlock()

	c.tokenMu.Lock()
	c.token = ""
	c.tokenExpiry = time.Time{}
	c.tokenMu.Unlock()
}

// Configured reports whether SetCredentials has been called with a
// non-empty pair.
func (c *Client) Configured() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clientID != "" && c.clientSecret != ""
}

// -----------------------------------------------------------------------------
// Catalog operations
// -----------------------------------------------------------------------------

// CountByPlatform returns how many IGDB games exist for the platform.
// Used by the catalogue picker to badge tile counts.
func (c *Client) CountByPlatform(ctx context.Context, igdbPlatformID int) (int, error) {
	if !c.Configured() {
		return 0, ErrNotConfigured
	}
	body := fmt.Sprintf("where platforms = %d & version_parent = null;", igdbPlatformID)
	resp, err := c.post(ctx, "/games/count", body)
	if err != nil {
		return 0, err
	}
	var parsed struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return 0, err
	}
	return parsed.Count, nil
}

// ListByPlatform pages through a platform's games sorted by name.
// `version_parent = null` filters out re-release/port variants so the
// catalogue shows the canonical entry (analogous to OpenVGDB dedup).
func (c *Client) ListByPlatform(ctx context.Context, igdbPlatformID, page, pageSize int) (*Page, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 60
	}
	total, err := c.CountByPlatform(ctx, igdbPlatformID)
	if err != nil {
		return nil, err
	}
	body := fmt.Sprintf(
		gameFields+
			" where platforms = %d & version_parent = null;"+
			" sort name asc;"+
			" limit %d; offset %d;",
		igdbPlatformID, pageSize, (page-1)*pageSize,
	)
	games, err := c.queryGames(ctx, body)
	if err != nil {
		return nil, err
	}
	return &Page{
		Items:   games,
		Total:   total,
		Page:    page,
		HasMore: page*pageSize < total,
	}, nil
}

// SearchByPlatform fires an IGDB `search` query, scoped to the platform.
// IGDB's full-text search is more forgiving than our OpenVGDB LIKE so
// "mario world" finds "Super Mario World" out of the box.
func (c *Client) SearchByPlatform(ctx context.Context, igdbPlatformID int, query string, limit int) ([]Game, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 30
	}
	// IGDB's `search` clause is incompatible with sort/where=platforms,
	// so we filter platforms in a second where clause and rely on the
	// search's own relevance ranking.
	body := fmt.Sprintf(
		gameFields+
			" search %q;"+
			" where platforms = %d & version_parent = null;"+
			" limit %d;",
		query, igdbPlatformID, limit,
	)
	return c.queryGames(ctx, body)
}

// GetByID returns one game by its IGDB id.
func (c *Client) GetByID(ctx context.Context, id int) (*Game, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	body := fmt.Sprintf(gameFields+" where id = %d; limit 1;", id)
	games, err := c.queryGames(ctx, body)
	if err != nil {
		return nil, err
	}
	if len(games) == 0 {
		return nil, nil
	}
	return &games[0], nil
}

// gameFields is the projection we request — covers everything the
// catalog page needs while staying under IGDB's 500-row max body.
const gameFields = `fields
	name, summary, first_release_date, total_rating,
	cover.image_id,
	genres.name,
	involved_companies.company.name, involved_companies.developer, involved_companies.publisher,
	platforms;`

// -----------------------------------------------------------------------------
// Low-level: OAuth + POST
// -----------------------------------------------------------------------------

func (c *Client) post(ctx context.Context, path, body string) ([]byte, error) {
	doOnce := func() (*http.Response, []byte, error) {
		token, err := c.accessToken(ctx)
		if err != nil {
			return nil, nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, igdbBaseURL+path, strings.NewReader(body))
		if err != nil {
			return nil, nil, err
		}
		c.mu.RLock()
		req.Header.Set("Client-ID", c.clientID)
		c.mu.RUnlock()
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("Accept", "application/json")
		res, err := c.http.Do(req)
		if err != nil {
			return nil, nil, err
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		return res, b, nil
	}

	res, b, err := doOnce()
	if err != nil {
		return nil, err
	}
	// Stale token → invalidate and retry once.
	if res.StatusCode == http.StatusUnauthorized {
		c.tokenMu.Lock()
		c.token = ""
		c.tokenMu.Unlock()
		res, b, err = doOnce()
		if err != nil {
			return nil, err
		}
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("igdb %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return b, nil
}

// accessToken returns a cached OAuth token, refreshing it if missing or
// within 5 minutes of expiry. Goroutine-safe.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.token != "" && time.Until(c.tokenExpiry) > 5*time.Minute {
		return c.token, nil
	}

	c.mu.RLock()
	id, secret := c.clientID, c.clientSecret
	c.mu.RUnlock()
	if id == "" || secret == "" {
		return "", ErrNotConfigured
	}

	form := url.Values{}
	form.Set("client_id", id)
	form.Set("client_secret", secret)
	form.Set("grant_type", "client_credentials")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, twitchTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("twitch oauth %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(b, &tok); err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		return "", errors.New("igdb: empty access_token from twitch")
	}
	c.token = tok.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	return c.token, nil
}

// -----------------------------------------------------------------------------
// Response parsing
// -----------------------------------------------------------------------------

type rawGame struct {
	ID                int         `json:"id"`
	Name              string      `json:"name"`
	Summary           string      `json:"summary,omitempty"`
	FirstReleaseDate  int64       `json:"first_release_date,omitempty"` // unix seconds
	TotalRating       float64     `json:"total_rating,omitempty"`
	Cover             *rawCover   `json:"cover,omitempty"`
	Genres            []rawNamed  `json:"genres,omitempty"`
	InvolvedCompanies []rawIC     `json:"involved_companies,omitempty"`
	Platforms         []int       `json:"platforms,omitempty"`
}
type rawCover struct {
	ImageID string `json:"image_id"`
}
type rawNamed struct {
	Name string `json:"name"`
}
type rawIC struct {
	Company   rawNamed `json:"company"`
	Developer bool     `json:"developer"`
	Publisher bool     `json:"publisher"`
}

func (c *Client) queryGames(ctx context.Context, body string) ([]Game, error) {
	raw, err := c.post(ctx, "/games", body)
	if err != nil {
		return nil, err
	}
	var rows []rawGame
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	out := make([]Game, 0, len(rows))
	for _, r := range rows {
		g := Game{
			ID:      r.ID,
			Name:    r.Name,
			Summary: r.Summary,
			Rating:  r.TotalRating,
		}
		if r.Cover != nil && r.Cover.ImageID != "" {
			g.CoverURL = fmt.Sprintf("%s/t_cover_big/%s.jpg", coverBase, r.Cover.ImageID)
		}
		if r.FirstReleaseDate > 0 {
			g.FirstYear = time.Unix(r.FirstReleaseDate, 0).UTC().Year()
		}
		if len(r.Genres) > 0 {
			names := make([]string, 0, len(r.Genres))
			for _, n := range r.Genres {
				names = append(names, n.Name)
			}
			g.Genre = strings.Join(names, ", ")
		}
		// Prefer developer, fall back to publisher when none.
		for _, ic := range r.InvolvedCompanies {
			if ic.Developer && ic.Company.Name != "" {
				g.Company = ic.Company.Name
				break
			}
		}
		if g.Company == "" {
			for _, ic := range r.InvolvedCompanies {
				if ic.Publisher && ic.Company.Name != "" {
					g.Company = ic.Company.Name
					break
				}
			}
		}
		// Take the first platform — for our usage queries always scope
		// to one platform so this is unambiguous.
		if len(r.Platforms) > 0 {
			g.PlatformID = r.Platforms[0]
		}
		out = append(out, g)
	}
	return out, nil
}
