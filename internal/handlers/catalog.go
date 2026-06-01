package handlers

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"retrox/internal/igdb"
	"retrox/internal/openvgdb"
	"retrox/internal/platforms"
	"retrox/internal/rawg"
	"retrox/internal/sources"
	"retrox/internal/tgdb"

	"github.com/labstack/echo/v4"
)

// catalogue.go routes catalogue requests across two metadata backends:
//
//   * IGDB (Twitch-authed REST) — the user-visible default once
//     credentials are set up. Uniform quality, modern coverage (PS2,
//     Dreamcast, Wii, …), rich descriptions and box art.
//   * OpenVGDB (offline SQLite) — fallback for the platforms IGDB has
//     a credential issue with OR when the user hasn't pasted Twitch
//     creds yet. Also continues to power the library scanner's
//     hash-based ROM identification (see internal/scanner) — that's
//     IGDB's blind spot since it doesn't index by CRC/MD5/SHA1.
//
// Source picking happens per-platform and is exposed to the frontend
// through a string release id: "ovgdb:42" vs "igdb:1942", so the same
// /catalog/:id endpoint can serve both.

const (
	sourceOVGDB = "ovgdb"
	sourceIGDB  = "igdb"
	sourceTGDB  = "tgdb"
	sourceRAWG  = "rawg"
)

// rawgPlatformNames maps our platform id → the RAWG platform name(s)
// to try when resolving the numeric id at the API. Multiple variants
// because RAWG isn't always consistent (e.g. "Genesis" vs "Mega
// Drive"); we walk in order and take the first hit.
var rawgPlatformNames = map[string][]string{
	"nes":          {"Nintendo Entertainment System", "NES"},
	"snes":         {"SNES", "Super Nintendo Entertainment System", "Super Nintendo"},
	"n64":          {"Nintendo 64"},
	"gb":           {"Game Boy"},
	"gbc":          {"Game Boy Color"},
	"gba":          {"Game Boy Advance"},
	"nds":          {"Nintendo DS"},
	"gamecube":     {"GameCube", "Nintendo GameCube"},
	"wii":          {"Wii", "Nintendo Wii"},
	"mastersystem": {"SEGA Master System", "Master System"},
	"megadrive":    {"Genesis", "Sega Genesis", "Mega Drive"},
	"gamegear":     {"Game Gear"},
	"sega32x":      {"Sega 32X", "32X"},
	"saturn":       {"Sega Saturn", "Saturn"},
	"dreamcast":    {"Dreamcast", "Sega Dreamcast"},
	"psx":          {"PlayStation"},
	"ps2":          {"PlayStation 2"},
	"psp":          {"PlayStation Portable", "PSP"},
	"pcengine":     {"PC Engine", "TurboGrafx-16"},
	"neogeo":       {"Neo Geo"},
	"ngp":          {"Neo Geo Pocket"},
	"atari2600":    {"Atari 2600"},
	"atari7800":    {"Atari 7800"},
	"lynx":         {"Atari Lynx", "Lynx"},
	"wonderswan":   {"WonderSwan"},
	"arcade":       {"Arcade"},
}

// pickSource returns the metadata backend to use for one platform,
// honoring the user's Preference setting. "auto" walks the preferred
// order (IGDB > TGDB > OpenVGDB) and picks the first one that has
// data for the platform; explicit values force their choice (no
// fallback — the UI shows an error if that source isn't usable).
func (h *Handler) pickSource(p platforms.Platform) string {
	pref := h.App.Config.Metadata.Preference
	if pref == "" {
		pref = "rawg"
	}
	igdbReady := h.App.IGDB != nil && h.App.IGDB.Configured()
	tgdbReady := h.App.TGDB != nil && h.App.TGDB.Configured()
	rawgReady := h.App.RAWG != nil && h.App.RAWG.Configured()
	ovgdbReady := h.App.OpenVGDB != nil && h.App.OpenVGDB.Ready()
	hasRawgName := len(rawgPlatformNames[p.ID]) > 0

	// Try the explicit preference first.
	switch pref {
	case "rawg":
		if rawgReady && hasRawgName {
			return sourceRAWG
		}
	case "igdb":
		if igdbReady && p.IGDBID > 0 {
			return sourceIGDB
		}
	case "tgdb":
		if tgdbReady && p.TGDBID > 0 {
			return sourceTGDB
		}
	case "openvgdb":
		if ovgdbReady && p.OpenVGDBID > 0 {
			return sourceOVGDB
		}
	}

	// Either the user picked "auto" or their preferred source isn't
	// usable for this platform — walk the chain in priority order so
	// the catalogue is never silently empty when *something* could
	// answer.
	if rawgReady && hasRawgName {
		return sourceRAWG
	}
	if igdbReady && p.IGDBID > 0 {
		return sourceIGDB
	}
	if tgdbReady && p.TGDBID > 0 {
		return sourceTGDB
	}
	if ovgdbReady && p.OpenVGDBID > 0 {
		return sourceOVGDB
	}
	return ""
}

