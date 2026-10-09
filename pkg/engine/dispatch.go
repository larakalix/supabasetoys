package engine

import (
	"context"
	"encoding/json"
	"errors"
)

// Request is the shared JSON contract exposed by the desktop bridge and fixtures.
type Request struct {
	Account     string            `json:"account"`
	Token       string            `json:"token"`
	CloudRef    string            `json:"cloud_ref"`
	Refresh     bool              `json:"refresh"`
	SessionOnly bool              `json:"session_only"`
	Ports       map[string]uint16 `json:"ports"`
	Action      string            `json:"action"`
	Project     string            `json:"project"`
	Operation   string            `json:"operation"`
	Path        string            `json:"path"`
	Name        string            `json:"name"`
	StackID     string            `json:"stack_id"`
	NewID       string            `json:"new_id"`
	Settings    Settings          `json:"settings"`
	Resources   bool              `json:"resources"`
	Restart     bool              `json:"restart"`
	Preview     json.RawMessage   `json:"preview"`
	File        string            `json:"file"`
	Mapping     map[string]string `json:"mapping"`
	Service     string            `json:"service"`
	Since       string            `json:"since"`
	IncludeLogs bool              `json:"include_logs"`
	Key         string            `json:"key"`
}

func (request Request) Mutating() bool {
	switch request.Action {
	case "account_add", "account_remove", "associate", "unassociate", "identity_apply", "lifecycle", "repair_apply", "env_apply", "configure", "add", "remove":
		return true
	}
	return false
}
func (e *Engine) Handle(ctx context.Context, request Request, progress func(OperationProgress)) (any, error) {
	switch request.Action {
	case "accounts":
		return e.Accounts()
	case "account_add":
		return e.AddAccount(ctx, request.Name, request.Token, request.SessionOnly)
	case "account_remove":
		return map[string]bool{"ok": true}, e.RemoveAccount(request.Account)
	case "cloud_list":
		return e.CloudList(ctx, request.Account, request.Refresh)
	case "associate":
		return e.Associate(ctx, request.Project, request.Account, request.CloudRef)
	case "unassociate":
		return map[string]bool{"ok": true}, e.Unassociate(request.Project)
	case "association_suggestions":
		return e.AssociationSuggestions(request.Project)
	case "registry":
		return e.Registry()
	case "inventory":
		return e.Inventory(ctx)
	case "configure":
		return map[string]bool{"ok": true}, e.Configure(request.Settings)
	case "add":
		return e.Add(ctx, request.Path, request.Name, request.StackID)
	case "remove":
		return map[string]bool{"ok": true}, e.Remove(request.Project)
	case "status":
		return e.Status(ctx, request.Project, request.Resources)
	case "lifecycle":
		return e.Lifecycle(ctx, request.Project, request.Operation, progress)
	case "ports_preview":
		return e.PreviewPorts(ctx, request.Project, request.Ports)
	case "repair_preview":
		return e.PreviewRepair(ctx, request.Project)
	case "repair_apply":
		var preview RepairPreview
		if err := json.Unmarshal(request.Preview, &preview); err != nil {
			return nil, errors.New("invalid repair preview")
		}
		if progress != nil {
			progress(OperationProgress{Project: preview.Project, Phase: "repairing", Message: "Applying reviewed repair to the selected project"})
		}
		return e.ApplyRepair(ctx, preview, request.Restart)
	case "identity_preview":
		return e.PreviewIdentity(ctx, request.Project, request.NewID)
	case "identity_apply":
		var preview IdentityPreview
		if err := json.Unmarshal(request.Preview, &preview); err != nil {
			return nil, errors.New("invalid identity preview")
		}
		return e.ApplyIdentity(ctx, preview)
	case "connections":
		return e.ConnectionValues(ctx, request.Project)
	case "env_preview":
		return e.PreviewEnv(ctx, request.Project, request.File, request.Mapping)
	case "env_apply":
		var preview EnvPreview
		if err := json.Unmarshal(request.Preview, &preview); err != nil {
			return nil, errors.New("invalid environment preview")
		}
		return map[string]bool{"ok": true}, e.ApplyEnv(ctx, preview, request.Mapping)
	case "logs":
		logs, err := e.Logs(ctx, request.Project, LogOptions{Service: request.Service, Since: request.Since, Tail: 200})
		return map[string]string{"logs": logs}, err
	case "diagnostics":
		report, err := e.DiagnosticReport(ctx, request.Project, request.IncludeLogs)
		return map[string]string{"report": report}, err
	case "export_diagnostics":
		report, err := e.DiagnosticReport(ctx, request.Project, request.IncludeLogs)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, AtomicWrite(request.Path, []byte(report))
	}
	return nil, errors.New("unsupported desktop action")
}
