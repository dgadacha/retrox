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

	"retrox/internal/igdb"
	"retrox/internal/openvgdb"
	"retrox/internal/platforms"
	"retrox/internal/sources"

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
)

// HandleCatalogPlatforms returns one tile per RETROX-supported platform
// that has *any* metadata available. IGDB is the user-visible default
// when configured (uniform quality); OpenVGDB is the fallback for
// platforms where IGDB can't help OR when the user hasn't pasted
// Twitch creds yet.
func (h *Handler) HandleCatalogPlatforms(c echo.Context) error {
	ctx := c.Request().Context()
	igdbReady := h.App.IGDB != nil && h.App.IGDB.Configured()
	ovgdbReady := h.App.OpenVGDB != nil && h.App.OpenVGDB.Ready()

	// Pre-load OpenVGDB counts (cheap, always available when DB is
	// ready) so the fallback path doesn't have to round-trip per row.
	ovgdbCounts := map[int]int{}
	if ovgdbReady {
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

	type tile struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Source string `json:"source"` // "ovgdb" | "igdb"
		Count  int    `json:"count"`
	}
	out := make([]tile, 0, 30)

	for _, p := range platforms.All() {
		// Prefer IGDB whenever it's wired up — that's the "full IGDB"
		// experience the user opted into.
		if igdbReady && p.IGDBID > 0 {
			n, err := h.App.IGDB.CountByPlatform(ctx, p.IGDBID)
			if err == nil && n > 0 {
				out = append(out, tile{ID: p.ID, Name: p.Name, Source: sourceIGDB, Count: n})
				continue
			}
		}
		// Fall back to OpenVGDB.
		if n := ovgdbCounts[p.OpenVGDBID]; n > 0 {
			out = append(out, tile{ID: p.ID, Name: p.Name, Source: sourceOVGDB, Count: n})
		}
	}

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

	// IGDB primary when configured.
	if p.IGDBID > 0 && h.App.IGDB != nil && h.App.IGDB.Configured() {
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
				ReleaseID:  fmt.Sprintf("%s:%d", sourceIGDB, g.ID),
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

	// OpenVGDB fallback (IGDB not configured, or platform not in IGDB).
	if p.OpenVGDBID > 0 && h.App.OpenVGDB != nil && h.App.OpenVGDB.Ready() {
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
				ReleaseID:       fmt.Sprintf("%s:%d", sourceOVGDB, r.ReleaseID),
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

	return RespondErr(c, http.StatusNotFound,
		"aucune source de métadonnées pour cette plateforme — configurez IGDB dans Réglages")
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

// HandleCatalogCover proxies the per-release cover regardless of source.
// OpenVGDB: gamefaqs hotlink (needs browser UA); IGDB: images.igdb.com
// (no Cloudflare). Both get the same disk cache treatment.
func (h *Handler) HandleCatalogCover(c echo.Context) error {
	source, idNum, err := parseCompositeID(c.Param("id"))
	if err != nil {
		return RespondErr(c, http.StatusBadRequest, err.Error())
	}
	d, err := h.getReleaseDetail(c.Request().Context(), source, idNum)
	if err != nil || d == nil || d.CoverURL == "" {
		return RespondErr(c, http.StatusNotFound, "pas de jaquette")
	}

	cacheDir := filepath.Join(h.App.Config.Data.Dir, "imgcache")
	sum := sha1.Sum([]byte("catalog|" + d.CoverURL))
	cachePath := filepath.Join(cacheDir, hex.EncodeToString(sum[:]))
	if body, rerr := os.ReadFile(cachePath); rerr == nil {
		return c.Blob(http.StatusOK, http.DetectContentType(body), body)
	}
	body, contentType, err := fetchBytes(d.CoverURL)
	if err != nil {
		return RespondErr(c, http.StatusBadGateway, err.Error())
	}
	if mkErr := os.MkdirAll(cacheDir, 0o755); mkErr == nil {
		_ = os.WriteFile(cachePath, body, 0o644)
	}
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	return c.Blob(http.StatusOK, contentType, body)
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
			ReleaseID:       fmt.Sprintf("%s:%d", sourceOVGDB, r.ReleaseID),
			Title:           r.Title,
			CoverURL:        r.CoverURL,
			BackCoverURL:    r.BackCoverURL,
			SystemShortName: r.SystemShortName,
			Region:          r.Region,
			PlatformID:      platformIDFor(r.OpenVGDBSystemID, 0),
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
			ReleaseID:   fmt.Sprintf("%s:%d", sourceIGDB, g.ID),
			Title:       g.Name,
			CoverURL:    g.CoverURL,
			PlatformID:  platformIDFor(0, g.PlatformID),
			Description: g.Summary,
			Genre:       g.Genre,
			Developer:   g.Company,
			ReleaseDate: date,
			Source:      sourceIGDB,
		}, nil
	}
	return nil, fmt.Errorf("source inconnue %q", source)
}

// parseCompositeID splits "ovgdb:42" / "igdb:1942" into its parts.
func parseCompositeID(s string) (string, int, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return "", 0, errors.New("id invalide (format attendu: <source>:<n>)")
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
	case "OpenVGDB indisponible", "IGDB non configuré":
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

// platformIDFor looks up our internal platform id from either an
// OpenVGDB system id or an IGDB platform id. Pass 0 for the one you
// don't have.
func platformIDFor(openvgdbID, igdbID int) string {
	for _, p := range platforms.All() {
		if openvgdbID > 0 && p.OpenVGDBID == openvgdbID {
			return p.ID
		}
		if igdbID > 0 && p.IGDBID == igdbID {
			return p.ID
		}
	}
	return ""
}