// rawgPlatformID resolves our platform's name candidates to RAWG's
// numeric id, caching nothing (the client itself caches the full
// /platforms list).
func (h *Handler) rawgPlatformID(ctx context.Context, ourPlatformID string) int {
	names := rawgPlatformNames[ourPlatformID]
	if len(names) == 0 || h.App.RAWG == nil {
		return 0
	}
	id, _ := h.App.RAWG.PlatformIDForName(ctx, names...)
	return id
}

type platformTile struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"`
	Count  int    `json:"count"`
}

// platformsCache memoises HandleCatalogPlatforms's response — N HTTP
// round-trips to RAWG/IGDB/TGDB make a fresh build expensive (1-2s),
// so we keep the result for 30 min keyed by App.SettingsVersion().
// A credential/preference change bumps the version → cache miss → fresh
// data on the very next request, no plumbing across packages needed.
type platformsCacheEntry struct {
	version int64
	tiles   []platformTile
	at      time.Time
}

var (
	platformsCacheMu sync.Mutex
	platformsCache   *platformsCacheEntry
)

// HandleCatalogPlatforms returns one tile per RETROX-supported platform
// that has any metadata available, picking the source per pickSource()
// (which honours the user's Preference). Cached for 30 min keyed by
// SettingsVersion so creds/preference changes invalidate instantly.
func (h *Handler) HandleCatalogPlatforms(c echo.Context) error {
	version := h.App.SettingsVersion()
	platformsCacheMu.Lock()
	if platformsCache != nil &&
		platformsCache.version == version &&
		time.Since(platformsCache.at) < 30*time.Minute {
		cached := platformsCache.tiles
		platformsCacheMu.Unlock()
		return RespondOK(c, cached)
	}
	platformsCacheMu.Unlock()

	ctx := c.Request().Context()
	ovgdbCounts := map[int]int{}
	if h.App.OpenVGDB != nil && h.App.OpenVGDB.Ready() {
		var ids []int
		for _, p := range platforms.All() {
			if p.OpenVGDBID > 0 {
				ids = append(ids, p.OpenVGDBID)
			}
		}
		cnt, err := h.App.OpenVGDB.CountReleasesByPlatform(ctx, ids)
		if err == nil {
			ovgdbCounts = cnt
		}
	}

	// Fan out the per-platform counts in parallel — 25 sequential RAWG
	// requests were ~5-10 s end-to-end; in parallel they finish in
	// ~one round-trip (~500 ms).
	type slot struct {
		tile platformTile
		ok   bool
	}
	all := platforms.All()
	slots := make([]slot, len(all))
	var wg sync.WaitGroup
	for i, p := range all {
		i, p := i, p
		src := h.pickSource(p)
		if src == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			var n int
			switch src {
			case sourceRAWG:
				id := h.rawgPlatformID(ctx, p.ID)
				if id > 0 {
					n, _ = h.App.RAWG.CountByPlatform(ctx, id)
				}
			case sourceIGDB:
				n, _ = h.App.IGDB.CountByPlatform(ctx, p.IGDBID)
			case sourceTGDB:
				n, _ = h.App.TGDB.CountByPlatform(ctx, p.TGDBID)
			case sourceOVGDB:
				n = ovgdbCounts[p.OpenVGDBID]
			}
			if n > 0 {
				slots[i] = slot{
					tile: platformTile{ID: p.ID, Name: p.Name, Source: src, Count: n},
					ok:   true,
				}
			}
		}()
	}
	wg.Wait()

	out := make([]platformTile, 0, len(slots))
	for _, s := range slots {
		if s.ok {
			out = append(out, s.tile)
		}
	}

	platformsCacheMu.Lock()
	platformsCache = &platformsCacheEntry{version: version, tiles: out, at: time.Now()}
	platformsCacheMu.Unlock()
	return RespondOK(c, out)
}

