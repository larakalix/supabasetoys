package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRunner struct {
	mu          sync.Mutex
	calls       []CommandSpec
	states      map[string]bool
	version     string
	unavailable bool
	failStart   bool
	volumes     string
}

func (f *fakeRunner) Run(ctx context.Context, spec CommandSpec) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, spec)
	success := func(text string) (Output, error) { return Output{Success: true, Stdout: text}, nil }
	encode := func(value any) (Output, error) {
		data, err := json.Marshal(value)
		return Output{Success: err == nil, Stdout: string(data)}, err
	}
	if slices.Equal(spec.Args, []string{"--version"}) {
		version := f.version
		if version == "" {
			version = "2.119.0"
		}
		return success(version)
	}
	if slices.Contains(spec.Args, "context") {
		return encode([]any{map[string]any{"Endpoints": map[string]any{"docker": map[string]string{"Host": "unix:///tmp/toys-test.sock"}}}})
	}
	if spec.Executable == "docker" && f.unavailable {
		return Output{}, errors.New("Docker unavailable")
	}
	if slices.Contains(spec.Args, "ps") {
		ids := []string{}
		for id := range f.states {
			ids = append(ids, "id-"+id)
		}
		return success(strings.Join(ids, "\n"))
	}
	if slices.Contains(spec.Args, "volume") {
		return success(f.volumes)
	}
	if slices.Contains(spec.Args, "network") {
		return success("")
	}
	if slices.Contains(spec.Args, "inspect") {
		containers := []any{}
		for id, running := range f.states {
			state := "exited"
			if running {
				state = "running"
			}
			containers = append(containers, map[string]any{"Id": "id-" + id, "Name": "/supabase_db_" + id, "Config": map[string]any{"Labels": map[string]string{"com.supabase.cli.project": id}}, "State": map[string]any{"Status": state, "Health": map[string]string{"Status": "healthy"}}, "HostConfig": map[string]any{"PortBindings": map[string]any{}}})
		}
		return encode(containers)
	}
	if slices.Contains(spec.Args, "stats") {
		return success(`{"ID":"id-A","CPUPerc":"105.4%","MemUsage":"2MiB / 8GiB"}`)
	}
	if slices.Contains(spec.Args, "logs") {
		return success("2026-10-05 request token=secret-value\nAPI ready\n")
	}
	if len(spec.Args) > 0 && spec.Args[0] == "status" {
		return encode(map[string]string{"API_URL": "http://127.0.0.1:54321", "STUDIO_URL": "http://127.0.0.1:54323", "ANON_KEY": "eyJfake.fake.signature", "SERVICE_ROLE_KEY": "sb_secret_secretvalue", "DB_URL": "postgresql://postgres:secret@127.0.0.1:54322/postgres"})
	}
	if len(spec.Args) > 0 && spec.Args[0] == "stop" {
		index := slices.Index(spec.Args, "--project-id")
		if index < 0 {
			return Output{}, errors.New("untargeted stop")
		}
		f.states[spec.Args[index+1]] = false
		return success("")
	}
	if len(spec.Args) > 0 && spec.Args[0] == "start" {
		config, err := readConfig(Project{Path: spec.Dir})
		if err != nil {
			return Output{}, err
		}
		f.states[config.ProjectID] = true
		if f.failStart {
			return Output{Success: false, Stderr: "password=super-secret\npartial start failed"}, nil
		}
		return success("")
	}
	return Output{}, fmt.Errorf("unexpected command %v", spec.Args)
}
func setup(t *testing.T) (*Engine, *fakeRunner) {
	t.Helper()
	fake := &fakeRunner{states: map[string]bool{}, calls: []CommandSpec{}}
	core, err := WithRunner(filepath.Join(t.TempDir(), "data"), fake)
	if err != nil {
		t.Fatal(err)
	}
	return core, fake
}
func freePair(t *testing.T) uint16 {
	t.Helper()
	for port := uint16(35000); port < 60000; port += 2 {
		if PortAvailable(port) && PortAvailable(port+1) {
			return port
		}
	}
	t.Fatal("no available test ports")
	return 0
}
func projectFolder(t *testing.T, id string, port uint16) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "supabase"), 0o700); err != nil {
		t.Fatal(err)
	}
	text := fmt.Sprintf("# preserve this\nproject_id = %q # identity comment\n[db]\nport = %d # port comment\nshadow_port = %d\n[api]\nenabled=false\n[studio]\nenabled=false\n[local_smtp]\nenabled=false\n[analytics]\nenabled=false\n[edge_runtime]\nenabled=false\n", id, port, port+1)
	if err := os.WriteFile(filepath.Join(root, "supabase", "config.toml"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}
func add(t *testing.T, e *Engine, id string, port uint16) Project {
	t.Helper()
	p, err := e.Add(t.Context(), projectFolder(t, id, port), id, "")
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestPreservesWorkingPortsAndComments(t *testing.T) {
	config, err := ParseConfig("# comment\nproject_id='A'\n[db]\nport=55000 # keep\nshadow_port=55001\n")
	if err != nil {
		t.Fatal(err)
	}
	changes, err := Allocate(config, map[uint16]bool{55000: true}, func(uint16) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	updates := map[string]any{}
	for _, change := range changes {
		updates[change.Key] = int64(change.After)
	}
	next, err := Patch(config, updates)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(next, "# keep") || !strings.Contains(next, "shadow_port=55001") {
		t.Fatal(next)
	}
}
func TestOptionalPortCatalog(t *testing.T) {
	config, err := ParseConfig("project_id='A'\n[db.pooler]\nenabled=true\nport=54329\n[local_smtp]\nenabled=true\nport=54324\nsmtp_port=54325\npop3_port=54326\n[analytics]\nenabled=true\nport=54327\nvector_port=54328\n[edge_runtime]\nenabled=true\ninspector_port=8083\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"db.shadow_port", "db.pooler.port", "local_smtp.smtp_port", "local_smtp.pop3_port", "analytics.vector_port", "edge_runtime.inspector_port"} {
		if _, exists := config.Ports[key]; !exists {
			t.Fatal(key)
		}
	}
	if _, exists := config.Ports["inbucket.port"]; exists {
		t.Fatal("mail aliases allocated twice")
	}
}
func TestInvalidConfiguration(t *testing.T) {
	for _, text := range []string{"project_id='bad/id'", "project_id='A'\n[db]\nport=0", "project_id='A'\n[db]\nport='env(PORT)'", "project_id='A'\n[db]\nport=65536"} {
		if _, err := ParseConfig(text); err == nil {
			t.Fatal(text)
		}
	}
}
func TestInternalDuplicatesAndWorkingReservations(t *testing.T) {
	config, err := ParseConfig("project_id='A'\n[api]\nport=20000\n[db]\nport=55000\nshadow_port=55000")
	if err != nil {
		t.Fatal(err)
	}
	changes, err := Allocate(config, map[uint16]bool{}, func(uint16) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].After == 20000 {
		t.Fatal(changes)
	}
}
func TestOccupiedListenerAndExhaustion(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if PortAvailable(uint16(listener.Addr().(*net.TCPAddr).Port)) {
		t.Fatal("occupied listener was considered free")
	}
	config, err := ParseConfig("project_id='A'")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Allocate(config, map[uint16]bool{}, func(uint16) bool { return false }); err == nil {
		t.Fatal("exhausted range accepted")
	}
}
func TestStoppedReservationsAndPersistence(t *testing.T) {
	core, _ := setup(t)
	port := freePair(t)
	add(t, core, "A", port)
	b := add(t, core, "B", port)
	if _, err := core.Lifecycle(t.Context(), b.ID, "start", nil); err == nil {
		t.Fatal("unrepaired conflict started")
	}
	preview, err := core.PreviewRepair(t.Context(), b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Changes) != 2 {
		t.Fatal(preview)
	}
	if _, err := core.ApplyRepair(t.Context(), preview, false); err != nil {
		t.Fatal(err)
	}
	reopened, err := WithRunner(core.Store.Root, core.Runner)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := reopened.PreviewRepair(t.Context(), b.ID)
	if err != nil || len(fresh.Changes) != 0 {
		t.Fatalf("%v %v", fresh, err)
	}
	files, err := filepath.Glob(filepath.Join(b.Path, "supabase", "*.bak"))
	if err != nil || len(files) == 0 {
		t.Fatal("backup absent")
	}
}
func TestStaleAndTamperedRepair(t *testing.T) {
	core, _ := setup(t)
	port := freePair(t)
	add(t, core, "A", port)
	b := add(t, core, "B", port)
	preview, err := core.PreviewRepair(t.Context(), b.ID)
	if err != nil {
		t.Fatal(err)
	}
	tampered := preview
	tampered.Changes = slices.Clone(preview.Changes)
	tampered.Changes[0].After++
	if _, err := core.ApplyRepair(t.Context(), tampered, false); err == nil {
		t.Fatal("tampered preview applied")
	}
	file := filepath.Join(b.Path, "supabase", "config.toml")
	original, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, append(original, []byte("\n# outside edit")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := core.ApplyRepair(t.Context(), preview, false); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatal(err)
	}
}
func TestDuplicateIdentityBlocksEveryLifecycle(t *testing.T) {
	core, fake := setup(t)
	port := freePair(t)
	a := add(t, core, "shared", port)
	add(t, core, "shared", port)
	for _, operation := range []string{"start", "stop", "restart"} {
		if _, err := core.Lifecycle(t.Context(), a.ID, operation, nil); err == nil || !strings.Contains(err.Error(), "duplicate") {
			t.Fatal(operation, err)
		}
	}
	for _, call := range fake.calls {
		if len(call.Args) > 0 && (call.Args[0] == "start" || call.Args[0] == "stop") {
			t.Fatal("ambiguous action executed")
		}
	}
}
func TestOutsideIdentityEditBlocksManagement(t *testing.T) {
	core, _ := setup(t)
	p := add(t, core, "A", freePair(t))
	file := filepath.Join(p.Path, "supabase", "config.toml")
	original, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(strings.Replace(string(original), `"A"`, `"B"`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Lifecycle(t.Context(), p.ID, "stop", nil); err == nil {
		t.Fatal("changed identity accepted")
	}
}
func TestTargetedLifecyclePinsContextAndPreservesA(t *testing.T) {
	core, fake := setup(t)
	port := freePair(t)
	a := add(t, core, "A", port)
	b := add(t, core, "B", port+10)
	for _, project := range []Project{a, b} {
		if _, err := core.Lifecycle(t.Context(), project.ID, "start", nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := core.Lifecycle(t.Context(), b.ID, "restart", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Lifecycle(t.Context(), b.ID, "stop", nil); err != nil {
		t.Fatal(err)
	}
	if !fake.states["A"] {
		t.Fatal("B management stopped A")
	}
	registry, err := core.Registry()
	if err != nil || registry.Settings.DockerEndpoint == nil {
		t.Fatal("endpoint not persisted")
	}
	for _, call := range fake.calls {
		if len(call.Args) > 0 && call.Args[0] == "stop" {
			index := slices.Index(call.Args, "--project-id")
			if index < 0 || call.Args[index+1] != "B" || slices.Contains(call.Args, "--all") || slices.Contains(call.Args, "--no-backup") {
				t.Fatal(call.Args)
			}
			if call.Env["DOCKER_HOST"] != "unix:///tmp/toys-test.sock" {
				t.Fatal(call.Env)
			}
		}
	}
}
func TestPartialFailureReconcilesAndRedacts(t *testing.T) {
	core, fake := setup(t)
	p := add(t, core, "A", freePair(t))
	fake.failStart = true
	_, err := core.Lifecycle(t.Context(), p.ID, "start", nil)
	if err == nil || strings.Contains(err.Error(), "super-secret") || !strings.Contains(err.Error(), "1 running") {
		t.Fatal(err)
	}
}
func TestUnavailabilityAndUnsupportedVersion(t *testing.T) {
	core, fake := setup(t)
	p := add(t, core, "A", freePair(t))
	fake.unavailable = true
	status, err := core.Status(t.Context(), p.ID, true)
	if err != nil || status.State != "unavailable" {
		t.Fatal(status, err)
	}
	fake.unavailable = false
	fake.version = "99.0.0"
	status, err = core.Status(t.Context(), p.ID, false)
	if err != nil || !slices.ContainsFunc(status.Diagnostics, func(d Diagnostic) bool { return d.Code == "unsupported_cli" }) {
		t.Fatal(status, err)
	}
	if _, err := core.Lifecycle(t.Context(), p.ID, "start", nil); err == nil {
		t.Fatal("unsupported version mutated")
	}
}
func TestSharedLockPreventsConcurrentMutation(t *testing.T) {
	core, _ := setup(t)
	lock, err := core.Store.Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Unlock()
	other, err := NewStore(core.Store.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Lock(); err == nil {
		t.Fatal("concurrent lock succeeded")
	}
	if err := core.Configure(Settings{SupabaseCLI: "supabase", DockerCLI: "docker"}); err == nil {
		t.Fatal("locked mutation succeeded")
	}
}
func TestRemoteTargetsRefused(t *testing.T) {
	for _, endpoint := range []string{"tcp://localhost:2375", "ssh://host", "unix:///tmp/x\n"} {
		if ValidateEndpoint(endpoint) == nil {
			t.Fatal(endpoint)
		}
	}
	for _, endpoint := range []string{"https://example.com", "file:///etc/passwd", "http://secret:password@127.0.0.1"} {
		if _, err := LocalHTTPURL(endpoint); err == nil {
			t.Fatal(endpoint)
		}
	}
	if _, err := LocalHTTPURL("http://[::1]:54323"); err != nil {
		t.Fatal(err)
	}
}
func TestEnvironmentMergePreservesUnrelatedValues(t *testing.T) {
	merged, err := MergeEnv("# keep\r\nOTHER=unchanged\r\nexport SUPABASE_URL=old\r\n", map[string]string{"SUPABASE_URL": "http://local"})
	if err != nil || !strings.Contains(merged, "OTHER=unchanged\r\n") || !strings.Contains(merged, "export SUPABASE_URL=\"http://local\"") {
		t.Fatal(merged, err)
	}
	if _, err := MergeEnv("X=old\nX=duplicate", map[string]string{"X": "new"}); err == nil {
		t.Fatal("duplicate assignments accepted")
	}
}
func TestBrowserPrivilegedValuesBlocked(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"role":"service_role"}`))
	for _, value := range []string{"sb_secret_private", "sbp_private", "postgresql://user:password@localhost/db", "eyJfake." + payload + ".signature"} {
		if _, err := MappedValues(map[string]string{"ANON_KEY": value}, map[string]string{"ANON_KEY": "VITE_KEY"}); err == nil {
			t.Fatal(value)
		}
	}
	if _, err := MappedValues(map[string]string{"SERVICE_ROLE_KEY": "plain"}, map[string]string{"SERVICE_ROLE_KEY": "NUXT_PUBLIC_KEY"}); err == nil {
		t.Fatal("privileged source accepted")
	}
	if _, err := MappedValues(map[string]string{"ANON_KEY": "public"}, map[string]string{"ANON_KEY": "VITE_KEY"}); err != nil {
		t.Fatal(err)
	}
}
func TestEnvironmentStaleAndPaths(t *testing.T) {
	core, _ := setup(t)
	p := add(t, core, "A", freePair(t))
	mapping := map[string]string{"API_URL": "SUPABASE_URL"}
	preview, err := core.PreviewEnv(t.Context(), p.ID, ".env.local", mapping)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Path, ".env.local"), []byte("OTHER=external"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := core.ApplyEnv(t.Context(), preview, mapping); err == nil {
		t.Fatal("stale environment applied")
	}
	for _, path := range []string{"../outside", "/tmp/outside", "a/../outside", ""} {
		if _, err := core.PreviewEnv(t.Context(), p.ID, path, mapping); err == nil {
			t.Fatal(path)
		}
	}
}
func TestLogsCannotCrossProjects(t *testing.T) {
	core, fake := setup(t)
	p := add(t, core, "A", freePair(t))
	fake.states = map[string]bool{"A": true, "B": true}
	if _, err := core.Logs(t.Context(), p.ID, LogOptions{Service: "id-B", Tail: 10}); err == nil {
		t.Fatal("cross-project logs allowed")
	}
	logs, err := core.Logs(t.Context(), p.ID, LogOptions{Tail: 10})
	if err != nil || strings.Contains(logs, "secret-value") || !strings.Contains(logs, "API ready") {
		t.Fatal(logs, err)
	}
}
func TestResourceIDsAndUnknownValues(t *testing.T) {
	readings, err := ParseStats("{\"ID\":\"a\",\"CPUPerc\":\"105.4%\",\"MemUsage\":\"2MiB / 8GiB\"}\n{\"ID\":\"foreign\",\"CPUPerc\":\"1%\"}", []string{"a", "missing"})
	if err != nil || len(readings) != 2 || *readings[0].CPUPercent != 105.4 || *readings[0].MemoryBytes != 2097152 || readings[1].CPUPercent != nil {
		t.Fatal(readings, err)
	}
	for _, value := range []string{"unknown", "-1MB", "NaNGB"} {
		if ParseMemory(value) != nil {
			t.Fatal(value)
		}
	}
}
func TestEmptyInventoryNeverSamplesUnfilteredStats(t *testing.T) {
	core, fake := setup(t)
	p := add(t, core, "A", freePair(t))
	if _, err := core.Status(t.Context(), p.ID, true); err != nil {
		t.Fatal(err)
	}
	for _, call := range fake.calls {
		if slices.Contains(call.Args, "stats") {
			t.Fatal("unfiltered stats requested")
		}
	}
}
func TestDiagnosticReportsAreValidAndRedacted(t *testing.T) {
	core, fake := setup(t)
	p := add(t, core, "A", freePair(t))
	fake.states["A"] = true
	report, err := core.DiagnosticReport(t.Context(), p.ID, true)
	if err != nil || !json.Valid([]byte(report)) || strings.Contains(report, "secret-value") || strings.Contains(report, "sb_secret_") || strings.Contains(report, p.Path) {
		t.Fatal(report, err)
	}
}
func TestRemoveLeavesRuntimeAndConfigIntact(t *testing.T) {
	core, fake := setup(t)
	p := add(t, core, "A", freePair(t))
	fake.states["A"] = true
	if err := core.Remove(p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := readConfig(p); err != nil {
		t.Fatal(err)
	}
	if !fake.states["A"] {
		t.Fatal("removal stopped runtime")
	}
}
func TestUnusedIdentityPreviewAndDataGuard(t *testing.T) {
	core, fake := setup(t)
	p := add(t, core, "A", freePair(t))
	preview, err := core.PreviewIdentity(t.Context(), p.ID, "B")
	if err != nil {
		t.Fatal(err)
	}
	next, err := core.ApplyIdentity(t.Context(), preview)
	if err != nil || next.ProjectID != "B" {
		t.Fatal(next, err)
	}
	config, err := readConfig(next)
	if err != nil || !strings.Contains(config.Text, "# identity comment") {
		t.Fatal(config, err)
	}
	fake.volumes = "supabase_db_B"
	if _, err := core.PreviewIdentity(t.Context(), p.ID, "C"); err == nil {
		t.Fatal("existing volume ignored")
	}
}
func TestExistingStoppedContainersBlockIdentityRepair(t *testing.T) {
	core, fake := setup(t)
	p := add(t, core, "A", freePair(t))
	fake.states["A"] = false
	if _, err := core.PreviewIdentity(t.Context(), p.ID, "B"); err == nil {
		t.Fatal("stopped container ignored")
	}
}
func TestBothVersionsAndExplicitStackArguments(t *testing.T) {
	for _, version := range []string{"2.119.0", "2.118.0"} {
		if err := Supported(version); err != nil {
			t.Fatal(err)
		}
	}
	spec := supabase(Settings{SupabaseCLI: "supabase"}, "unix:///tmp/docker.sock", Project{Path: "/tmp/project", Adapter: Adapter{Kind: "stack", StackID: "abcdef123456"}}, "start")
	if !slices.Contains(spec.Args, "--stack-id") || !slices.Contains(spec.Args, "abcdef123456") || !slices.Contains(spec.Args, "docker") || spec.Env["SUPABASE_EXPERIMENTAL_STACK"] != "1" {
		t.Fatal(spec)
	}
}
func TestUpstreamContainerCompatibilityFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/docker-inventory.json")
	if err != nil {
		t.Fatal(err)
	}
	services, err := ParseInventory(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(services, func(s Service) bool { return s.ProjectID != "" }) {
		t.Fatal(services)
	}
	if !slices.ContainsFunc(services, func(s Service) bool { return s.ProjectID == "" && len(s.Ports) > 0 }) {
		t.Fatal("foreign port reservation absent")
	}
}
func TestExistingRegistryAndRegistrationIDRemainCompatible(t *testing.T) {
	core, _ := setup(t)
	path := projectFolder(t, "A", freePair(t))
	canonicalPath, err := canonical(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := core.Add(t.Context(), path, "A", "")
	if err != nil {
		t.Fatal(err)
	}
	expected := Hash([]byte(canonicalPath + ":Standard"))[:16]
	if p.ID != expected {
		t.Fatal(p.ID, expected)
	}
	again, err := core.Add(t.Context(), path, "Renamed", "")
	if err != nil || again.ID != p.ID {
		t.Fatal(again, err)
	}
}
func TestTOMLMultilineAndInlineSettingsAreNeverCorrupted(t *testing.T) {
	config, err := ParseConfig("project_id='A'\nnotes='''\n[db]\nport = 55000\n'''\n[db]\nport=55000\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Patch(config, map[string]any{"db.port": int64(20000)}); err == nil {
		t.Fatal("ambiguous text edit accepted")
	}
	inline, err := ParseConfig("project_id='A'\ndb={ port=55000, shadow_port=55001 }\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Patch(inline, map[string]any{"db.port": int64(20000)}); err == nil {
		t.Fatal("inline table corrupted")
	}
}
func TestCancelledProcessIsBounded(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	started := time.Now()
	if _, err := (SystemRunner{}).Run(ctx, command(executable)); err == nil {
		t.Fatal("cancelled command ran")
	}
	if time.Since(started) > time.Second {
		t.Fatal("cancellation was not bounded")
	}
}
func TestJSONDispatchMatchesSharedTypes(t *testing.T) {
	core, _ := setup(t)
	p := add(t, core, "A", freePair(t))
	value, err := core.Handle(t.Context(), Request{Action: "status", Project: p.ID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var state Status
	if err := json.Unmarshal(data, &state); err != nil || state.Project.ID != p.ID || state.Services == nil || state.Resources == nil {
		t.Fatal(state, err)
	}
}
