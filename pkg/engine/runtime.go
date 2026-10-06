package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

var supportedVersions = []string{"2.119.0", "2.118.0"}

func Supported(version string) error {
	if !slices.Contains(supportedVersions, version) {
		return fmt.Errorf("Supabase CLI %s is not in the tested compatibility matrix (2.119.0, 2.118.0); select a supported binary", version)
	}
	return nil
}
func (e *Engine) version(ctx context.Context, settings Settings) (string, error) {
	text, err := require(e.Runner.Run(ctx, command(settings.SupabaseCLI, "--version")))
	if err != nil {
		return "", err
	}
	version := strings.TrimPrefix(strings.TrimSpace(text), "v")
	if !regexp.MustCompile(`^\d+\.\d+\.\d+(?:[-+][A-Za-z0-9.-]+)?$`).MatchString(version) {
		return "", errors.New("cannot parse Supabase CLI version")
	}
	return version, nil
}
func ValidateEndpoint(endpoint string) error {
	if strings.ContainsAny(endpoint, "\n\r\x00") {
		return errors.New("invalid Docker endpoint")
	}
	if strings.HasPrefix(endpoint, "unix:///") {
		return nil
	}
	if runtime.GOOS == "windows" && strings.HasPrefix(endpoint, "npipe:////./pipe/") {
		return nil
	}
	return errors.New("only local Docker Unix sockets or Windows named pipes are supported; remote/TCP contexts are refused")
}
func LocalHTTPURL(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse local endpoint: %w", err)
	}
	scheme := parsed.Scheme == "http" || parsed.Scheme == "https"
	local := slices.Contains([]string{"localhost", "127.0.0.1", "::1"}, parsed.Hostname())
	if !scheme || !local || parsed.User != nil {
		return "", errors.New("only local HTTP endpoints can be opened")
	}
	return parsed.String(), nil
}
func (e *Engine) endpoint(ctx context.Context, settings Settings) (string, error) {
	if settings.DockerEndpoint != nil {
		if err := ValidateEndpoint(*settings.DockerEndpoint); err != nil {
			return "", err
		}
		return *settings.DockerEndpoint, nil
	}
	text, err := require(e.Runner.Run(ctx, command(settings.DockerCLI, "context", "inspect")))
	if err != nil {
		return "", err
	}
	var contexts []struct {
		Endpoints map[string]struct{ Host string }
	}
	if err := json.Unmarshal([]byte(text), &contexts); err != nil {
		return "", fmt.Errorf("invalid Docker context output: %w", err)
	}
	if len(contexts) == 0 {
		return "", errors.New("Docker context has no endpoint")
	}
	host := contexts[0].Endpoints["docker"].Host
	return host, ValidateEndpoint(host)
}
func docker(settings Settings, endpoint string, args ...string) CommandSpec {
	spec := command(settings.DockerCLI, "--host", endpoint)
	spec.Args = append(spec.Args, args...)
	return spec
}
func (e *Engine) inventory(ctx context.Context, settings Settings) Inventory {
	result := Inventory{Services: []Service{}}
	endpoint, err := e.endpoint(ctx, settings)
	if err != nil {
		result.Error = pointer(Redact(err.Error()))
		return result
	}
	result.Endpoint = &endpoint
	ids, err := require(e.Runner.Run(ctx, docker(settings, endpoint, "ps", "-a", "--no-trunc", "--format", "{{.ID}}")))
	if err != nil {
		result.Error = pointer(Redact(err.Error()))
		return result
	}
	if strings.TrimSpace(ids) == "" {
		result.Available = true
		return result
	}
	spec := docker(settings, endpoint, "inspect")
	spec.Args = append(spec.Args, strings.Fields(ids)...)
	text, err := require(e.Runner.Run(ctx, spec))
	if err != nil {
		result.Error = pointer(Redact(err.Error()))
		return result
	}
	services, err := ParseInventory(text)
	if err != nil {
		result.Error = pointer(Redact(err.Error()))
		return result
	}
	result.Available = true
	result.Services = services
	return result
}
func ParseInventory(text string) ([]Service, error) {
	var containers []struct {
		ID         string `json:"Id"`
		Name       string
		Config     struct{ Labels map[string]string }
		HostConfig struct {
			PortBindings map[string][]struct{ HostPort string }
		}
		State struct {
			Status string
			Health *struct{ Status string }
		}
	}
	if err := json.Unmarshal([]byte(text), &containers); err != nil {
		return nil, fmt.Errorf("invalid Docker inventory output: %w", err)
	}
	services := []Service{}
	for _, container := range containers {
		labels := container.Config.Labels
		if strings.EqualFold(labels["com.docker.compose.oneoff"], "true") {
			continue
		}
		if container.ID == "" {
			return nil, errors.New("container has no ID")
		}
		id := labels["com.supabase.stack"]
		if id == "" {
			id = labels["com.supabase.cli.project"]
		}
		if id == "" {
			id = labels["com.supabase.stack.id"]
		}
		ports := []uint16{}
		for _, bindings := range container.HostConfig.PortBindings {
			for _, binding := range bindings {
				port, err := strconv.ParseUint(binding.HostPort, 10, 16)
				if err == nil && port > 0 && !slices.Contains(ports, uint16(port)) {
					ports = append(ports, uint16(port))
				}
			}
		}
		slices.Sort(ports)
		service := Service{ID: container.ID, Name: strings.TrimPrefix(container.Name, "/"), ProjectID: id, State: container.State.Status, Ports: ports}
		if path := labels["com.supabase.cli.workdir"]; path != "" {
			service.Workdir = &path
		}
		if container.State.Health != nil {
			service.Health = &container.State.Health.Status
		}
		services = append(services, service)
	}
	return services, nil
}
func selected(inventory Inventory, project Project) []Service {
	id := project.ProjectID
	if project.Adapter.Kind == "stack" {
		id = project.Adapter.StackID
	}
	services := []Service{}
	for _, service := range inventory.Services {
		if service.ProjectID == id {
			services = append(services, service)
		}
	}
	return services
}
func supabase(settings Settings, endpoint string, project Project, operation string) CommandSpec {
	spec := command(settings.SupabaseCLI)
	spec.Dir = project.Path
	spec.Env["DOCKER_HOST"] = endpoint
	spec.Env["SUPABASE_EXPERIMENTAL_STACK"] = "0"
	if project.Adapter.Kind == "stack" {
		spec.Env["SUPABASE_EXPERIMENTAL_STACK"] = "1"
		spec.Args = []string{"stack", operation, "--stack-id", project.Adapter.StackID}
		if operation == "start" {
			spec.Args = append(spec.Args, "--runtime", "docker")
		}
		if operation == "status" {
			spec.Args = append(spec.Args, "--output-format", "json")
		}
	} else {
		spec.Args = []string{operation}
		if operation == "stop" {
			spec.Args = append(spec.Args, "--project-id", project.ProjectID)
		}
		if operation == "status" {
			spec.Args = append(spec.Args, "--output", "json")
		}
	}
	spec.Args = append(spec.Args, "--workdir", project.Path)
	if operation == "start" {
		spec.Timeout = 600 * time.Second
	}
	return spec
}
func (e *Engine) connections(ctx context.Context, settings Settings, endpoint string, project Project) (map[string]string, error) {
	text, err := require(e.Runner.Run(ctx, supabase(settings, endpoint, project, "status")))
	if err != nil {
		return nil, err
	}
	values := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(text), &values); err != nil {
		return nil, fmt.Errorf("Supabase status did not return valid JSON: %w", err)
	}
	if env, exists := values["env"]; exists {
		if err := json.Unmarshal(env, &values); err != nil {
			return nil, fmt.Errorf("status JSON has no environment map: %w", err)
		}
	}
	connections := map[string]string{}
	for key, raw := range values {
		var value string
		if json.Unmarshal(raw, &value) == nil {
			connections[key] = value
		}
	}
	return connections, nil
}
func (e *Engine) verifyStack(ctx context.Context, settings Settings, project Project) error {
	if project.Adapter.Kind != "stack" {
		return nil
	}
	spec := command(settings.SupabaseCLI, "stack", "--help")
	spec.Dir = project.Path
	spec.Env["SUPABASE_EXPERIMENTAL_STACK"] = "1"
	help, err := require(e.Runner.Run(ctx, spec))
	if err != nil {
		return err
	}
	if !strings.Contains(help, "list") || !strings.Contains(help, "status") {
		return errors.New("this CLI has no compatible experimental stack commands")
	}
	spec.Args = []string{"stack", "list", "--output-format", "json"}
	text, err := require(e.Runner.Run(ctx, spec))
	if err != nil {
		return err
	}
	type stack struct {
		ID          string
		ProjectRoot string `json:"project_root"`
		RootAlias   string `json:"projectRoot"`
		Runtime     string
	}
	stacks := []stack{}
	if err := json.Unmarshal([]byte(text), &stacks); err != nil {
		var response struct{ Stacks []stack }
		if err := json.Unmarshal([]byte(text), &response); err != nil {
			return fmt.Errorf("unsupported experimental stack list format: %w", err)
		}
		stacks = response.Stacks
	}
	for _, stack := range stacks {
		if stack.ID != project.Adapter.StackID {
			continue
		}
		root := stack.ProjectRoot
		if root == "" {
			root = stack.RootAlias
		}
		path, err := canonical(root)
		if root == "" || err != nil || path != project.Path {
			return errors.New("stack ID belongs to a different or unverifiable project folder")
		}
		if stack.Runtime != "docker" {
			return errors.New("only Docker experimental stacks are supported in v1")
		}
		return nil
	}
	return errors.New("stack ID not found; create it with Supabase CLI first")
}