type taggedRelease struct {
	ReleaseID        string `json:"releaseId"` // "ovgdb:42" or "igdb:1942"
	Title            string `json:"title"`
	CoverURL         string `json:"coverUrl,omitempty"`
	SystemShortName  string `json:"systemShortName,omitempty"`
	Region           string `json:"region,omitempty"`
	PlatformID       string `json:"platformId"`
	VariantCount     int    `json:"variantCount,omitempty"`
	Source           string `json:"source"`
}

// HandleCatalogList pages through one platform's catalogue, picking the
// right backend per-platform.
func (h *Handler) HandleCatalogList(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	platformID := c.QueryParam("platform")
	query := c.QueryParam("q")

	p, ok := platforms.ByID(platformID)
	if !ok || platformID == "" {
		return RespondErr(c, http.StatusBadRequest, "plateforme requise")
	}

	switch h.pickSource(p) {
	case sourceRAWG:
		return h.listRAWG(c, p, query, page)
	case sourceIGDB:
		return h.listIGDB(c, p, query, page)
	case sourceTGDB:
		return h.listTGDB(c, p, query, page)
	case sourceOVGDB:
		return h.listOpenVGDB(c, p, query, page)
	}
	return RespondErr(c, http.StatusNotFound,
		"aucune source de métadonnées pour cette plateforme — configurez IGDB ou TheGamesDB dans Réglages")
}

func (h *Handler) listRAWG(c echo.Context, p platforms.Platform, query string, page int) error {
	ctx := c.Request().Context()
	rawgID := h.rawgPlatformID(ctx, p.ID)
	if rawgID == 0 {
		return RespondErr(c, http.StatusNotFound, "RAWG ne connaît pas cette plateforme")
	}
	var (
		games []rawg.Game
		total int
		err   error
	)
	if strings.TrimSpace(query) != "" {
		games, err = h.App.RAWG.SearchByPlatform(ctx, rawgID, query, 60)
		total = len(games)
	} else {
		out, lerr := h.App.RAWG.ListByPlatform(ctx, rawgID, page)
		err = lerr
		if out != nil {
			games = out.Items
			total = out.Total
		}
	}
	if err != nil {
		return RespondErr(c, http.StatusBadGateway, err.Error())
	}
	items := make([]taggedRelease, 0, len(games))
	for _, g := range games {
		items = append(items, taggedRelease{
			ReleaseID:  fmt.Sprintf("%s_%d", sourceRAWG, g.ID),
			Title:      g.Title,
			CoverURL:   g.CoverURL,
			PlatformID: p.ID,
			Source:     sourceRAWG,
		})
	}
	return RespondOK(c, map[string]any{
		"items": items, "total": total, "page": page, "hasMore": page*60 < total,
	})
}

func (h *Handler) listIGDB(c echo.Context, p platforms.Platform, query string, page int) error {
	var (
		games []igdb.Game
		total int
		err   error
	)
	if strings.TrimSpace(query) != "" {
		games, err = h.App.IGDB.SearchByPlatform(c.Request().Context(), p.IGDBID, query, 60)
		total = len(games)
	} else {
		out, lerr := h.App.IGDB.ListByPlatform(c.Request().Context(), p.IGDBID, page, 60)
		err = lerr
		if out != nil {
			games = out.Items
			total = out.Total
		}
	}
	if err != nil {
		return RespondErr(c, http.StatusBadGateway, err.Error())
	}
	items := make([]taggedRelease, 0, len(games))
	for _, g := range games {
		items = append(items, taggedRelease{
			ReleaseID:  fmt.Sprintf("%s_%d", sourceIGDB, g.ID),
			Title:      g.Name,
			CoverURL:   g.CoverURL,
			PlatformID: p.ID,
			Source:     sourceIGDB,
		})
	}
	return RespondOK(c, map[string]any{
		"items": items, "total": total, "page": page, "hasMore": page*60 < total,
	})
}

