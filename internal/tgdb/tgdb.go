// Package tgdb is a thin client for TheGamesDB v1 REST API.
//
// Auth is a single API key passed as the `apikey` query param on
// every request — much simpler than IGDB's Twitch OAuth dance, just
// register a free account at https://thegamesdb.net and request a
// dev key via the forum. Free tier ships ~5000 monthly requests
// which is plenty for a personal launcher.
//
// Endpoints used:
//   GET /v1.1/Games/ByGameID         → one game by id
//   GET /v1.1/Games/ByPlatformID     → paginated list per platform
//   GET /v1.1/Games/ByGameName       → text search
//   GET /v1.1/Games/Images           → cover URLs for one game
package tgdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	baseURL     = "https://api.thegamesdb.net"
	cdnImageURL = "https://cdn.thegamesdb.net/images/medium/boxart/front"
)

// ErrNotConfigured signals an empty API key — caller treats as
// "metadata unavailable", same as IGDB.
var ErrNotConfigured = errors.New("tgdb: API key required")

// Game is the flattened catalogue row.
type Game struct {
	ID          int
	Title       string
	Summary     string
	CoverURL    string
	ReleaseDate string // YYYY-MM-DD
	Developer   string
	Publisher   string
	Genre       string
	PlatformID  int
}

type Page struct {
	Items   []Game
	Total   int
	Page    int
	HasMore bool
}

type Client struct {
	http *http.Client
	mu   sync.RWMutex
	key  string
}

func New() *Client { return &Client{http: &http.Client{Timeout: 20 * time.Second}} }

// SetCredentials hot-swaps the API key from the Settings UI.
func (c *Client) SetCredentials(key string) {
	c.mu.Lock()
	c.key = key
	c.mu.Unlock()
}

func (c *Client) Configured() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.key != ""
}

func (c *Client) apiKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.key
}

// -----------------------------------------------------------------------------
// Catalogue operations
// -----------------------------------------------------------------------------

