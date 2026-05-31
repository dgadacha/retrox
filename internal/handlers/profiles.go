package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"retrox/internal/database/models"

	"github.com/labstack/echo/v4"
)

func (h *Handler) HandleListProfiles(c echo.Context) error {
	profiles, err := h.App.Database.ListProfiles()
	if err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	return RespondOK(c, profiles)
}

type profileReq struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
	Color  string `json:"color"`
}

func (h *Handler) HandleCreateProfile(c echo.Context) error {
	var req profileReq
	if err := c.Bind(&req); err != nil {
		return RespondErr(c, http.StatusBadRequest, "corps de requête invalide")
	}
	if req.Name == "" {
		return RespondErr(c, http.StatusBadRequest, "nom requis")
	}
	p, err := h.App.Database.CreateProfile(&models.Profile{
		UID:    newUID(),
		Name:   req.Name,
		Avatar: req.Avatar,
		Color:  req.Color,
	})
	if err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	return RespondOK(c, p)
}

func (h *Handler) HandleUpdateProfile(c echo.Context) error {
	var req profileReq
	if err := c.Bind(&req); err != nil {
		return RespondErr(c, http.StatusBadRequest, "corps de requête invalide")
	}
	p, err := h.App.Database.UpdateProfile(c.Param("uid"), req.Name, req.Avatar, req.Color)
	if err != nil {
		return RespondErr(c, http.StatusNotFound, "profil introuvable")
	}
	return RespondOK(c, p)
}

func (h *Handler) HandleDeleteProfile(c echo.Context) error {
	if err := h.App.Database.DeleteProfile(c.Param("uid")); err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	return RespondOK(c, map[string]any{"deleted": true})
}

func (h *Handler) HandleListHistory(c echo.Context) error {
	rows, err := h.App.Database.ListPlayHistory(c.Param("uid"))
	if err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	return RespondOK(c, rows)
}

func (h *Handler) HandleListFavorites(c echo.Context) error {
	rows, err := h.App.Database.ListFavorites(c.Param("uid"))
	if err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	return RespondOK(c, rows)
}

type favoriteReq struct {
	GameID uint `json:"gameId"`
}

func (h *Handler) HandleAddFavorite(c echo.Context) error {
	var req favoriteReq
	if err := c.Bind(&req); err != nil {
		return RespondErr(c, http.StatusBadRequest, "corps de requête invalide")
	}
	g, err := h.App.Database.GetGame(req.GameID)
	if err != nil {
		return RespondErr(c, http.StatusNotFound, "jeu introuvable")
	}
	if err := h.App.Database.AddFavorite(c.Param("uid"), g); err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	return RespondOK(c, map[string]any{"added": true})
}

func (h *Handler) HandleRemoveFavorite(c echo.Context) error {
	gameID, err := parseUintParam(c, "gameId")
	if err != nil {
		return RespondErr(c, http.StatusBadRequest, "id invalide")
	}
	if err := h.App.Database.RemoveFavorite(c.Param("uid"), gameID); err != nil {
		return RespondErr(c, http.StatusInternalServerError, err.Error())
	}
	return RespondOK(c, map[string]any{"removed": true})
}

// newUID returns a random 16-hex-char identifier for a profile.
func newUID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
