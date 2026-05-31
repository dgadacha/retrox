package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// HandleGetSettings returns the admin-mutable server config. IGDB
// secret is echoed back as a "is it set" boolean so the UI can hide
// the input behind a "•••••• (défini)" placeholder.
func (h *Handler) HandleGetSettings(c echo.Context) error {
	cfg := h.App.Config
	return RespondOK(c, map[string]any{
		"romDirs":               cfg.Library.Roots,
		"retroarchBin":          cfg.Emulator.RetroArchBin,
		"retroarchCores":        cfg.Emulator.RetroArchCores,
		"openvgdbPath":          cfg.Metadata.Path,
		"igdbClientId":          cfg.Metadata.IGDBClientID,
		"igdbClientSecretSet":   cfg.Metadata.IGDBClientSecret != "",
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

type igdbCredsReq struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"` // "" = keep current
}

// HandleSetIGDBCreds updates Twitch OAuth credentials and verifies them
// by asking for an access token. Failed auth comes back as 400 so the
// UI can red-flag the inputs.
func (h *Handler) HandleSetIGDBCreds(c echo.Context) error {
	var req igdbCredsReq
	if err := c.Bind(&req); err != nil {
		return RespondErr(c, http.StatusBadRequest, "corps de requête invalide")
	}
	secret := req.ClientSecret
	if secret == "" {
		secret = h.App.Config.Metadata.IGDBClientSecret
	}
	if err := h.App.ApplyIGDBCredentials(req.ClientID, secret); err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	// Probe by counting NES games — any failure here surfaces bad creds
	// before the user discovers them later via the catalogue.
	if req.ClientID != "" && secret != "" {
		if _, err := h.App.IGDB.CountByPlatform(c.Request().Context(), 18); err != nil {
			return RespondErr(c, http.StatusBadRequest,
				"credentials refusés par Twitch/IGDB : "+err.Error())
		}
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
