package handlers

import (
	"net/http"

	"retrox/internal/database/models"
	"retrox/internal/platforms"

	"github.com/labstack/echo/v4"
)

// emulatorView merges a platform's catalog defaults with any admin
// override so the settings UI can show "what will run" per platform.
type emulatorView struct {
	PlatformID        string                  `json:"platformId"`
	Name              string                  `json:"name"`
	DefaultCore       string                  `json:"defaultCore,omitempty"`
	DefaultStandalone string                  `json:"defaultStandalone,omitempty"`
	Override          *models.EmulatorBinding `json:"override,omitempty"`
}

func (h *Handler) HandleListEmulators(c echo.Context) error {
	bindings, err := h.App.Database.ListEmulatorBindings()
	if err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	byPlatform := make(map[string]*models.EmulatorBinding, len(bindings))
	for _, b := range bindings {
		byPlatform[b.PlatformID] = b
	}

	views := make([]emulatorView, 0, len(platforms.All()))
	for _, p := range platforms.All() {
		views = append(views, emulatorView{
			PlatformID:        p.ID,
			Name:              p.Name,
			DefaultCore:       p.Core,
			DefaultStandalone: p.Standalone,
			Override:          byPlatform[p.ID],
		})
	}
	return RespondOK(c, views)
}

type setEmulatorReq struct {
	Command string `json:"command"`
	Args    string `json:"args"`
	Core    string `json:"core"`
}

func (h *Handler) HandleSetEmulator(c echo.Context) error {
	platformID := c.Param("platformId")
	if _, ok := platforms.ByID(platformID); !ok {
		return RespondErr(c, http.StatusBadRequest, "plateforme inconnue")
	}
	var req setEmulatorReq
	if err := c.Bind(&req); err != nil {
		return RespondErr(c, http.StatusBadRequest, "corps de requête invalide")
	}
	binding := &models.EmulatorBinding{
		PlatformID: platformID,
		Command:    req.Command,
		Args:       req.Args,
		Core:       req.Core,
	}
	if err := h.App.Database.UpsertEmulatorBinding(binding); err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	return RespondOK(c, binding)
}

func (h *Handler) HandleDeleteEmulator(c echo.Context) error {
	platformID := c.Param("platformId")
	if err := h.App.Database.DeleteEmulatorBinding(platformID); err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	return RespondOK(c, map[string]any{"deleted": true})
}
