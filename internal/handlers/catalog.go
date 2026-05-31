package handlers

import (
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"retrox/internal/openvgdb"
	"retrox/internal/platforms"
	"retrox/internal/sources"

	"github.com/labstack/echo/v4"
)

// HandleCatalogPlatforms returns the platform-picker payload: one entry
// per RETROX-supported platform that has at least one OpenVGDB release,
// with the deduped game count for tile badges.
func (h *Handler) HandleCatalogPlatforms(c echo.Context) error {
	if h.App.OpenVGDB == nil || !h.App.OpenVGDB.Ready() {
		return RespondErr(c, http.StatusServiceUnavailable, "OpenVGDB indisponible")
	}
	var ids []int
	for _, p := range platforms.All() {
		if p.OpenVGDBID > 0 {
			ids = append(ids, p.OpenVGDBID)
		}
	}
	counts, err := h.App.OpenVGDB.CountReleasesByPlatform(c.Request().Context(), ids)
	if err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	type tile struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		OpenVGDBID int    `json:"openvgdbId"`
		Count      int    `json:"count"`
	}
	out := make([]tile, 0, len(ids))
	for _, p := range platforms.All() {
		if p.OpenVGDBID == 0 {
			continue
		}
		n := counts[p.OpenVGDBID]
		if n == 0 {
			continue
		}
		out = append(out, tile{ID: p.ID, Name: p.Name, OpenVGDBID: p.OpenVGDBID, Count: n})
	}
	return RespondOK(c, out)
}

// HandleCatalogList returns a paginated slice of OpenVGDB releases.
// Filter by `platform` (our internal id, e.g. "snes") and free-text
// `q`. Falls back to "all RETROX-supported platforms" when no platform
// is given so the user doesn't see games from systems they can't run.
func (h *Handler) HandleCatalogList(c echo.Context) error {
	if h.App.OpenVGDB == nil || !h.App.OpenVGDB.Ready() {
		return RespondErr(c, http.StatusServiceUnavailable,
			"OpenVGDB n'est pas disponible — téléchargez-la dans Réglages")
	}
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	q := openvgdb.CatalogQuery{
		SystemIDs: catalogSystemIDsFor(c.QueryParam("platform")),
		Query:     c.QueryParam("q"),
		Page:      page,
		PageSize:  60,
	}
	out, err := h.App.OpenVGDB.ListReleases(c.Request().Context(), q)
	if err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	// Tag each release with our platform ID (frontend wants it for
	// routing the download into the right folder).
	type withPlatform struct {
		openvgdb.Release
		PlatformID string `json:"platformId"`
	}
	tagged := make([]withPlatform, 0, len(out.Items))
	for _, r := range out.Items {
		tagged = append(tagged, withPlatform{Release: r, PlatformID: platformIDFor(r.OpenVGDBSystemID)})
	}
	return RespondOK(c, map[string]any{
		"items":   tagged,
		"total":   out.Total,
		"page":    out.Page,
		"hasMore": out.HasMore,
	})
}

// HandleCatalogGet returns one release's full detail.
func (h *Handler) HandleCatalogGet(c echo.Context) error {
	if h.App.OpenVGDB == nil || !h.App.OpenVGDB.Ready() {
		return RespondErr(c, http.StatusServiceUnavailable, "OpenVGDB indisponible")
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return RespondErr(c, http.StatusBadRequest, "id invalide")
	}
	r, err := h.App.OpenVGDB.GetRelease(c.Request().Context(), id)
	if err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	if r == nil {
		return RespondErr(c, http.StatusNotFound, "release introuvable")
	}
	type withPlatform struct {
		openvgdb.ReleaseDetail
		PlatformID string `json:"platformId"`
	}
	return RespondOK(c, withPlatform{ReleaseDetail: *r, PlatformID: platformIDFor(r.OpenVGDBSystemID)})
}

// HandleCatalogCover proxies the release's coverFront URL (gamefaqs CDN
// behind Cloudflare), adding a browser-like User-Agent so the request
// isn't bot-challenged, and disk-caching the bytes for next time.
func (h *Handler) HandleCatalogCover(c echo.Context) error {
	if h.App.OpenVGDB == nil || !h.App.OpenVGDB.Ready() {
		return RespondErr(c, http.StatusServiceUnavailable, "OpenVGDB indisponible")
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return RespondErr(c, http.StatusBadRequest, "id invalide")
	}
	r, err := h.App.OpenVGDB.GetRelease(c.Request().Context(), id)
	if err != nil || r == nil || r.CoverURL == "" {
		return RespondErr(c, http.StatusNotFound, "pas de jaquette")
	}

	cacheDir := filepath.Join(h.App.Config.Data.Dir, "imgcache")
	sum := sha1.Sum([]byte("catalog|" + r.CoverURL))
	cachePath := filepath.Join(cacheDir, hex.EncodeToString(sum[:]))
	if body, rerr := os.ReadFile(cachePath); rerr == nil {
		return c.Blob(http.StatusOK, http.DetectContentType(body), body)
	}

	body, contentType, err := fetchBytes(r.CoverURL)
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

// HandleCatalogSources fans out the title across every registered Source
// in parallel and returns the merged candidates so the detail page can
// render them as a pick-list.
func (h *Handler) HandleCatalogSources(c echo.Context) error {
	if h.App.OpenVGDB == nil || !h.App.OpenVGDB.Ready() {
		return RespondErr(c, http.StatusServiceUnavailable, "OpenVGDB indisponible")
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return RespondErr(c, http.StatusBadRequest, "id invalide")
	}
	r, err := h.App.OpenVGDB.GetRelease(c.Request().Context(), id)
	if err != nil || r == nil {
		return RespondErr(c, http.StatusNotFound, "release introuvable")
	}
	platform := platformIDFor(r.OpenVGDBSystemID)
	title := strings.TrimSpace(r.Title)

	results := make([]sources.AggregatedResult, len(h.App.Sources))
	var wg sync.WaitGroup
	for i, src := range h.App.Sources {
		i, src := i, src
		wg.Add(1)
		go func() {
			defer wg.Done()
			info := sources.InfoFrom(src)
			items, err := src.Search(c.Request().Context(), title, platform)
			// Materialise a nil slice into [] so the frontend never has
			// to guard against `null.length`.
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

// platformIDFor inverts the platforms catalog: given an OpenVGDB
// system id, returns our internal platform id ("snes", "n64", …).
func platformIDFor(openvgdbID int) string {
	for _, p := range platforms.All() {
		if p.OpenVGDBID == openvgdbID {
			return p.ID
		}
	}
	return ""
}

// catalogSystemIDsFor returns the OpenVGDB system ids matching either a
// single platform filter or the union of all RETROX-supported platforms
// (so an empty filter still hides systems we can't run).
func catalogSystemIDsFor(platformID string) []int {
	if platformID != "" {
		if p, ok := platforms.ByID(platformID); ok && p.OpenVGDBID > 0 {
			return []int{p.OpenVGDBID}
		}
		return []int{0} // unknown filter → no results
	}
	var out []int
	for _, p := range platforms.All() {
		if p.OpenVGDBID > 0 {
			out = append(out, p.OpenVGDBID)
		}
	}
	return out
}
