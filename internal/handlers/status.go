package handlers

import (
	"net/http"

	"retrox/internal/scanner"

	"github.com/labstack/echo/v4"
)

func (h *Handler) HandleStatus(c echo.Context) error {
	scraped, total, _ := h.App.Database.CountGames()
	ovgdbReady := h.App.OpenVGDB != nil && h.App.OpenVGDB.Ready()
	var ovgdbRoms, ovgdbReleases int
	if ovgdbReady {
		ovgdbRoms, ovgdbReleases = h.App.OpenVGDB.Counts()
	}
	return c.JSON(http.StatusOK, map[string]any{
		"data": map[string]any{
			"app":               "retrox",
			"version":           "0.1.0",
			"metadataReady":     h.App.Metadata != nil && h.App.Metadata.Ready(),
			"openvgdbReady":     ovgdbReady,
			"openvgdbRoms":      ovgdbRoms,
			"openvgdbReleases":  ovgdbReleases,
			"openvgdbPath":      h.App.Config.Metadata.Path,
			"datadir":           h.App.Config.Data.Dir,
			"romDirs":           h.App.Config.Library.Roots,
			"games":             total,
			"gamesScraped":      scraped,
			"scanning":          scanner.Running(),
			"defaultProfileUid":   h.App.DefaultProfileUID(),
			"igdbConfigured":     h.App.IGDB != nil && h.App.IGDB.Configured(),
			"tgdbConfigured":     h.App.TGDB != nil && h.App.TGDB.Configured(),
			"metadataPreference": h.App.Config.Metadata.Preference,
		},
	})
}
