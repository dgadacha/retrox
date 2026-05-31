package handlers

import (
	"net/http"

	"retrox/internal/scanner"

	"github.com/labstack/echo/v4"
)

// HandleScan kicks a background (re)scan of the configured ROM roots.
// Returns 409 if one is already running; the frontend polls
// /library/scan/status for progress and reloads the grid when it ends.
func (h *Handler) HandleScan(c echo.Context) error {
	if scanner.Running() {
		return RespondErr(c, http.StatusConflict, "un scan est déjà en cours")
	}
	if len(h.App.Config.Library.Roots) == 0 {
		return RespondErr(c, http.StatusBadRequest, "aucun dossier ROM configuré")
	}
	h.App.ScanAsync()
	return RespondOK(c, map[string]any{"started": true})
}

func (h *Handler) HandleScanStatus(c echo.Context) error {
	return RespondOK(c, scanner.Progress())
}
