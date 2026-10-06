//go:build desktop || bindings

package main

import (
	"embed"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"supabasetoys/pkg/engine"
)

//go:embed all:apps/desktop/dist
var assets embed.FS

func main() {
	core, err := engine.New(os.Getenv("TOYS_DATA_DIR"))
	if err != nil {
		slog.Error("initialize desktop engine", "error", engine.Redact(err.Error()))
		os.Exit(1)
	}
	app := &App{engine: core}
	err = wails.Run(&options.App{
		Title: "Supabase Toys", Width: 1180, Height: 780, MinWidth: 720, MinHeight: 560,
		BackgroundColour: options.NewRGB(20, 25, 22),
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup, OnShutdown: app.shutdown, Bind: []any{app},
	})
	if err != nil {
		slog.Error("run desktop app", "error", engine.Redact(err.Error()))
		os.Exit(1)
	}
}