var memoryPattern = regexp.MustCompile(`^([0-9.]+)\s*([A-Za-z]+)$`)

func ParseMemory(value string) *uint64 {
	used, _, _ := strings.Cut(value, "/")
	parts := memoryPattern.FindStringSubmatch(strings.TrimSpace(used))
	if parts == nil {
		return nil
	}
	amount, err := strconv.ParseFloat(parts[1], 64)
	factors := map[string]float64{"B": 1, "kB": 1e3, "KB": 1e3, "MB": 1e6, "GB": 1e9, "TB": 1e12, "KiB": 1024, "MiB": 1048576, "GiB": 1073741824, "TiB": 1099511627776}
	factor, exists := factors[parts[2]]
	bytes := amount * factor
	if err != nil || !exists || math.IsInf(bytes, 0) || math.IsNaN(bytes) || bytes < 0 || bytes >= math.MaxUint64 {
		return nil
	}
	return pointer(uint64(bytes))
}
func ParseStats(text string, ids []string) ([]ResourceReading, error) {
	readings := map[string]ResourceReading{}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var sample struct {
			ID       string
			CPUPerc  string
			MemUsage string
		}
		if err := json.Unmarshal([]byte(line), &sample); err != nil {
			return nil, fmt.Errorf("invalid Docker stats output: %w", err)
		}
		if !slices.Contains(ids, sample.ID) {
			continue
		}
		reading := ResourceReading{ContainerID: sample.ID, MemoryBytes: ParseMemory(sample.MemUsage)}
		cpu, err := strconv.ParseFloat(strings.TrimSuffix(sample.CPUPerc, "%"), 64)
		if err == nil && cpu >= 0 && !math.IsInf(cpu, 0) && !math.IsNaN(cpu) {
			reading.CPUPercent = &cpu
		}
		readings[sample.ID] = reading
	}
	result := make([]ResourceReading, 0, len(ids))
	for _, id := range ids {
		reading, exists := readings[id]
		if !exists {
			reading = ResourceReading{ContainerID: id}
		}
		result = append(result, reading)
	}
	return result, nil
}
func (e *Engine) resources(ctx context.Context, settings Settings, endpoint string, services []Service) ([]ResourceReading, error) {
	ids := []string{}
	for _, service := range services {
		if service.State == "running" {
			ids = append(ids, service.ID)
		}
	}
	if len(ids) == 0 {
		return []ResourceReading{}, nil
	}
	spec := docker(settings, endpoint, "stats", "--no-stream", "--no-trunc", "--format", "{{json .}}")
	spec.Args = append(spec.Args, ids...)
	text, err := require(e.Runner.Run(ctx, spec))
	if err != nil {
		return nil, err
	}
	return ParseStats(text, ids)
}
