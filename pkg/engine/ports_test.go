package engine

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestPlanPortChanges(t *testing.T) {
	config := Config{Ports: map[string]uint16{"api.port": 54000, "db.port": 54001, "db.shadow_port": 54002}}
	tests := []struct {
		name        string
		updates     map[string]uint16
		reserved    map[uint16]bool
		unavailable uint16
		wantError   string
	}{
		{name: "changes one port", updates: map[string]uint16{"api.port": 55000}},
		{name: "swap ports", updates: map[string]uint16{"api.port": 54001, "db.port": 54000}},
		{name: "unchanged", updates: map[string]uint16{"api.port": 54000}},
		{name: "disabled or unknown", updates: map[string]uint16{"studio.port": 55000}, wantError: "not an enabled"},
		{name: "zero", updates: map[string]uint16{"api.port": 0}, wantError: "between 1 and 65535"},
		{name: "duplicate service port", updates: map[string]uint16{"api.port": 54001}, wantError: "both request"},
		{name: "stopped reservation", updates: map[string]uint16{"api.port": 55000}, reserved: map[uint16]bool{55000: true}, wantError: "reserved"},
		{name: "host listener", updates: map[string]uint16{"api.port": 55000}, unavailable: 55000, wantError: "occupied"},
		{name: "unresolved other conflict", updates: map[string]uint16{"api.port": 55000}, reserved: map[uint16]bool{54002: true}, wantError: "reserved"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes, err := PlanPortChanges(config, tt.updates, tt.reserved, func(port uint16) bool { return port != tt.unavailable })
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("got %v, want %s", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, change := range changes {
				if change.Before != config.Ports[change.Key] || change.After != tt.updates[change.Key] {
					t.Fatal(changes)
				}
			}
			if config.Ports["api.port"] != 54000 {
				t.Fatal("planner mutated source")
			}
		})
	}
}
func TestPreviewPortsAndTargetedApply(t *testing.T) {
	core, fake := setup(t)
	port := freePair(t)
	a := add(t, core, "A", port)
	b := add(t, core, "B", port+2)
	fake.states["A"] = true
	fake.states["B"] = true
	preview, err := core.PreviewPorts(t.Context(), b.ID, map[string]uint16{"db.port": port + 4})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.RestartRequired || preview.Mode != "manual" || len(preview.Changes) != 1 {
		t.Fatal(preview)
	}
	if _, err := core.ApplyRepair(t.Context(), preview, false); err == nil {
		t.Fatal("restart approval required")
	}
	before, _ := os.ReadFile(filepath.Join(a.Path, "supabase/config.toml"))
	if _, err := core.ApplyRepair(t.Context(), preview, true); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(a.Path, "supabase/config.toml"))
	if string(before) != string(after) || !fake.states["A"] || !fake.states["B"] {
		t.Fatal("other project changed or target not restarted")
	}
	config, err := readConfig(b)
	if err != nil {
		t.Fatal(err)
	}
	if config.Ports["db.port"] != port+4 || config.Ports["db.shadow_port"] != port+3 || !strings.Contains(config.Text, "# port comment") {
		t.Fatal(config)
	}
	backups, _ := filepath.Glob(filepath.Join(b.Path, "supabase/config.toml.toys-*.bak"))
	if len(backups) == 0 {
		t.Fatal("missing backup")
	}
	status, err := core.Status(t.Context(), b.ID, false)
	if err != nil || status.ConfiguredPorts["db.port"] != port+4 {
		t.Fatalf("%+v %v", status, err)
	}
}
func TestManualPortsRejectStaleConflictingAndTamperedPreviews(t *testing.T) {
	for _, scenario := range []string{"stale", "listener", "reservation", "duplicate", "unknown mode"} {
		t.Run(scenario, func(t *testing.T) {
			core, _ := setup(t)
			port := freePair(t)
			project := add(t, core, "B", port)
			preview, err := core.PreviewPorts(t.Context(), project.ID, map[string]uint16{"db.port": port + 2})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(project.Path, "supabase/config.toml")
			original, _ := os.ReadFile(path)
			switch scenario {
			case "stale":
				if err := os.WriteFile(path, append(original, []byte("\n# external edit\n")...), 0600); err != nil {
					t.Fatal(err)
				}
			case "listener":
				listener, err := net.Listen("tcp4", ":"+strconv.Itoa(int(port+2)))
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
			case "reservation":
				add(t, core, "A", port+2)
			case "duplicate":
				preview.Changes = append(preview.Changes, preview.Changes[0])
			case "unknown mode":
				preview.Mode = "unsafe"
			}
			expected, _ := os.ReadFile(path)
			if _, err := core.ApplyRepair(t.Context(), preview, false); err == nil {
				t.Fatal("unsafe preview accepted")
			}
			actual, _ := os.ReadFile(path)
			if string(actual) != string(expected) {
				t.Fatal("rejected preview changed file")
			}
		})
	}
}
func TestPortRequestRejectsInvalidNumbers(t *testing.T) {
	for _, value := range []string{"0", "65536", "-1", "3.5", "\"54321\""} {
		t.Run(value, func(t *testing.T) {
			var request Request
			err := json.Unmarshal([]byte(`{"action":"ports_preview","ports":{"api.port":`+value+`}}`), &request)
			if value != "0" && err == nil {
				t.Fatal("invalid port decoded")
			}
		})
	}
}

type portEditRunner struct {
	Runner
	after func(CommandSpec)
}

func (r portEditRunner) Run(ctx context.Context, spec CommandSpec) (Output, error) {
	output, err := r.Runner.Run(ctx, spec)
	r.after(spec)
	return output, err
}
func TestManualPortApplyPreservesEditsDuringRuntimeRecheck(t *testing.T) {
	core, fake := setup(t)
	port := freePair(t)
	project := add(t, core, "B", port)
	preview, err := core.PreviewPorts(t.Context(), project.ID, map[string]uint16{"db.port": port + 2})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project.Path, "supabase/config.toml")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := string(original) + "\n# external edit during final inventory\n"
	inspections := 0
	core.Runner = portEditRunner{Runner: fake, after: func(spec CommandSpec) {
		if slices.Contains(spec.Args, "ps") {
			inspections++
			if inspections == 2 {
				if err := os.WriteFile(path, []byte(edited), 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}}
	if _, err := core.ApplyRepair(t.Context(), preview, false); err == nil || !strings.Contains(err.Error(), "configuration changed before write") {
		t.Fatalf("expected stale rejection, got %v", err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != edited {
		t.Fatal("external edit overwritten", err)
	}
}
