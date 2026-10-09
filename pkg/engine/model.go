// Package engine manages local Supabase projects without depending on a UI framework.
package engine

type Adapter struct {
	Kind    string `json:"kind"`
	StackID string `json:"stack_id,omitempty"`
}
type Project struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Path      string  `json:"path"`
	ProjectID string  `json:"project_id"`
	Adapter   Adapter `json:"adapter"`
}
type Settings struct {
	SupabaseCLI    string  `json:"supabase_cli"`
	DockerCLI      string  `json:"docker_cli"`
	DockerEndpoint *string `json:"docker_endpoint"`
}
type Registry struct {
	Projects     []Project                 `json:"projects"`
	Accounts     []AccountProfile          `json:"accounts"`
	Associations []LocalProjectAssociation `json:"associations"`
	Settings     Settings                  `json:"settings"`
}
type Service struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	ProjectID string   `json:"project_id"`
	Workdir   *string  `json:"workdir"`
	State     string   `json:"state"`
	Health    *string  `json:"health"`
	Ports     []uint16 `json:"ports"`
}
type Inventory struct {
	Endpoint  *string   `json:"endpoint"`
	Available bool      `json:"available"`
	Services  []Service `json:"services"`
	Error     *string   `json:"error"`
}
type Diagnostic struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Remedy     string `json:"remedy"`
	Repairable bool   `json:"repairable"`
}
type ResourceReading struct {
	ContainerID string   `json:"container_id"`
	CPUPercent  *float64 `json:"cpu_percent"`
	MemoryBytes *uint64  `json:"memory_bytes"`
}
type Status struct {
	Project         Project           `json:"project"`
	State           string            `json:"state"`
	Services        []Service         `json:"services"`
	Diagnostics     []Diagnostic      `json:"diagnostics"`
	ConfiguredPorts map[string]uint16 `json:"configured_ports"`
	Endpoints       map[string]string `json:"endpoints"`
	Resources       []ResourceReading `json:"resources"`
	ResourcesError  *string           `json:"resources_error"`
	SupabaseVersion *string           `json:"supabase_version"`
}
type PortChange struct {
	Key    string `json:"key"`
	Before uint16 `json:"before"`
	After  uint16 `json:"after"`
}
type RepairPreview struct {
	Mode            string       `json:"mode,omitempty"`
	Project         string       `json:"project"`
	ConfigHash      string       `json:"config_hash"`
	Changes         []PortChange `json:"changes"`
	RestartRequired bool         `json:"restart_required"`
}
type EnvPreview struct {
	Project    string            `json:"project"`
	File       string            `json:"file"`
	SourceHash string            `json:"source_hash"`
	Updates    map[string]string `json:"updates"`
}
type IdentityPreview struct {
	Project    string `json:"project"`
	ConfigHash string `json:"config_hash"`
	Before     string `json:"before"`
	After      string `json:"after"`
}
type OperationProgress struct {
	Project string `json:"project"`
	Phase   string `json:"phase"`
	Message string `json:"message"`
}

func pointer[T any](value T) *T { return &value }
