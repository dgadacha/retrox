package handlers

import (
	"net/http"
	"strconv"

	"retrox/internal/emulator"

	"github.com/labstack/echo/v4"
)

func (h *Handler) HandleListGames(c echo.Context) error {
	platform := c.QueryParam("platform")
	if platform != "" {
		games, err := h.App.Database.ListGamesByPlatform(platform)
		if err != nil {
			return RespondErr(c, http.StatusInternalServerError, err.Error())
		}
		return RespondOK(c, games)
	}
	games, err := h.App.Database.ListGames()
	if err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	return RespondOK(c, games)
}

func (h *Handler) HandleGetGame(c echo.Context) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return RespondErr(c, http.StatusBadRequest, "id invalide")
	}
	g, err := h.App.Database.GetGame(id)
	if err != nil {
		return RespondErr(c, http.StatusNotFound, "jeu introuvable")
	}
	return RespondOK(c, g)
}

// HandlePlayGame launches the native emulator for a game and, when a
// ?profile=uid is supplied, records the play in that profile's history.
// Returns the resolved command so the UI can show what was launched.
func (h *Handler) HandlePlayGame(c echo.Context) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return RespondErr(c, http.StatusBadRequest, "id invalide")
	}
	g, err := h.App.Database.GetGame(id)
	if err != nil {
		return RespondErr(c, http.StatusNotFound, "jeu introuvable")
	}
	if g.Missing {
		return RespondErr(c, http.StatusConflict, "le fichier ROM est introuvable sur le disque — relancez un scan")
	}

	binding, _ := h.App.Database.GetEmulatorBinding(g.PlatformID)
	resolved, err := emulator.Launch(h.App.EmulatorConfig(), g.PlatformID, g.Path, binding)
	if err != nil {
		return RespondErr(c, http.StatusBadRequest, err.Error())
	}

	profile := c.QueryParam("profile")
	if profile == "" {
		profile = h.App.DefaultProfileUID()
	}
	if profile != "" {
		if err := h.App.Database.IncrementPlay(profile, g); err != nil {
			// Non-fatal: the game launched, history is best-effort.
			c.Logger().Warnf("play history: %v", err)
		}
	}
	return RespondOK(c, resolved)
}

func parseUintParam(c echo.Context, name string) (uint, error) {
	n, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(n), nil
}
