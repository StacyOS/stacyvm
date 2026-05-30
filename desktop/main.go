package main

import (
	"context"
	"io/fs"
	"os"

	"github.com/StacyOs/stacyvm/web"
	"github.com/rs/zerolog"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

// App struct
type App struct {
	ctx      context.Context
	logger   zerolog.Logger
	shutdown func()
}

// NewApp creates a new App application struct
func NewApp() *App {
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
	return &App{
		logger: logger,
	}
}

// startup is called when the app starts.
// It will boot up the StacyVM API daemon in the background.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.logger.Info().Msg("starting StacyVM daemon...")

	shutdown, err := runDaemon(ctx, a.logger)
	if err != nil {
		a.logger.Fatal().Err(err).Msg("failed to start StacyVM daemon")
	}
	a.shutdown = shutdown
}

// beforeClose is called when the application is about to quit.
// We will trigger the daemon shutdown here.
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	if a.shutdown != nil {
		a.shutdown()
		a.shutdown = nil
	}
	return false
}

// shutdownHandler is called by Wails when the application is shutting down.
func (a *App) shutdownHandler(ctx context.Context) {
	if a.shutdown != nil {
		a.shutdown()
		a.shutdown = nil
	}
}

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Reuse the web UI assets embedded by the `web` package (web/embed.go's
	// `//go:embed all:out`). The frontend is built into web/out by the
	// `frontend:build` step in wails.json before this binary is compiled.
	assets, err := fs.Sub(web.Assets, "out")
	if err != nil {
		app.logger.Fatal().Err(err).Msg("failed to load embedded web assets")
	}

	// Create application with options
	err = wails.Run(&options.App{
		Title:  "StacyVM Desktop",
		Width:  1280,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 15, G: 23, B: 42, A: 1}, // Slate 900
		OnStartup:        app.startup,
		OnBeforeClose:    app.beforeClose,
		OnShutdown:       app.shutdownHandler,
		Linux: &linux.Options{
			WebviewGpuPolicy: linux.WebviewGpuPolicyOnDemand,
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		app.logger.Fatal().Err(err).Msg("wails run failed")
	}
}
