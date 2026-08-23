package main

import (
	"embed"
	"log"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ahmed/odoonoir/internal/config"
	"github.com/ahmed/odoonoir/internal/service"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	os.Setenv("GDK_BACKEND", "x11")
	os.Setenv("WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS", "1")
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("load config: ", err)
	}
	svc, err := service.New(cfg)
	if err != nil {
		log.Fatal("start service: ", err)
	}
	app := application.New(application.Options{
		Name:        "odoonoir",
		Description: "Odoo instance manager",
		LogLevel:    slog.LevelDebug,
		Services: []application.Service{
			application.NewService(NewApp(svc)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Odoonoir",
		Width:  1280,
		Height: 800,
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
