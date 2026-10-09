package engine

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRegistrationMutationsPreserveOtherProjects(t *testing.T) {
	for _, operation := range []string{"remove", "unassociate"} {
		for _, selection := range []string{"id", "name", "missing", "ambiguous", "locked", "corrupt"} {
			t.Run(operation+"/"+selection, func(t *testing.T) {
				t.Parallel()
				core, runner := setup(t)
				selected := registerLocal(t, core, "selected")
				other := registerLocal(t, core, "other")
				runner.states[selected.ProjectID] = true
				runner.states[other.ProjectID] = true
				registry, err := core.Store.Read()
				if err != nil {
					t.Fatal(err)
				}
				registry.Associations = []LocalProjectAssociation{
					{LocalProjectID: selected.ID, CloudRef: refA},
					{LocalProjectID: other.ID, CloudRef: refB},
				}
				id := selected.ID
				expectedError := ""
				switch selection {
				case "name":
					id = selected.Name
				case "missing":
					id = "unknown"
					expectedError = "project not registered"
				case "ambiguous":
					registry.Projects[1].Name = selected.Name
					id = selected.Name
					expectedError = "more than one project"
				case "locked":
					lock, err := core.Store.Lock()
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() {
						if err := lock.Unlock(); err != nil {
							t.Error(err)
						}
					})
					expectedError = "another Supabase Toys operation is active"
				case "corrupt":
					expectedError = "invalid registry"
				}
				if err := core.Store.Write(registry); err != nil {
					t.Fatal(err)
				}
				registryPath := filepath.Join(core.Store.Root, "registry.json")
				if selection == "corrupt" {
					if err := os.WriteFile(registryPath, []byte("{broken"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				before, err := os.ReadFile(registryPath)
				if err != nil {
					t.Fatal(err)
				}
				mutate := core.Remove
				if operation == "unassociate" {
					mutate = core.Unassociate
				}
				err = mutate(id)
				if expectedError != "" {
					if err == nil || !strings.Contains(err.Error(), expectedError) {
						t.Fatalf("expected %q, got %v", expectedError, err)
					}
					after, readErr := os.ReadFile(registryPath)
					if readErr != nil || !bytes.Equal(before, after) {
						t.Fatalf("rejected operation changed registry: %v", readErr)
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					after, err := core.Store.Read()
					if err != nil {
						t.Fatal(err)
					}
					if !slices.Equal(after.Associations, registry.Associations[1:]) {
						t.Fatal("unrelated association changed or selected association retained")
					}
					expectedProjects := registry.Projects
					if operation == "remove" {
						expectedProjects = expectedProjects[1:]
					}
					if !slices.Equal(after.Projects, expectedProjects) || after.Settings != registry.Settings {
						t.Fatal("unexpected registration or settings change")
					}
				}
				if len(runner.calls) != 0 || !runner.states[selected.ProjectID] || !runner.states[other.ProjectID] {
					t.Fatal("registry operation touched runtime")
				}
				for _, project := range []Project{selected, other} {
					config, err := readConfig(project)
					if err != nil || config.ProjectID != project.ProjectID || config.Ports["api.port"] != 55001 {
						t.Fatalf("registry operation changed project config: %v", err)
					}
				}
			})
		}
	}
}
