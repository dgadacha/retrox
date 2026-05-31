package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// HandleGetSettings returns the admin-mutable server config.
func (h *Handler) HandleGetSettings(c echo.Context) error {
	cfg := h.App.Config
	return RespondOK(c, map[string]any{
		"romDirs":        cfg.Library.Roots,
		"retroarchBin":   cfg.Emulator.RetroArchBin,
		"retroarchCores": cfg.Emulator.RetroArchCores,
		"openvgdbPath":   cfg.Metadata.Path,
	})
}

type updateSettingsReq struct {
	RomDirs        []string `json:"romDirs"`
	RetroArchBin   string   `json:"retroarchBin"`
	RetroArchCores string   `json:"retroarchCores"`
}

func (h *Handler) HandleUpdateSettings(c echo.Context) error {
	var req updateSettingsReq
	if err := c.Bind(&req); err != nil {
		return RespondErr(c, http.StatusBadRequest, "corps de requête invalide")
	}
	if err := h.App.ApplyServerConfig(req.RomDirs, req.RetroArchBin, req.RetroArchCores); err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	return h.HandleGetSettings(c)
}

// HandleDownloadOpenVGDB fetches and extracts the OpenVGDB SQLite. It's
// synchronous (the file is small — ~9 MB) so the UI gets a clear
// success/failure response without polling.
func (h *Handler) HandleDownloadOpenVGDB(c echo.Context) error {
	if err := h.App.DownloadOpenVGDB(c.Request().Context()); err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	roms, releases := h.App.OpenVGDB.Counts()
	return RespondOK(c, map[string]any{
		"ready":    true,
		"roms":     roms,
		"releases": releases,
		"path":     h.App.Config.Metadata.Path,
	})
}
