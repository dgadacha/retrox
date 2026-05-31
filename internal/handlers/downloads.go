package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func (h *Handler) HandleListDownloads(c echo.Context) error {
	rows, err := h.App.Database.ListDownloads()
	if err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	return RespondOK(c, rows)
}

type createDownloadReq struct {
	URL        string `json:"url"`
	PlatformID string `json:"platformId"`
	Title      string `json:"title"`
}

func (h *Handler) HandleCreateDownload(c echo.Context) error {
	var req createDownloadReq
	if err := c.Bind(&req); err != nil {
		return RespondErr(c, http.StatusBadRequest, "corps de requête invalide")
	}
	if req.URL == "" {
		return RespondErr(c, http.StatusBadRequest, "url requise")
	}
	row, err := h.App.Downloads.Enqueue(req.URL, req.PlatformID, req.Title)
	if err != nil {
		return RespondErr(c, http.StatusBadRequest, err.Error())
	}
	return RespondOK(c, row)
}

func (h *Handler) HandleCancelDownload(c echo.Context) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return RespondErr(c, http.StatusBadRequest, "id invalide")
	}
	if err := h.App.Downloads.Cancel(id); err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	return RespondOK(c, map[string]any{"canceled": true})
}
