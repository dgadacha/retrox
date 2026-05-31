// Package handlers wires Echo routes to the App service layer.
//
// Route map (all under /api/v1):
//
//	GET    /status                          health + scraper-configured flag
//	GET    /platforms                       static platform catalog
//	GET    /games                           list games (?platform= to filter)
//	GET    /games/:id                       one game with full metadata
//	POST   /games/:id/play                  launch the emulator (?profile=uid)
//	GET    /games/:id/image/:kind           ScreenScraper media proxy
//	POST   /library/scan                    trigger a (re)scan
//	GET    /library/scan/status             live scan progress
//	GET    /downloads                       list downloads
//	POST   /downloads                       queue a ROM download
//	DELETE /downloads/:id                   cancel a download
//	GET    /emulators                       per-platform launch config
//	PUT    /emulators/:platformId           set a launch override
//	DELETE /emulators/:platformId           clear an override
//	GET    /profiles                        list profiles
//	POST   /profiles                        create profile
//	PATCH  /profiles/:uid                   update profile
//	DELETE /profiles/:uid                   delete profile
//	GET    /profiles/:uid/history           play history
//	GET    /profiles/:uid/favorites         favorites
//	POST   /profiles/:uid/favorites         add favorite {gameId}
//	DELETE /profiles/:uid/favorites/:gameId remove favorite
//	GET    /settings                        admin server config
//	PUT    /settings                        update server config
//	POST   /metadata/openvgdb/download      download (or refresh) openvgdb.sqlite
package handlers

import (
	"net/http"

	"retrox/internal/core"

	"github.com/labstack/echo/v4"
)

type Handler struct {
	App *core.App
}

func New(app *core.App) *Handler { return &Handler{App: app} }

func RegisterRoutes(e *echo.Echo, h *Handler) {
	v1 := e.Group("/api/v1")

	v1.GET("/status", h.HandleStatus)
	v1.GET("/platforms", h.HandlePlatforms)

	v1.GET("/games", h.HandleListGames)
	v1.GET("/games/:id", h.HandleGetGame)
	v1.POST("/games/:id/play", h.HandlePlayGame)
	v1.GET("/games/:id/image/:kind", h.HandleGameImage)

	lib := v1.Group("/library")
	lib.POST("/scan", h.HandleScan)
	lib.GET("/scan/status", h.HandleScanStatus)

	dl := v1.Group("/downloads")
	dl.GET("", h.HandleListDownloads)
	dl.POST("", h.HandleCreateDownload)
	dl.DELETE("/:id", h.HandleCancelDownload)

	emu := v1.Group("/emulators")
	emu.GET("", h.HandleListEmulators)
	emu.PUT("/:platformId", h.HandleSetEmulator)
	emu.DELETE("/:platformId", h.HandleDeleteEmulator)

	p := v1.Group("/profiles")
	p.GET("", h.HandleListProfiles)
	p.POST("", h.HandleCreateProfile)
	p.PATCH("/:uid", h.HandleUpdateProfile)
	p.DELETE("/:uid", h.HandleDeleteProfile)
	p.GET("/:uid/history", h.HandleListHistory)
	p.GET("/:uid/favorites", h.HandleListFavorites)
	p.POST("/:uid/favorites", h.HandleAddFavorite)
	p.DELETE("/:uid/favorites/:gameId", h.HandleRemoveFavorite)

	v1.GET("/settings", h.HandleGetSettings)
	v1.PUT("/settings", h.HandleUpdateSettings)

	v1.POST("/metadata/openvgdb/download", h.HandleDownloadOpenVGDB)

	cat := v1.Group("/catalog")
	cat.GET("", h.HandleCatalogList)
	cat.GET("/:id", h.HandleCatalogGet)
	cat.GET("/:id/cover", h.HandleCatalogCover)
	cat.GET("/:id/sources", h.HandleCatalogSources)

	src := v1.Group("/sources")
	src.GET("", h.HandleListSources)
	src.POST("/:id/download", h.HandleDownloadFromSource)
}

// RespondOK wraps a payload as {"data": ...}.
func RespondOK(c echo.Context, data any) error {
	return c.JSON(http.StatusOK, map[string]any{"data": data})
}

// RespondErr returns {"error": msg} with the given status code.
func RespondErr(c echo.Context, code int, msg string) error {
	return c.JSON(code, map[string]any{"error": msg})
}
