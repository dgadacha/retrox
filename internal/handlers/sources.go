package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"retrox/internal/sources"

	"github.com/labstack/echo/v4"
)

func (h *Handler) HandleListSources(c echo.Context) error {
	out := make([]sources.Info, 0, len(h.App.Sources))
	for _, s := range h.App.Sources {
		out = append(out, sources.InfoFrom(s))
	}
	return RespondOK(c, out)
}

func (h *Handler) HandleBrowseSource(c echo.Context) error {
	src := h.findSource(c.Param("id"))
	if src == nil {
		return RespondErr(c, http.StatusNotFound, "source inconnue")
	}
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	p, err := src.Browse(c.Request().Context(), sources.BrowseOptions{
		PlatformID: c.QueryParam("platform"),
		Query:      c.QueryParam("q"),
		Page:       page,
	})
	if err != nil {
		return RespondErr(c, http.StatusBadGateway, err.Error())
	}
	return RespondOK(c, p)
}

type sourceDownloadReq struct {
	RomID      string `json:"romId"`
	PlatformID string `json:"platformId"`
	Title      string `json:"title"`
}

// HandleDownloadFromSource resolves the source-internal id to a direct
// URL and enqueues it through the existing download manager so the file
// lands in ./roms/<platform>/ and triggers an automatic rescan.
func (h *Handler) HandleDownloadFromSource(c echo.Context) error {
	src := h.findSource(c.Param("id"))
	if src == nil {
		return RespondErr(c, http.StatusNotFound, "source inconnue")
	}
	var req sourceDownloadReq
	if err := c.Bind(&req); err != nil {
		return RespondErr(c, http.StatusBadRequest, "corps de requête invalide")
	}
	if !src.Downloadable() {
		return RespondErr(c, http.StatusUnprocessableEntity,
			"cette source n'expose pas d'URL directe — ouvrez la page externe")
	}
	url, err := src.Resolve(c.Request().Context(), req.RomID)
	if err != nil {
		if errors.Is(err, sources.ErrNotDownloadable) {
			return RespondErr(c, http.StatusUnprocessableEntity, err.Error())
		}
		return RespondErr(c, http.StatusBadGateway, err.Error())
	}
	dl, err := h.App.Downloads.Enqueue(url, req.PlatformID, req.Title)
	if err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	return RespondOK(c, dl)
}

func (h *Handler) findSource(id string) sources.Source {
	for _, s := range h.App.Sources {
		if s.ID() == id {
			return s
		}
	}
	return nil
}
