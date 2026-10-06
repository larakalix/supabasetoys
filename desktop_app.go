//go:build desktop || bindings

package main

import (
	"context"
	"errors"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"supabasetoys/pkg/engine"
)

// App binds a narrow, typed engine API rather than exposing a generic shell.
type App struct {
	engine *engine.Engine
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
}

func (a *App) startup(ctx context.Context) { a.mu.Lock(); a.ctx = ctx; a.mu.Unlock() }
func (a *App) shutdown(_ context.Context)  { a.Cancel() }
func (a *App) Cancel() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}
func (a *App) Request(request engine.Request) (any, error) {
	a.mu.Lock()
	ctx := a.ctx
	if ctx == nil {
		a.mu.Unlock()
		return nil, errors.New("desktop runtime is not ready")
	}
	if request.Mutating() {
		if a.cancel != nil {
			a.mu.Unlock()
			return nil, errors.New("another desktop operation is active")
		}
		ctx, a.cancel = context.WithCancel(ctx)
	}
	a.mu.Unlock()
	if request.Mutating() {
		defer func() { a.mu.Lock(); a.cancel(); a.cancel = nil; a.mu.Unlock() }()
	}
	if request.Action == "open_endpoint" {
		status, err := a.engine.Status(ctx, request.Project, false)
		if err != nil {
			return nil, errors.New(engine.Redact(err.Error()))
		}
		url, err := engine.LocalHTTPURL(status.Endpoints[request.Key])
		if err != nil {
			return nil, err
		}
		runtime.BrowserOpenURL(ctx, url)
		return map[string]bool{"ok": true}, nil
	}
	value, err := a.engine.Handle(ctx, request, func(progress engine.OperationProgress) { runtime.EventsEmit(ctx, "operation-progress", progress) })
	if err != nil {
		return nil, errors.New(engine.Redact(err.Error()))
	}
	return value, nil
}
func (a *App) PickFolder() (string, error) {
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{Title: "Choose a Supabase project folder"})
}
func (a *App) SaveDiagnosticsPath() (string, error) {
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	return runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{Title: "Export redacted diagnostics", DefaultFilename: "supabase-toys-diagnostics.json", Filters: []runtime.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}}})
}
func (a *App) CopyText(value string) error {
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	return runtime.ClipboardSetText(ctx, value)
}
