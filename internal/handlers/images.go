package handlers

import (
	"crypto/sha1"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/labstack/echo/v4"
)

var imageHTTP = &http.Client{Timeout: 30 * time.Second}

// HandleGameImage proxies a public cover / screenshot URL for a game.
// Cover URLs come from OpenVGDB (gamefaqs CDN) or libretro-thumbnails;
// both are credential-free, so the proxy only adds a disk cache (under
// <datadir>/imgcache) to spare upstream and keep the UI snappy.
func (h *Handler) HandleGameImage(c echo.Context) error {
	id, err := parseUintParam(c, "id")
	if err != nil {
		return RespondErr(c, http.StatusBadRequest, "id invalide")
	}
	g, err := h.App.Database.GetGame(id)
	if err != nil {
		return RespondErr(c, http.StatusNotFound, "jeu introuvable")
	}

	kind := c.Param("kind")
	var rawURL string
	switch kind {
	case "cover":
		rawURL = g.CoverURL
	case "screenshot":
		rawURL = g.ScreenshotURL
	default:
		return RespondErr(c, http.StatusBadRequest, "type d'image inconnu")
	}
	if rawURL == "" {
		return RespondErr(c, http.StatusNotFound, "pas de média pour ce type")
	}

	cacheDir := filepath.Join(h.App.Config.Data.Dir, "imgcache")
	sum := sha1.Sum([]byte(kind + "|" + rawURL))
	cachePath := filepath.Join(cacheDir, hex.EncodeToString(sum[:]))

	if body, rerr := os.ReadFile(cachePath); rerr == nil {
		return c.Blob(http.StatusOK, http.DetectContentType(body), body)
	}

	body, contentType, err := fetchBytes(rawURL)
	if err != nil {
		return RespondErr(c, http.StatusBadGateway, err.Error())
	}
	if mkErr := os.MkdirAll(cacheDir, 0o755); mkErr == nil {
		_ = os.WriteFile(cachePath, body, 0o644)
	}
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	return c.Blob(http.StatusOK, contentType, body)
}

// fetchBytes GETs an image with browser-like headers. Some hosts
// (notably gamefaqs.gamespot.com, where OpenVGDB covers live) are
// behind Cloudflare and 403 the default Go User-Agent.
func fetchBytes(rawURL string) ([]byte, string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/*,*/*;q=0.8")
	req.Header.Set("Accept-Language", "fr-FR,fr;q=0.9,en;q=0.8")
	res, err := imageHTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", &httpError{status: res.StatusCode, msg: res.Status}
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return nil, "", err
	}
	return body, res.Header.Get("Content-Type"), nil
}

type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }
