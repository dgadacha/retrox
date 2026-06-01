// Package rawg is a thin client for the RAWG.io REST API.
//
// RAWG is the friendliest of the modern game DBs to integrate: just an
// email signup at rawg.io → an API key is visible immediately in the
// user's profile. 20k free requests per month, no OAuth, no forum
// approval dance. Coverage is broad (500k+ games, modern + retro) and
// the JSON shape is straightforward.
//
// The slightly awkward bit is platform IDs — they're idiosyncratic and
// not documented in a single canonical list, so we lazy-load them via
// /platforms on first use and resolve our internal platform IDs from
// the platform names. The mapping is cached in-memory.
package rawg

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

const baseURL = "https://api.rawg.io/api"

var ErrNotConfigured = errors.New("rawg: API key required")

// Game is the flattened catalogue row our handlers consume.
type Game struct {
	ID          int
	Slug        string
	Title       string
	Summary     string
	CoverURL    string
	ReleaseDate string // YYYY-MM-DD
	Genre       string
	Developer   string
	Publisher   string
	Rating      float64 // 0..5
	PlatformID  int     // RAWG platform id
}

type Page struct {
	Items   []Game
	Total   int
	Page    int
	HasMore bool
}

type Client struct {
	http *http.Client

	mu  sync.RWMutex
	key string

	// platformsCache maps a name fragment → RAWG platform id. Filled
	// on first use; refreshed on any 401/key-change.
	pMu       sync.Mutex
	platforms map[string]int // normalized name → id
}

func New() *Client {
	return &Client{http: &http.Client{Timeout: 20 * time.Second}}
}

