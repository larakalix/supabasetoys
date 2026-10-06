package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

type Engine struct {
	Store  *Store
	Runner Runner
}

func New(root string) (*Engine, error) { return WithRunner(root, SystemRunner{}) }
func WithRunner(root string, runner Runner) (*Engine, error) {
	if runner == nil {
		return nil, errors.New("process runner is required")
	}
	store, err := NewStore(root)
	if err != nil {
		return nil, err
	}
	return &Engine{Store: store, Runner: runner}, nil
}
func (e *Engine) mutate(work func() error) (err error) {
	lock, err := e.Store.Lock()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	return work()
}
func (e *Engine) Registry() (Registry, error) { return e.Store.Read() }
func (e *Engine) Configure(settings Settings) error {
	return e.mutate(func() error {
		if settings.DockerEndpoint != nil {
			if err := ValidateEndpoint(*settings.DockerEndpoint); err != nil {
				return err
			}
		}
		if settings.SupabaseCLI == "" || settings.DockerCLI == "" {
			return errors.New("executable names cannot be empty")
		}
		registry, err := e.Store.Read()
		if err != nil {
			return err
		}
		registry.Settings = settings
		return e.Store.Write(registry)
	})
}
func (e *Engine) Add(ctx context.Context, path, name, stackID string) (Project, error) {
	var project Project
	err := e.mutate(func() error {
		canonicalPath, err := canonical(path)
		if err != nil {
			return err
		}
		text, err := os.ReadFile(filepath.Join(canonicalPath, "supabase", "config.toml"))
		if err != nil {
			return fmt.Errorf("choose a folder containing supabase/config.toml: %w", err)
		}
		config, err := ParseConfig(string(text))
		if err != nil {
			return err
		}
		adapter := Adapter{Kind: "standard"}
		identitySuffix := "Standard"
		if stackID != "" {
			if !regexp.MustCompile(`^[0-9a-fA-F-]{8,}$`).MatchString(stackID) {
				return errors.New("use the full stack ID from supabase stack list --output-format json")
			}
			adapter = Adapter{Kind: "stack", StackID: stackID}
			identitySuffix = fmt.Sprintf("Stack { stack_id: %q }", stackID)
		} else if config.Experimental {
			return errors.New("this config enables experimental stacks; register with --stack-id or explicitly disable stack mode")
		}
		registry, err := e.Store.Read()
		if err != nil {
			return err
		}
		for _, existing := range registry.Projects {
			if existing.Path == canonicalPath && existing.Adapter == adapter {
				project = existing
				return nil
			}
		}
		if strings.TrimSpace(name) == "" {
			name = filepath.Base(canonicalPath)
		}
		project = Project{ID: Hash([]byte(canonicalPath + ":" + identitySuffix))[:16], Name: name, Path: canonicalPath, ProjectID: config.ProjectID, Adapter: adapter}
		if adapter.Kind == "stack" {
			version, err := e.version(ctx, registry.Settings)
			if err != nil {
				return err
			}
			if err := Supported(version); err != nil {
				return err
			}
			if err := e.verifyStack(ctx, registry.Settings, project); err != nil {
				return err
			}
		}
		registry.Projects = append(registry.Projects, project)
		return e.Store.Write(registry)
	})
	return project, err
}
func find(registry Registry, id string) (Project, error) {
	for _, project := range registry.Projects {
		if project.ID == id {
			return project, nil
		}
	}
	matches := []Project{}
	for _, project := range registry.Projects {
		if project.Name == id {
			matches = append(matches, project)
		}
	}
	if len(matches) > 1 {
		return Project{}, fmt.Errorf("more than one project is named %s; use the project ID", id)
	}
	if len(matches) == 0 {
		return Project{}, errors.New("project not registered; run project list or add its folder")
	}
	return matches[0], nil
}
func (e *Engine) Remove(id string) error {
	return e.mutate(func() error {
		registry, err := e.Store.Read()
		if err != nil {
			return err
		}
		project, err := find(registry, id)
		if err != nil {
			return err
		}
		registry.Projects = slices.DeleteFunc(registry.Projects, func(p Project) bool { return p.ID == project.ID })
		return e.Store.Write(registry)
	})
}
func (e *Engine) Inventory(ctx context.Context) (Inventory, error) {
	registry, err := e.Store.Read()
	if err != nil {
		return Inventory{}, err
	}
	return e.inventory(ctx, registry.Settings), nil
}
func readConfig(project Project) (Config, error) {
	text, err := os.ReadFile(filepath.Join(project.Path, "supabase", "config.toml"))
	if err != nil {
		return Config{}, fmt.Errorf("cannot read registered project %s; restore its config or remove the registration: %w", project.Name, err)
	}
	return ParseConfig(string(text))
}
func checkedConfig(registry Registry, project Project) (Config, error) {
	config, err := readConfig(project)
	if err != nil {
		return Config{}, err
	}
	if config.ProjectID != project.ProjectID {
		return Config{}, errors.New("project identity changed outside Toys; remove and re-add the folder to explicitly associate its current identity")
	}
	for _, other := range registry.Projects {
		if other.ID == project.ID {
			continue
		}
		otherConfig, err := readConfig(other)
		if err != nil {
			return Config{}, err
		}
		standard := project.Adapter.Kind == "standard" && other.Adapter.Kind == "standard"
		if standard && otherConfig.ProjectID == config.ProjectID {
			return Config{}, fmt.Errorf("duplicate project_id %q in %s and %s; lifecycle actions are blocked to protect their data; changing an ID does not migrate existing data", config.ProjectID, project.Name, other.Name)
		}
	}
	return config, nil
}
func ownership(inventory Inventory, project Project) error {
	for _, service := range selected(inventory, project) {
		if service.Workdir == nil {
			continue
		}
		path, err := canonical(*service.Workdir)
		if err != nil || path != project.Path {
			return errors.New("existing containers with this identity belong to another folder; no lifecycle action was performed")
		}
	}
	return nil
}
func portReservations(registry Registry, project Project, inventory Inventory) (map[uint16]bool, map[uint16]bool, error) {
	reserved := map[uint16]bool{}
	for _, other := range registry.Projects {
		if other.ID == project.ID {
			continue
		}
		config, err := readConfig(other)
		if err != nil {
			return nil, nil, err
		}
		for _, port := range config.Ports {
			reserved[port] = true
		}
	}
	ownedIDs := map[string]bool{}
	ownedPorts := map[uint16]bool{}
	for _, service := range selected(inventory, project) {
		ownedIDs[service.ID] = true
		if service.State == "running" {
			for _, port := range service.Ports {
				ownedPorts[port] = true
			}
		}
	}
	for _, service := range inventory.Services {
		if !ownedIDs[service.ID] {
			for _, port := range service.Ports {
				reserved[port] = true
			}
		}
	}
	return reserved, ownedPorts, nil
}
func (e *Engine) changes(registry Registry, project Project, inventory Inventory, config Config) ([]PortChange, error) {
	reserved, owned, err := portReservations(registry, project, inventory)
	if err != nil {
		return nil, err
	}
	return Allocate(config, reserved, func(port uint16) bool { return owned[port] || PortAvailable(port) })
}
func issue(code, message, remedy string, repairable bool) Diagnostic {
	return Diagnostic{Code: code, Severity: "error", Message: Redact(message), Remedy: remedy, Repairable: repairable}
}
func (e *Engine) Status(ctx context.Context, id string, sampleResources bool) (Status, error) {
	registry, err := e.Store.Read()
	if err != nil {
		return Status{}, err
	}
	project, err := find(registry, id)
	if err != nil {
		return Status{}, err
	}
	inventory := e.inventory(ctx, registry.Settings)
	services := selected(inventory, project)
	status := Status{Project: project, Services: services, Diagnostics: []Diagnostic{}, Endpoints: map[string]string{}, ConfiguredPorts: map[string]uint16{}, Resources: []ResourceReading{}}
	version, err := e.version(ctx, registry.Settings)
	if err != nil {
		status.Diagnostics = append(status.Diagnostics, issue("missing_cli", err.Error(), "Install the official Supabase CLI or select its executable in Settings.", false))
	} else {
		status.SupabaseVersion = &version
		if err := Supported(version); err != nil {
			status.Diagnostics = append(status.Diagnostics, issue("unsupported_cli", err.Error(), "Select Supabase CLI 2.119.0 or 2.118.0 in Settings.", false))
		}
	}
	if !inventory.Available {
		message := "Docker unavailable"
		if inventory.Error != nil {
			message = *inventory.Error
		}
		status.Diagnostics = append(status.Diagnostics, issue("docker_unavailable", message, "Start Docker and select a local socket or named-pipe context.", false))
	}
	if err := ownership(inventory, project); err != nil {
		status.Diagnostics = append(status.Diagnostics, issue("identity_conflict", err.Error(), "Associate the correct folder with the existing stack.", false))
	}
	config, err := checkedConfig(registry, project)
	if err != nil {
		status.Diagnostics = append(status.Diagnostics, issue("invalid_identity_or_config", err.Error(), "Fix the identified config or registration issue before starting.", false))
	} else {
		status.ConfiguredPorts = config.Ports
		if config.Experimental && project.Adapter.Kind == "standard" {
			status.Diagnostics = append(status.Diagnostics, issue("adapter_changed", "Experimental mode was enabled outside Toys.", "Remove and re-register explicitly with a stack ID.", false))
		}
		if inventory.Available && project.Adapter.Kind == "standard" {
			changes, err := e.changes(registry, project, inventory, config)
			if err != nil {
				status.Diagnostics = append(status.Diagnostics, issue("allocation_failed", err.Error(), "Restore configs for all registered projects.", false))
			} else {
				for _, change := range changes {
					status.Diagnostics = append(status.Diagnostics, issue("port_conflict", fmt.Sprintf("%s requests port %d, which is occupied or reserved", change.Key, change.Before), "Review a port repair preview. App environment URLs may also need updating.", true))
				}
			}
		}
	}
	running := 0
	unhealthy := false
	for _, service := range services {
		if service.State == "running" {
			running++
		}
		badHealth := service.Health != nil && *service.Health == "unhealthy"
		failedState := slices.Contains([]string{"restarting", "dead", "exited"}, service.State)
		if badHealth || failedState {
			status.Diagnostics = append(status.Diagnostics, issue("unhealthy_service", service.Name+" is "+service.State, "Inspect this service's logs; restart only this project if needed.", false))
		}
		if service.State != "running" || badHealth {
			unhealthy = true
		}
	}
	if sampleResources && inventory.Available {
		readings, err := e.resources(ctx, registry.Settings, *inventory.Endpoint, services)
		if err != nil {
			status.ResourcesError = pointer(Redact(err.Error()))
		} else {
			status.Resources = readings
		}
	}
	var cpu float64
	var memory uint64
	for _, reading := range status.Resources {
		if reading.CPUPercent != nil {
			cpu += *reading.CPUPercent
		}
		if reading.MemoryBytes != nil {
			memory += *reading.MemoryBytes
		}
	}
	if cpu >= 200 || memory >= 4*1024*1024*1024 {
		status.Diagnostics = append(status.Diagnostics, Diagnostic{Code: "resource_pressure", Severity: "warning", Message: "This project exceeds 200% CPU or 4 GiB RAM; this is a usage signal, not proof the host is exhausted.", Remedy: "Check Docker resource limits and stop unused projects explicitly."})
	}
	status.State = "running"
	if unhealthy {
		status.State = "unhealthy"
	}
	if running == 0 {
		status.State = "stopped"
	}
	if !inventory.Available {
		status.State = "unavailable"
	}
	for _, diagnostic := range status.Diagnostics {
		if slices.Contains([]string{"identity_conflict", "invalid_identity_or_config", "adapter_changed"}, diagnostic.Code) {
			status.State = "unavailable"
		}
	}
	if status.State == "running" && status.SupabaseVersion != nil && Supported(*status.SupabaseVersion) == nil {
		values, err := e.connections(ctx, registry.Settings, *inventory.Endpoint, project)
		if err != nil {
			status.Diagnostics = append(status.Diagnostics, issue("status_failed", err.Error(), "Inspect service logs before using the connection details.", false))
		} else {
			for key, value := range values {
				if strings.HasSuffix(key, "URL") && !Privileged(key) && !PrivilegedValue(value) {
					status.Endpoints[key] = value
				}
			}
		}
	}
	return status, nil
}
func (e *Engine) ready(ctx context.Context, registry Registry, id string) (Project, Inventory, error) {
	project, err := find(registry, id)
	if err != nil {
		return Project{}, Inventory{}, err
	}
	if _, err := checkedConfig(registry, project); err != nil {
		return Project{}, Inventory{}, err
	}
	version, err := e.version(ctx, registry.Settings)
	if err != nil {
		return Project{}, Inventory{}, err
	}
	if err := Supported(version); err != nil {
		return Project{}, Inventory{}, err
	}
	if err := e.verifyStack(ctx, registry.Settings, project); err != nil {
		return Project{}, Inventory{}, err
	}
	inventory := e.inventory(ctx, registry.Settings)
	if !inventory.Available {
		message := "Docker unavailable"
		if inventory.Error != nil {
			message = *inventory.Error
		}
		return Project{}, inventory, errors.New(message)
	}
	return project, inventory, ownership(inventory, project)
}
func (e *Engine) pin(ctx context.Context, registry *Registry) error {
	if registry.Settings.DockerEndpoint != nil {
		return nil
	}
	endpoint, err := e.endpoint(ctx, registry.Settings)
	if err != nil {
		return err
	}
	registry.Settings.DockerEndpoint = &endpoint
	return e.Store.Write(*registry)
}
func (e *Engine) PreviewRepair(ctx context.Context, id string) (RepairPreview, error) {
	registry, err := e.Store.Read()
	if err != nil {
		return RepairPreview{}, err
	}
	project, inventory, err := e.ready(ctx, registry, id)
	if err != nil {
		return RepairPreview{}, err
	}
	if project.Adapter.Kind != "standard" {
		return RepairPreview{}, errors.New("experimental stacks allocate their own ports; configuration migration is not automated")
	}
	config, err := checkedConfig(registry, project)
	if err != nil {
		return RepairPreview{}, err
	}
	if config.Experimental {
		return RepairPreview{}, errors.New("config adapter changed; re-register the project")
	}
	changes, err := e.changes(registry, project, inventory, config)
	if err != nil {
		return RepairPreview{}, err
	}
	preview := RepairPreview{Project: project.ID, ConfigHash: Hash([]byte(config.Text)), Changes: changes}
	for _, service := range selected(inventory, project) {
		if service.State == "running" {
			preview.RestartRequired = true
		}
	}
	return preview, nil
}
func (e *Engine) ApplyRepair(ctx context.Context, preview RepairPreview, restart bool) (Status, error) {
	var status Status
	err := e.mutate(func() error {
		registry, err := e.Store.Read()
		if err != nil {
			return err
		}
		if err := e.pin(ctx, &registry); err != nil {
			return err
		}
		project, inventory, err := e.ready(ctx, registry, preview.Project)
		if err != nil {
			return err
		}
		config, err := checkedConfig(registry, project)
		if err != nil {
			return err
		}
		if Hash([]byte(config.Text)) != preview.ConfigHash {
			return errors.New("configuration changed since preview; review a fresh preview")
		}
		if project.Adapter.Kind != "standard" || config.Experimental {
			return errors.New("only standard projects support port repair")
		}
		changes, err := e.reviewedChanges(registry, project, inventory, config, preview)
		if err != nil {
			return err
		}
		if !slices.Equal(changes, preview.Changes) {
			return errors.New("port availability changed since preview; review a fresh preview")
		}
		running := false
		for _, service := range selected(inventory, project) {
			running = running || service.State == "running"
		}
		if running && !restart {
			return errors.New("this repair requires a targeted restart; pass --restart or stop this project first")
		}
		if len(changes) > 0 {
			updates := map[string]any{}
			for _, change := range changes {
				updates[change.Key] = int64(change.After)
			}
			next, err := Patch(config, updates)
			if err != nil {
				return err
			}
			path := filepath.Join(project.Path, "supabase", "config.toml")
			if err := backup(path, []byte(config.Text)); err != nil {
				return err
			}
			if running {
				if err := e.execute(ctx, registry, project, inventory, "stop"); err != nil {
					return err
				}
			}
			current, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("recheck config during repair: %w", err)
			}
			if Hash(current) != preview.ConfigHash {
				return errors.New("configuration changed during repair; target is stopped, review a fresh preview")
			}
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("repair cancelled before config write: %w", err)
			}
			if preview.Mode == "manual" {
				fresh := e.inventory(ctx, registry.Settings)
				if !fresh.Available {
					return errors.New("Docker became unavailable; refresh before applying ports")
				}
				if err := ownership(fresh, project); err != nil {
					return err
				}
				if _, err := e.reviewedChanges(registry, project, fresh, config, preview); err != nil {
					return fmt.Errorf("port availability changed before write: %w", err)
				}
			}
			current, err = os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("recheck config before write: %w", err)
			}
			if Hash(current) != preview.ConfigHash {
				return errors.New("configuration changed before write; review a fresh preview")
			}
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("port change cancelled before write: %w", err)
			}
			if err := AtomicWrite(path, []byte(next)); err != nil {
				return err
			}
			if running {
				if err := e.startChecked(ctx, registry, project, e.inventory(ctx, registry.Settings)); err != nil {
					return err
				}
			}
		}
		status, err = e.Status(ctx, project.ID, false)
		return err
	})
	return status, err
}
func (e *Engine) execute(ctx context.Context, registry Registry, project Project, inventory Inventory, operation string) error {
	if !inventory.Available || inventory.Endpoint == nil {
		return errors.New("Docker endpoint unavailable")
	}
	_, err := require(e.Runner.Run(ctx, supabase(registry.Settings, *inventory.Endpoint, project, operation)))
	if err == nil {
		return nil
	}
	// Cancellation ends the child command; a separate bounded read reconciles any
	// services that Docker may already have launched, without stopping them.
	reconcile, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	services := selected(e.inventory(reconcile, registry.Settings), project)
	running := 0
	for _, service := range services {
		if service.State == "running" {
			running++
		}
	}
	return fmt.Errorf("%s; reconciled selected project: %d known services, %d running; refresh status before retrying", Redact(err.Error()), len(services), running)
}
func (e *Engine) startChecked(ctx context.Context, registry Registry, project Project, inventory Inventory) error {
	if !inventory.Available {
		return errors.New("Docker became unavailable; refresh status")
	}
	if err := ownership(inventory, project); err != nil {
		return err
	}
	config, err := checkedConfig(registry, project)
	if err != nil {
		return err
	}
	if project.Adapter.Kind == "standard" {
		if config.Experimental {
			return errors.New("adapter changed; explicitly re-register this project")
		}
		changes, err := e.changes(registry, project, inventory, config)
		if err != nil {
			return err
		}
		if len(changes) > 0 {
			return errors.New("port conflicts detected; run doctor --preview and review a repair before starting")
		}
	}
	return e.execute(ctx, registry, project, inventory, "start")
}
func (e *Engine) Lifecycle(ctx context.Context, id, operation string, progress func(OperationProgress)) (Status, error) {
	var status Status
	if !slices.Contains([]string{"start", "stop", "restart"}, operation) {
		return status, errors.New("only targeted start, stop, and restart are supported")
	}
	if progress == nil {
		progress = func(OperationProgress) {}
	}
	err := e.mutate(func() error {
		registry, err := e.Store.Read()
		if err != nil {
			return err
		}
		if err := e.pin(ctx, &registry); err != nil {
			return err
		}
		project, inventory, err := e.ready(ctx, registry, id)
		if err != nil {
			return err
		}
		progress(OperationProgress{Project: project.ID, Phase: "preflight", Message: "Identity, CLI compatibility, and local Docker target verified"})
		if operation == "stop" || operation == "restart" {
			progress(OperationProgress{Project: project.ID, Phase: "stopping", Message: "Stopping only the selected project, preserving data"})
			if err := e.execute(ctx, registry, project, inventory, "stop"); err != nil {
				return err
			}
		}
		if operation == "start" || operation == "restart" {
			progress(OperationProgress{Project: project.ID, Phase: "starting", Message: "Rechecking ports and starting the selected project"})
			if err := e.startChecked(ctx, registry, project, e.inventory(ctx, registry.Settings)); err != nil {
				return err
			}
		}
		status, err = e.Status(ctx, project.ID, false)
		if err == nil {
			progress(OperationProgress{Project: project.ID, Phase: "complete", Message: "Project is " + status.State})
		}
		return err
	})
	return status, err
}
func (e *Engine) ConnectionValues(ctx context.Context, id string) (map[string]string, error) {
	registry, err := e.Store.Read()
	if err != nil {
		return nil, err
	}
	project, inventory, err := e.ready(ctx, registry, id)
	if err != nil {
		return nil, err
	}
	return e.connections(ctx, registry.Settings, *inventory.Endpoint, project)
}
func envPath(project Project, file string) (string, error) {
	if file == "" || filepath.IsAbs(file) || strings.Contains(file, "\\") {
		return "", errors.New("environment file must be a relative path inside the registered project")
	}
	for _, part := range strings.Split(file, "/") {
		if part == ".." || part == "." || part == "" {
			return "", errors.New("environment file must be a relative path inside the registered project")
		}
	}
	path := filepath.Join(project.Path, file)
	parent, err := canonical(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(project.Path, parent)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("environment path cannot escape the project")
	}
	info, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect environment path: %w", err)
	}
	if info != nil && info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("environment path cannot use a symlink")
	}
	return path, nil
}
func readOptional(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []byte{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read environment file: %w", err)
	}
	return data, nil
}
func (e *Engine) PreviewEnv(ctx context.Context, id, file string, mapping map[string]string) (EnvPreview, error) {
	registry, err := e.Store.Read()
	if err != nil {
		return EnvPreview{}, err
	}
	project, err := find(registry, id)
	if err != nil {
		return EnvPreview{}, err
	}
	path, err := envPath(project, file)
	if err != nil {
		return EnvPreview{}, err
	}
	original, err := readOptional(path)
	if err != nil {
		return EnvPreview{}, err
	}
	values, err := e.ConnectionValues(ctx, id)
	if err != nil {
		return EnvPreview{}, err
	}
	updates, err := MappedValues(values, mapping)
	if err != nil {
		return EnvPreview{}, err
	}
	if _, err := MergeEnv(string(original), updates); err != nil {
		return EnvPreview{}, err
	}
	return EnvPreview{Project: project.ID, File: file, SourceHash: Hash(original), Updates: updates}, nil
}
func (e *Engine) ApplyEnv(ctx context.Context, preview EnvPreview, mapping map[string]string) error {
	return e.mutate(func() error {
		registry, err := e.Store.Read()
		if err != nil {
			return err
		}
		project, err := find(registry, preview.Project)
		if err != nil {
			return err
		}
		path, err := envPath(project, preview.File)
		if err != nil {
			return err
		}
		original, err := readOptional(path)
		if err != nil {
			return err
		}
		if Hash(original) != preview.SourceHash {
			return errors.New("environment file changed since preview; generate a fresh preview")
		}
		values, err := e.ConnectionValues(ctx, preview.Project)
		if err != nil {
			return err
		}
		updates, err := MappedValues(values, mapping)
		if err != nil {
			return err
		}
		if !maps.Equal(updates, preview.Updates) {
			return errors.New("connection values changed since preview; generate a fresh preview")
		}
		if _, err := os.Stat(path); err == nil {
			if err := backup(path, original); err != nil {
				return err
			}
		}
		merged, err := MergeEnv(string(original), updates)
		if err != nil {
			return err
		}
		current, err := readOptional(path)
		if err != nil {
			return err
		}
		if Hash(current) != preview.SourceHash {
			return errors.New("environment file changed during export; generate a fresh preview")
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("environment export cancelled: %w", err)
		}
		return AtomicWrite(path, []byte(merged))
	})
}