func (h *Handler) listTGDB(c echo.Context, p platforms.Platform, query string, page int) error {
	var (
		games []tgdb.Game
		total int
		err   error
	)
	if strings.TrimSpace(query) != "" {
		games, err = h.App.TGDB.SearchByPlatform(c.Request().Context(), p.TGDBID, query, 60)
		total = len(games)
	} else {
		out, lerr := h.App.TGDB.ListByPlatform(c.Request().Context(), p.TGDBID, page)
		err = lerr
		if out != nil {
			games = out.Items
			total = out.Total
		}
	}
	if err != nil {
		return RespondErr(c, http.StatusBadGateway, err.Error())
	}
	items := make([]taggedRelease, 0, len(games))
	for _, g := range games {
		items = append(items, taggedRelease{
			ReleaseID:  fmt.Sprintf("%s_%d", sourceTGDB, g.ID),
			Title:      g.Title,
			CoverURL:   g.CoverURL,
			PlatformID: p.ID,
			Source:     sourceTGDB,
		})
	}
	return RespondOK(c, map[string]any{
		"items": items, "total": total, "page": page, "hasMore": page*60 < total,
	})
}

func (h *Handler) listOpenVGDB(c echo.Context, p platforms.Platform, query string, page int) error {
	out, err := h.App.OpenVGDB.ListReleases(c.Request().Context(), openvgdb.CatalogQuery{
		SystemIDs: []int{p.OpenVGDBID},
		Query:     query,
		Page:      page,
		PageSize:  60,
	})
	if err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	items := make([]taggedRelease, 0, len(out.Items))
	for _, r := range out.Items {
		items = append(items, taggedRelease{
			ReleaseID:       fmt.Sprintf("%s_%d", sourceOVGDB, r.ReleaseID),
			Title:           r.Title,
			CoverURL:        r.CoverURL,
			SystemShortName: r.SystemShortName,
			Region:          r.Region,
			PlatformID:      p.ID,
			VariantCount:    r.VariantCount,
			Source:          sourceOVGDB,
		})
	}
	return RespondOK(c, map[string]any{
		"items": items, "total": out.Total, "page": out.Page, "hasMore": out.HasMore,
	})
}

// taggedReleaseDetail is the per-release detail payload, shared shape
// across both backends.
type taggedReleaseDetail struct {
	ReleaseID       string `json:"releaseId"`
	Title           string `json:"title"`
	CoverURL        string `json:"coverUrl,omitempty"`
	SystemShortName string `json:"systemShortName,omitempty"`
	Region          string `json:"region,omitempty"`
	PlatformID      string `json:"platformId"`
	Description     string `json:"description,omitempty"`
	Genre           string `json:"genre,omitempty"`
	Developer       string `json:"developer,omitempty"`
	Publisher       string `json:"publisher,omitempty"`
	ReleaseDate     string `json:"releaseDate,omitempty"`
	BackCoverURL    string `json:"backCoverUrl,omitempty"`
	Source          string `json:"source"`
}

// HandleCatalogGet dispatches on the "<source>:<id>" prefix.
func (h *Handler) HandleCatalogGet(c echo.Context) error {
	source, idNum, err := parseCompositeID(c.Param("id"))
	if err != nil {
		return RespondErr(c, http.StatusBadRequest, err.Error())
	}
	d, err := h.getReleaseDetail(c.Request().Context(), source, idNum)
	if err != nil {
		return RespondErr(c, statusFor(err), err.Error())
	}
	if d == nil {
		return RespondErr(c, http.StatusNotFound, "release introuvable")
	}
	return RespondOK(c, d)
}