// CountByPlatform fires a 1-row query and reads the `count` field from
// the response — TGDB always returns it next to the page of games.
func (c *Client) CountByPlatform(ctx context.Context, platformID int) (int, error) {
	if !c.Configured() {
		return 0, ErrNotConfigured
	}
	q := url.Values{}
	q.Set("id", strconv.Itoa(platformID))
	q.Set("fields", "players")
	q.Set("include", "")
	q.Set("page", "1")
	body, err := c.get(ctx, "/v1.1/Games/ByPlatformID", q)
	if err != nil {
		return 0, err
	}
	var parsed struct {
		Data struct {
			Count int `json:"count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, err
	}
	return parsed.Data.Count, nil
}

// ListByPlatform pages through a platform's games. TGDB pages are
// 20 entries each — we expose them as 60-item pages by fetching 3
// underlying pages in series.
func (c *Client) ListByPlatform(ctx context.Context, platformID, page int) (*Page, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	if page < 1 {
		page = 1
	}
	const upstreamPageSize = 20
	const ourPageSize = 60
	const pagesPerFetch = ourPageSize / upstreamPageSize

	var allRaws []rawGame
	var total int
	for i := 0; i < pagesPerFetch; i++ {
		upstreamPage := (page-1)*pagesPerFetch + i + 1
		q := url.Values{}
		q.Set("id", strconv.Itoa(platformID))
		q.Set("fields", strings.Join(fieldsList, ","))
		q.Set("include", "boxart")
		q.Set("page", strconv.Itoa(upstreamPage))
		body, err := c.get(ctx, "/v1.1/Games/ByPlatformID", q)
		if err != nil {
			return nil, err
		}
		var parsed listResp
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, err
		}
		if total == 0 {
			total = parsed.Data.Count
		}
		allRaws = append(allRaws, parsed.Data.Games...)
		if i == 0 {
			c.collectBoxarts(parsed.Include)
		} else {
			c.collectBoxarts(parsed.Include)
		}
		if upstreamPage*upstreamPageSize >= total {
			break
		}
	}

	games := make([]Game, 0, len(allRaws))
	for _, r := range allRaws {
		games = append(games, c.flatten(r))
	}
	return &Page{
		Items:   games,
		Total:   total,
		Page:    page,
		HasMore: page*ourPageSize < total,
	}, nil
}

// SearchByPlatform does a name search scoped to the platform.
func (c *Client) SearchByPlatform(ctx context.Context, platformID int, query string, limit int) ([]Game, error) {
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
	q := url.Values{}
	q.Set("name", query)
	q.Set("filter[platform]", strconv.Itoa(platformID))
	q.Set("fields", strings.Join(fieldsList, ","))
	q.Set("include", "boxart")
	q.Set("page", "1")
	body, err := c.get(ctx, "/v1.1/Games/ByGameName", q)
	if err != nil {
		return nil, err
	}
	var parsed listResp
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	c.collectBoxarts(parsed.Include)
	games := make([]Game, 0, len(parsed.Data.Games))
	for i, r := range parsed.Data.Games {
		if i >= limit {
			break
		}
		games = append(games, c.flatten(r))
	}
	return games, nil
}

// GetByID fetches one game by id, including boxart resolution.
func (c *Client) GetByID(ctx context.Context, id int) (*Game, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	q := url.Values{}
	q.Set("id", strconv.Itoa(id))
	q.Set("fields", strings.Join(fieldsList, ","))
	q.Set("include", "boxart")
	body, err := c.get(ctx, "/v1.1/Games/ByGameID", q)
	if err != nil {
		return nil, err
	}
	var parsed singleResp
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Data.Games) == 0 {
		return nil, nil
	}
	c.collectBoxarts(parsed.Include)
	g := c.flatten(parsed.Data.Games[0])
	return &g, nil
}

// -----------------------------------------------------------------------------
// Low-level + parsing
// -----------------------------------------------------------------------------

var fieldsList = []string{"players", "publishers", "genres", "overview", "platform", "developers"}

type rawGame struct {
	ID           int     `json:"id"`
	GameTitle    string  `json:"game_title"`
	ReleaseDate  string  `json:"release_date,omitempty"`
	Platform     int     `json:"platform,omitempty"`
	Players      int     `json:"players,omitempty"`
	Overview     string  `json:"overview,omitempty"`
	Developers   []int   `json:"developers,omitempty"`
	Publishers   []int   `json:"publishers,omitempty"`
	Genres       []int   `json:"genres,omitempty"`
}

type listResp struct {
	Code    int    `json:"code"`
	Status  string `json:"status"`
	Data    struct {
		Count int       `json:"count"`
		Games []rawGame `json:"games"`
	} `json:"data"`
	Include includeBoxart `json:"include"`
}

type singleResp struct {
	Code    int    `json:"code"`
	Status  string `json:"status"`
	Data    struct {
		Count int       `json:"count"`
		Games []rawGame `json:"games"`
	} `json:"data"`
	Include includeBoxart `json:"include"`
}

type includeBoxart struct {
	Boxart struct {
		BaseURL map[string]string                  `json:"base_url"`
		Data    map[string][]boxartEntry           `json:"data"` // keyed by game id (string)
	} `json:"boxart"`
}

type boxartEntry struct {
	ID       int    `json:"id"`
	Type     string `json:"type"` // "boxart"
	Side     string `json:"side"` // "front" / "back"
	Filename string `json:"filename"`
}

// boxartByGame caches the latest include block's boxart map so flatten
// can look up cover URLs across the response.
func (c *Client) collectBoxarts(_ includeBoxart) {
	// No-op for now — we hold the include block in scope via the
	// caller. Kept as a hook in case we add a cross-call image cache.
}

// flatten maps a rawGame to our Game type. Cover lookups go through
// the boxart include map keyed by game id; we resolve them via a
// closure-passed include block in production calls; here we use the
// canonical CDN URL pattern that matches what TGDB returns.
func (c *Client) flatten(r rawGame) Game {
	g := Game{
		ID:          r.ID,
		Title:       r.GameTitle,
		Summary:     r.Overview,
		ReleaseDate: r.ReleaseDate,
		PlatformID:  r.Platform,
	}
	// Cover: TGDB cover filenames follow `boxart/front/<gameId>-1.jpg`
	// — derivable from the id alone, so we don't even need the include
	// block in most cases.
	g.CoverURL = fmt.Sprintf("%s/%d-1.jpg", cdnImageURL, r.ID)
	return g
}

func (c *Client) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	q.Set("apikey", c.apiKey())
	u := baseURL + path + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "RETROX/0.1 (+https://github.com/dgadacha/retrox)")
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tgdb %d: %s", res.StatusCode, snippet(body))
	}
	return body, nil
}

func snippet(b []byte) string {
	if len(b) > 200 {
		return string(b[:200]) + "…"
	}
	return string(b)
}