type LogOptions struct {
	Service string
	Tail    int
	Since   string
}

func (e *Engine) Logs(ctx context.Context, id string, options LogOptions) (string, error) {
	registry, err := e.Store.Read()
	if err != nil {
		return "", err
	}
	project, inventory, err := e.ready(ctx, registry, id)
	if err != nil {
		return "", err
	}
	services := selected(inventory, project)
	if options.Service != "" && !slices.ContainsFunc(services, func(service Service) bool { return service.ID == options.Service || service.Name == options.Service }) {
		return "", errors.New("service is not part of the selected project")
	}
	var result strings.Builder
	for _, service := range services {
		if options.Service != "" && service.ID != options.Service && service.Name != options.Service {
			continue
		}
		spec := docker(registry.Settings, *inventory.Endpoint, "logs", "--timestamps", "--tail", fmt.Sprint(max(0, min(options.Tail, 1000))))
		if options.Since != "" {
			spec.Args = append(spec.Args, "--since", options.Since)
		}
		spec.Args = append(spec.Args, service.ID)
		output, err := e.Runner.Run(ctx, spec)
		if err != nil {
			return "", err
		}
		if !output.Success {
			return "", errors.New(Redact(output.Stderr))
		}
		result.WriteString("--- " + service.Name + " ---\n" + Redact(output.Stdout) + Redact(output.Stderr) + "\n")
		if result.Len() > 4*1024*1024 {
			break
		}
	}
	return result.String(), nil
}
func (e *Engine) DiagnosticReport(ctx context.Context, id string, includeLogs bool) (string, error) {
	status, err := e.Status(ctx, id, true)
	if err != nil {
		return "", err
	}
	logs := "Not included"
	if includeLogs {
		logs, err = e.Logs(ctx, id, LogOptions{Tail: 50})
		if err != nil {
			logs = "Logs unavailable: " + Redact(err.Error())
		}
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		return "", fmt.Errorf("encode diagnostic status: %w", err)
	}
	data := map[string]any{}
	if err := json.Unmarshal(encoded, &data); err != nil {
		return "", fmt.Errorf("decode diagnostic status: %w", err)
	}
	data["logs"] = Redact(logs)
	if project, ok := data["project"].(map[string]any); ok {
		delete(project, "path")
	}
	if services, ok := data["services"].([]any); ok {
		for _, service := range services {
			if row, ok := service.(map[string]any); ok {
				delete(row, "workdir")
			}
		}
	}
	encoded, err = json.MarshalIndent(redactJSON(data), "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode redacted report: %w", err)
	}
	return strings.ReplaceAll(string(encoded), status.Project.Path, "[PROJECT]"), nil
}