// HandleCatalogCover serves the per-release cover with an aggressive
// fall-back chain so the user gets a real box art whenever one exists:
//
//   1. Disk cache (keyed by composite release id)
//   2. libretro-thumbnails Named_Boxarts — proper box scans for every
//      platform listed in github.com/libretro-thumbnails. Preferred
//      over RAWG's `background_image`, which is a gameplay screenshot
//      rather than a cover.
//   3. The source's own coverUrl (RAWG screenshot / IGDB cover /
//      gamefaqs hotlink from OpenVGDB).
//
// The first successful fetch is disk-cached so the chain only runs
// once per release.
func (h *Handler) HandleCatalogCover(c echo.Context) error {
	source, idNum, err := parseCompositeID(c.Param("id"))
	if err != nil {
		return RespondErr(c, http.StatusBadRequest, err.Error())
	}

	cacheDir := filepath.Join(h.App.Config.Data.Dir, "imgcache")
	sum := sha1.Sum([]byte("catalog|" + c.Param("id") + "|" + c.QueryParam("platform")))
	cachePath := filepath.Join(cacheDir, hex.EncodeToString(sum[:]))
	if body, rerr := os.ReadFile(cachePath); rerr == nil {
		return c.Blob(http.StatusOK, http.DetectContentType(body), body)
	}

	d, err := h.getReleaseDetail(c.Request().Context(), source, idNum)
	if err != nil || d == nil {
		return RespondErr(c, http.StatusNotFound, "release introuvable")
	}
	// The list view knows the platform when it builds the card; pass
	// it as ?platform=… so we don't lose it for RAWG / IGDB sources
	// that return their own platform ids in the detail payload.
	if p := c.QueryParam("platform"); p != "" {
		d.PlatformID = p
	}

	candidates := h.coverCandidates(c.Request().Context(), d)
	if len(candidates) == 0 {
		return RespondErr(c, http.StatusNotFound, "pas de jaquette")
	}

	for _, url := range candidates {
		body, contentType, ferr := fetchBytes(url)
		if ferr != nil || len(body) == 0 {
			continue
		}
		if mkErr := os.MkdirAll(cacheDir, 0o755); mkErr == nil {
			_ = os.WriteFile(cachePath, body, 0o644)
		}
		if contentType == "" {
			contentType = http.DetectContentType(body)
		}
		return c.Blob(http.StatusOK, contentType, body)
	}
	return RespondErr(c, http.StatusNotFound, "pas de jaquette disponible")
}

// coverCandidates returns the ordered URLs to try when fetching the
// box art for one release. libretro-thumbnails is matched against the
// cached file listing (fuzzy on canonical title — strips region/rev
// tags), giving near-100 % hit-rate for No-Intro titles, then we fall
// back to whatever URL the source backend handed us.
func (h *Handler) coverCandidates(ctx context.Context, d *taggedReleaseDetail) []string {
	var urls []string
	if d.PlatformID != "" {
		if p, ok := platforms.ByID(d.PlatformID); ok && p.LibretroThumbsName != "" {
			if u := h.App.Thumbs.MatchBoxart(ctx, p.LibretroThumbsName, d.Title); u != "" {
				urls = append(urls, u)
			}
		}
	}
	if d.CoverURL != "" {
		urls = append(urls, d.CoverURL)
	}
	return urls
}

// HandleCatalogSources fans the title across every registered Source.
// Source dispatch first resolves the title via the appropriate backend.
func (h *Handler) HandleCatalogSources(c echo.Context) error {
	source, idNum, err := parseCompositeID(c.Param("id"))
	if err != nil {
		return RespondErr(c, http.StatusBadRequest, err.Error())
	}
	d, err := h.getReleaseDetail(c.Request().Context(), source, idNum)
	if err != nil || d == nil {
		return RespondErr(c, http.StatusNotFound, "release introuvable")
	}

	results := make([]sources.AggregatedResult, len(h.App.Sources))
	var wg sync.WaitGroup
	for i, src := range h.App.Sources {
		i, src := i, src
		wg.Add(1)
		go func() {
			defer wg.Done()
			info := sources.InfoFrom(src)
			items, err := src.Search(c.Request().Context(), d.Title, d.PlatformID)
			if items == nil {
				items = []sources.ROM{}
			}
			res := sources.AggregatedResult{Source: info, Items: items}
			if err != nil {
				res.Error = err.Error()
			}
			results[i] = res
		}()
	}
	wg.Wait()
	return RespondOK(c, results)
}

// -----------------------------------------------------------------------------
// shared helpers
// -----------------------------------------------------------------------------

