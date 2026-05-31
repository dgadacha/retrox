// RETROX — Netflix-style self-hosted launcher for retro games.
//
// Architecture echoes Notflix: a single Go binary with the React build
// embedded via //go:embed serves both the API and the UI on one port.
// ScreenScraper.fr is proxied through the backend (keys stay server-side)
// for box-art + synopsis; a local scanner indexes ROM folders; and the
// "Play" endpoint spawns the right native emulator (RetroArch core or a
// standalone like Dolphin / PCSX2) for the selected game.
package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"retrox/internal/core"
	"retrox/internal/handlers"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

//go:embed all:web
var embeddedWeb embed.FS

func main() {
	app, err := core.New()
	if err != nil {
		log.Fatalf("retrox: failed to initialize: %v", err)
	}
	defer app.Close()

	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())

	// CORS — the dev frontend (rsbuild) runs on :50001 while the API
	// listens on :50000. In production the SPA is served by this same
	// Echo instance (same-origin) so CORS doesn't fire there.
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{
			"http://127.0.0.1:50001",
			"http://localhost:50001",
		},
		AllowCredentials: true,
		AllowMethods: []string{
			http.MethodGet, http.MethodHead, http.MethodPost,
			http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions,
		},
		AllowHeaders: []string{
			echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept,
			echo.HeaderAuthorization, echo.HeaderXRequestedWith,
		},
	}))

	// API routes (all prefixed with /api/v1).
	h := handlers.New(app)
	handlers.RegisterRoutes(e, h)

	// Static frontend — served from the embedded FS. Anything that isn't
	// /api/* gets the SPA index.html (client-side router takes over).
	webFS, _ := fs.Sub(embeddedWeb, "web")
	e.GET("/static/*", echo.WrapHandler(http.FileServer(http.FS(webFS))))
	e.GET("/*", func(c echo.Context) error {
		idx, err := fs.ReadFile(webFS, "index.html")
		if err != nil {
			return c.String(http.StatusInternalServerError, "web build missing — run `make build` first")
		}
		return c.Blob(http.StatusOK, "text/html; charset=utf-8", idx)
	})

	addr := app.Config.Server.Host + ":" + app.Config.PortStr()
	log.Printf("retrox: listening on http://%s", addr)
	if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
