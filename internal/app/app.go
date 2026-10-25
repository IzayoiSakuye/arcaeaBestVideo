package app

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"rhythm-game-bvideo/internal/catalog"
)

type App struct {
	ctx         context.Context
	root        string
	mu          sync.Mutex
	jobID       string
	cancel      context.CancelFunc
	cataOnce    sync.Once
	cata        *catalog.Catalog
	cataErr     error
	previews    map[string]string
	videoCancel context.CancelFunc
}

func New() *App {
	root, _ := os.Getwd()
	if found := findRoot(root); found != "" {
		root = found
	} else if exe, err := os.Executable(); err == nil {
		if found := findRoot(filepath.Dir(exe)); found != "" {
			root = found
		}
	}
	return &App{root: root}
}

func findRoot(start string) string {
	for dir := start; dir != ""; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "data", "arcaea-catalog.json")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "templates", "text.html")); err == nil {
				return dir
			}
		}
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
	}
	return ""
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) Version() string {
	return "v0.0.1"
}

func (a *App) context() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (a *App) catalog() (*catalog.Catalog, error) {
	a.cataOnce.Do(func() {
		a.cata, a.cataErr = catalog.Load(filepath.Join(a.root, "data", "arcaea-catalog.json"), 1)
	})
	return a.cata, a.cataErr
}