// getReleaseDetail dispatches the per-source detail fetch and flattens
// it to the unified taggedReleaseDetail shape.
func (h *Handler) getReleaseDetail(ctx context.Context, source string, id int) (*taggedReleaseDetail, error) {
	switch source {
	case sourceOVGDB:
		if h.App.OpenVGDB == nil || !h.App.OpenVGDB.Ready() {
			return nil, errors.New("OpenVGDB indisponible")
		}
		r, err := h.App.OpenVGDB.GetRelease(ctx, id)
		if err != nil || r == nil {
			return nil, err
		}
		return &taggedReleaseDetail{
			ReleaseID:       fmt.Sprintf("%s_%d", sourceOVGDB, r.ReleaseID),
			Title:           r.Title,
			CoverURL:        r.CoverURL,
			BackCoverURL:    r.BackCoverURL,
			SystemShortName: r.SystemShortName,
			Region:          r.Region,
			PlatformID:      platformIDFor(r.OpenVGDBSystemID, 0, 0),
			Description:     r.Description,
			Genre:           r.Genre,
			Developer:       r.Developer,
			Publisher:       r.Publisher,
			ReleaseDate:     r.ReleaseDate,
			Source:          sourceOVGDB,
		}, nil
	case sourceIGDB:
		if h.App.IGDB == nil || !h.App.IGDB.Configured() {
			return nil, errors.New("IGDB non configuré")
		}
		g, err := h.App.IGDB.GetByID(ctx, id)
		if err != nil || g == nil {
			return nil, err
		}
		date := ""
		if g.FirstYear > 0 {
			date = strconv.Itoa(g.FirstYear)
		}
		return &taggedReleaseDetail{
			ReleaseID:   fmt.Sprintf("%s_%d", sourceIGDB, g.ID),
			Title:       g.Name,
			CoverURL:    g.CoverURL,
			PlatformID:  platformIDFor(0, g.PlatformID, 0),
			Description: g.Summary,
			Genre:       g.Genre,
			Developer:   g.Company,
			ReleaseDate: date,
			Source:      sourceIGDB,
		}, nil
	case sourceTGDB:
		if h.App.TGDB == nil || !h.App.TGDB.Configured() {
			return nil, errors.New("TheGamesDB non configuré")
		}
		g, err := h.App.TGDB.GetByID(ctx, id)
		if err != nil || g == nil {
			return nil, err
		}
		return &taggedReleaseDetail{
			ReleaseID:   fmt.Sprintf("%s_%d", sourceTGDB, g.ID),
			Title:       g.Title,
			CoverURL:    g.CoverURL,
			PlatformID:  platformIDFor(0, 0, g.PlatformID),
			Description: g.Summary,
			Genre:       g.Genre,
			Developer:   g.Developer,
			Publisher:   g.Publisher,
			ReleaseDate: g.ReleaseDate,
			Source:      sourceTGDB,
		}, nil
	case sourceRAWG:
		if h.App.RAWG == nil || !h.App.RAWG.Configured() {
			return nil, errors.New("RAWG non configuré")
		}
		g, err := h.App.RAWG.GetByID(ctx, id)
		if err != nil || g == nil {
			return nil, err
		}
		// RAWG response doesn't tell us which of our platforms — we
		// recover it from the URL params in callers. Here we leave it
		// empty; the frontend uses the platformId field carried in
		// the list response when it was already shown.
		return &taggedReleaseDetail{
			ReleaseID:   fmt.Sprintf("%s_%d", sourceRAWG, g.ID),
			Title:       g.Title,
			CoverURL:    g.CoverURL,
			Description: g.Summary,
			Genre:       g.Genre,
			Developer:   g.Developer,
			Publisher:   g.Publisher,
			ReleaseDate: g.ReleaseDate,
			Source:      sourceRAWG,
		}, nil
	}
	return nil, fmt.Errorf("source inconnue %q", source)
}

// parseCompositeID splits "ovgdb:42" / "igdb:1942" into its parts.
func parseCompositeID(s string) (string, int, error) {
	parts := strings.SplitN(s, "_", 2)
	if len(parts) != 2 {
		return "", 0, errors.New("id invalide (format attendu: <source>_<n>)")
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, errors.New("id numérique invalide")
	}
	return parts[0], n, nil
}

// statusFor maps a backend error to a sensible HTTP code.
func statusFor(err error) int {
	if err == nil {
		return http.StatusOK
	}
	switch err.Error() {
	case "OpenVGDB indisponible", "IGDB non configuré", "TheGamesDB non configuré", "RAWG non configuré":
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

// platformIDFor looks up our internal platform id from any combination
// of source ids — pass 0 for the ones you don't have. First non-zero
// match wins so callers don't need to know the precedence.
func platformIDFor(openvgdbID, igdbID, tgdbID int) string {
	for _, p := range platforms.All() {
		if openvgdbID > 0 && p.OpenVGDBID == openvgdbID {
			return p.ID
		}
		if igdbID > 0 && p.IGDBID == igdbID {
			return p.ID
		}
		if tgdbID > 0 && p.TGDBID == tgdbID {
			return p.ID
		}
	}
	return ""
}