// SetCredentials hot-swaps the API key and invalidates the platforms cache.
func (c *Client) SetCredentials(key string) {
	c.mu.Lock()
	c.key = key
	c.mu.Unlock()
	c.pMu.Lock()
	c.platforms = nil
	c.pMu.Unlock()
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
// Platform resolution
// -----------------------------------------------------------------------------

// PlatformIDForName resolves our human-friendly platform name (e.g.
// "Super Nintendo") to RAWG's internal numeric id. Returns 0 when no
// reasonable match. Loads the full platforms list on first call.
func (c *Client) PlatformIDForName(ctx context.Context, names ...string) (int, error) {
	if !c.Configured() {
		return 0, ErrNotConfigured
	}
	if err := c.ensurePlatforms(ctx); err != nil {
		return 0, err
	}
	c.pMu.Lock()
	defer c.pMu.Unlock()
	for _, n := range names {
		if id, ok := c.platforms[normalize(n)]; ok {
			return id, nil
		}
	}
	return 0, nil
}

func (c *Client) ensurePlatforms(ctx context.Context) error {
	c.pMu.Lock()
	loaded := c.platforms != nil
	c.pMu.Unlock()
	if loaded {
		return nil
	}
	// Walk every page until exhausted (RAWG returns ~6 pages with default page_size).
	mapping := map[string]int{}
	page := 1
	for page > 0 && page <= 10 {
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("page_size", "50")
		body, err := c.get(ctx, "/platforms", q)
		if err != nil {
			return err
		}
		var parsed struct {
			Count   int    `json:"count"`
			Next    string `json:"next"`
			Results []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
				Slug string `json:"slug"`
			} `json:"results"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return err
		}
		for _, p := range parsed.Results {
			mapping[normalize(p.Name)] = p.ID
			mapping[normalize(p.Slug)] = p.ID
		}
		if parsed.Next == "" {
			break
		}
		page++
	}
	c.pMu.Lock()
	c.platforms = mapping
	c.pMu.Unlock()
	return nil
}

// -----------------------------------------------------------------------------
// Catalogue operations
// -----------------------------------------------------------------------------

// CountByPlatform returns total games for a RAWG platform id. We piggy
// back on the regular /games endpoint and read the `count` field.
func (c *Client) CountByPlatform(ctx context.Context, platformID int) (int, error) {
	if !c.Configured() {
		return 0, ErrNotConfigured
	}
	q := url.Values{}
	q.Set("platforms", strconv.Itoa(platformID))
	q.Set("page", "1")
	q.Set("page_size", "1")
	body, err := c.get(ctx, "/games", q)
	if err != nil {
		return 0, err
	}
	var parsed struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, err
	}
	return parsed.Count, nil
}

// ListByPlatform pages 60-at-a-time. RAWG allows up to 40 per page so
// we fetch two and concat.
func (c *Client) ListByPlatform(ctx context.Context, platformID, page int) (*Page, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	if page < 1 {
		page = 1
	}
	const ourPageSize = 60
	const upstreamPageSize = 40
	const pagesPerFetch = 2 // 40 + 40 → 80, trim to 60

	var (
		all   []rawGame
		total int
	)
	for i := 0; i < pagesPerFetch; i++ {
		upstreamPage := (page-1)*pagesPerFetch + i + 1
		q := url.Values{}
		q.Set("platforms", strconv.Itoa(platformID))
		q.Set("ordering", "name")
		q.Set("page", strconv.Itoa(upstreamPage))
		q.Set("page_size", strconv.Itoa(upstreamPageSize))
		body, err := c.get(ctx, "/games", q)
		if err != nil {
			return nil, err
		}
		var parsed gamesResp
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, err
		}
		if total == 0 {
			total = parsed.Count
		}
		all = append(all, parsed.Results...)
		if parsed.Next == "" {
			break
		}
	}
	if len(all) > ourPageSize {
		all = all[:ourPageSize]
	}
	games := make([]Game, 0, len(all))
	for _, r := range all {
		games = append(games, c.flatten(r, platformID))
	}
	return &Page{
		Items:   games,
		Total:   total,
		Page:    page,
		HasMore: page*ourPageSize < total,
	}, nil
}

// SearchByPlatform does a text search scoped to a platform.
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
	q.Set("platforms", strconv.Itoa(platformID))
	q.Set("search", query)
	q.Set("search_precise", "true")
	q.Set("page_size", strconv.Itoa(limit))
	body, err := c.get(ctx, "/games", q)
	if err != nil {
		return nil, err
	}
	var parsed gamesResp
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	games := make([]Game, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		games = append(games, c.flatten(r, platformID))
	}
	return games, nil
}

// GetByID fetches one game with full detail (description, genres,
// developers, publishers).
func (c *Client) GetByID(ctx context.Context, id int) (*Game, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	body, err := c.get(ctx, "/games/"+strconv.Itoa(id), nil)
	if err != nil {
		return nil, err
	}
	var r rawGameDetail
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	g := c.flatten(r.rawGame, 0)
	g.Summary = stripHTML(r.DescriptionRaw)
	if g.Summary == "" {
		g.Summary = stripHTML(r.Description)
	}
	if len(r.Developers) > 0 {
		g.Developer = r.Developers[0].Name
	}
	if len(r.Publishers) > 0 {
		g.Publisher = r.Publishers[0].Name
	}
	return &g, nil
}

// -----------------------------------------------------------------------------
// Low-level
// -----------------------------------------------------------------------------

type gamesResp struct {
	Count   int       `json:"count"`
	Next    string    `json:"next"`
	Results []rawGame `json:"results"`
}

type rawGame struct {
	ID              int     `json:"id"`
	Slug            string  `json:"slug"`
	Name            string  `json:"name"`
	Released        string  `json:"released,omitempty"`
	BackgroundImage string  `json:"background_image,omitempty"`
	Rating          float64 `json:"rating,omitempty"`
	Genres          []named `json:"genres,omitempty"`
}

type rawGameDetail struct {
	rawGame
	Description    string  `json:"description"`
	DescriptionRaw string  `json:"description_raw"`
	Developers     []named `json:"developers,omitempty"`
	Publishers     []named `json:"publishers,omitempty"`
}

type named struct {
	Name string `json:"name"`
}

func (c *Client) flatten(r rawGame, platformID int) Game {
	g := Game{
		ID:          r.ID,
		Slug:        r.Slug,
		Title:       r.Name,
		CoverURL:    r.BackgroundImage,
		ReleaseDate: r.Released,
		Rating:      r.Rating,
		PlatformID:  platformID,
	}
	if len(r.Genres) > 0 {
		names := make([]string, 0, len(r.Genres))
		for _, g := range r.Genres {
			names = append(names, g.Name)
		}
		g.Genre = strings.Join(names, ", ")
	}
	return g
}

func (c *Client) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("key", c.apiKey())
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
		return nil, fmt.Errorf("rawg %d: %s", res.StatusCode, snippet(body))
	}
	return body, nil
}

// normalize collapses platform name variants (cases, dashes, /).
func normalize(s string) string {
	s = strings.ToLower(s)
	repl := strings.NewReplacer(
		"-", " ", "_", " ", "/", " ", "(", "", ")", "",
		".", "", ",", "", "  ", " ",
	)
	s = repl.Replace(s)
	return strings.TrimSpace(s)
}

// stripHTML is a coarse cleaner for RAWG's `description` field (HTML)
// — adequate for inline display, not a full parser.
func stripHTML(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '<':
			depth++
		case '>':
			depth--
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func snippet(b []byte) string {
	if len(b) > 200 {
		return string(b[:200]) + "…"
	}
	return string(b)
}
