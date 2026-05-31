package handlers

import (
	"retrox/internal/platforms"

	"github.com/labstack/echo/v4"
)

// HandlePlatforms returns the static catalog so the frontend can label
// rails and offer a platform picker on the downloads form.
func (h *Handler) HandlePlatforms(c echo.Context) error {
	return RespondOK(c, platforms.All())
}
