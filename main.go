package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"quickcull/internal/domain"
	"quickcull/internal/frontendassets"
	"quickcull/internal/review"
	"quickcull/internal/utils"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var version = domain.AppVersion

func main() {
	defer utils.HandlePanic()

	path := flag.String("p", "", "Folder path to review")
	debug := flag.Bool("debug", false, "Enable debug logging")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.BoolVar(showVersion, "v", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		if _, err := os.Stdout.WriteString(domain.AppName + " " + version + "\n"); err != nil {
			_, _ = os.Stderr.WriteString(domain.AppName + "\n")
		}
		os.Exit(0)
	}

	utils.SetupGlobalLogging(*debug, domain.GetLogPath())
	defer utils.FlushLogs()

	// Initialize the review server (logic part)
	srv := review.NewServer()
	if *path != "" {
		if err := srv.LoadState(*path); err != nil {
			slog.Error("Failed to load folder", "path", *path, "error", err)
		}
	}

	// Create the Wails application
	app := review.NewApp(srv)

	cfg := domain.GetConfig()
	windowState := options.Normal
	if cfg.WindowIsFullscreen {
		windowState = options.Fullscreen
	} else if cfg.WindowIsMaximized {
		windowState = options.Maximised
	}

	width := cfg.WindowWidth
	if width < 800 {
		width = 1280
	}
	height := cfg.WindowHeight
	if height < 600 {
		height = 800
	}

	// Configure Wails options
	appOptions := &options.App{
		Title:  domain.AppName + " v" + version,
		Width:  width,
		Height: height,
		MinWidth: 800,
		MinHeight: 600,
		WindowStartState: windowState,
		AssetServer: &assetserver.Options{
			Assets:  frontendassets.Assets,
			Handler: http.StripPrefix("/raw-media", http.HandlerFunc(srv.ServeMedia)),
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Prevent caching of the main entry point and HTML files to ensure updates are picked up.
					// JS and CSS files are hashed by Vite, so they can be cached safely.
					if r.URL.Path == "/" || strings.HasSuffix(r.URL.Path, ".html") {
						w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
						w.Header().Set("Pragma", "no-cache")
						w.Header().Set("Expires", "0")
					}
					next.ServeHTTP(w, r)
				})
			},
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup: func(ctx context.Context) {
			if cfg.WindowX >= 0 && cfg.WindowY >= 0 {
				wailsruntime.WindowSetPosition(ctx, cfg.WindowX, cfg.WindowY)
			}
			if err := app.Startup(ctx); err != nil {
				slog.Error("Startup failed", "error", err)
			}
		},
		OnBeforeClose: func(ctx context.Context) bool {
			isMaximized := wailsruntime.WindowIsMaximised(ctx)
			isFullscreen := wailsruntime.WindowIsFullscreen(ctx)
			w, h := wailsruntime.WindowGetSize(ctx)
			x, y := wailsruntime.WindowGetPosition(ctx)

			c := domain.GetConfig()
			c.WindowIsMaximized = isMaximized
			c.WindowIsFullscreen = isFullscreen

			// Only update width/height and position if the window is in a normal state
			// to preserve the restored size/position.
			if !isMaximized && !isFullscreen {
				if w >= 800 && h >= 600 {
					c.WindowWidth = w
					c.WindowHeight = h
					// Only persist position for primary-screen placements to avoid
					// off-screen window on monitor disconnect.
					if x >= 0 && y >= 0 {
						c.WindowX = x
						c.WindowY = y
					}
				}
			}

			if err := domain.UpdateConfig(c); err != nil {
				slog.Error("Failed to save config on close", "error", err)
			}
			app.FlushPersistence()
			return false
		},
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewUserDataPath: filepath.Join(domain.GetAppCacheDir(), "webview"),
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarDefault(),
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			About: &mac.AboutInfo{
				Title:   domain.AppName,
				Message: "The fastest way to cull and review your local photos.",
			},
		},
	}

	err := wails.Run(appOptions)

	if err != nil {
		slog.Error("Wails error", "error", err)
		os.Exit(1)
	}
}
