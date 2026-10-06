package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (e *Engine) PreviewIdentity(ctx context.Context, id, newID string) (IdentityPreview, error) {
	registry, err := e.Store.Read()
	if err != nil {
		return IdentityPreview{}, err
	}
	project, err := find(registry, id)
	if err != nil {
		return IdentityPreview{}, err
	}
	if project.Adapter.Kind != "standard" {
		return IdentityPreview{}, errors.New("stack identities are immutable; create a separate stack with Supabase CLI")
	}
	version, err := e.version(ctx, registry.Settings)
	if err != nil {
		return IdentityPreview{}, err
	}
	if err := Supported(version); err != nil {
		return IdentityPreview{}, err
	}
	config, err := readConfig(project)
	if err != nil {
		return IdentityPreview{}, err
	}
	if config.ProjectID != project.ProjectID || config.Experimental {
		return IdentityPreview{}, errors.New("project identity or adapter changed outside Toys; re-register explicitly")
	}
	if !identityPattern.MatchString(newID) || newID == project.ProjectID {
		return IdentityPreview{}, errors.New("choose a different project ID using only letters, digits, underscores, and hyphens")
	}
	for _, other := range registry.Projects {
		if other.ID == project.ID {
			continue
		}
		config, err := readConfig(other)
		if err != nil {
			return IdentityPreview{}, err
		}
		if config.ProjectID == newID {
			return IdentityPreview{}, errors.New("the proposed project ID is already registered")
		}
	}
	inventory := e.inventory(ctx, registry.Settings)
	if !inventory.Available {
		return IdentityPreview{}, errors.New("Docker must be available to prove this project is unused")
	}
	if err := e.unusedIdentity(ctx, registry.Settings, project, inventory); err != nil {
		return IdentityPreview{}, err
	}
	proposed := project
	proposed.ProjectID = newID
	if err := e.unusedIdentity(ctx, registry.Settings, proposed, inventory); err != nil {
		return IdentityPreview{}, err
	}
	return IdentityPreview{Project: project.ID, ConfigHash: Hash([]byte(config.Text)), Before: project.ProjectID, After: newID}, nil
}
func (e *Engine) unusedIdentity(ctx context.Context, settings Settings, project Project, inventory Inventory) error {
	if !inventory.Available || len(selected(inventory, project)) > 0 {
		return errors.New("existing containers may own this ID; identity repair is refused because changing IDs does not migrate data")
	}
	for _, kind := range []string{"volume", "network"} {
		text, err := require(e.Runner.Run(ctx, docker(settings, *inventory.Endpoint, kind, "ls", "--format", "{{.Name}}")))
		if err != nil {
			return err
		}
		for _, name := range strings.Split(text, "\n") {
			if strings.HasPrefix(name, "supabase_") && strings.HasSuffix(name, "_"+project.ProjectID) {
				return fmt.Errorf("existing %s resources may contain data for this ID; identity repair is refused", kind)
			}
		}
	}
	return nil
}
func (e *Engine) ApplyIdentity(ctx context.Context, preview IdentityPreview) (Project, error) {
	var project Project
	err := e.mutate(func() error {
		verified, err := e.PreviewIdentity(ctx, preview.Project, preview.After)
		if err != nil {
			return err
		}
		if verified != preview {
			return errors.New("identity preview is stale; review a fresh preview")
		}
		registry, err := e.Store.Read()
		if err != nil {
			return err
		}
		project, err = find(registry, preview.Project)
		if err != nil {
			return err
		}
		config, err := readConfig(project)
		if err != nil {
			return err
		}
		next, err := Patch(config, map[string]any{"project_id": preview.After})
		if err != nil {
			return err
		}
		path := filepath.Join(project.Path, "supabase", "config.toml")
		if err := backup(path, []byte(config.Text)); err != nil {
			return err
		}
		current, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("recheck config during identity repair: %w", err)
		}
		if Hash(current) != preview.ConfigHash {
			return errors.New("configuration changed during identity repair")
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("identity repair cancelled: %w", err)
		}
		if err := AtomicWrite(path, []byte(next)); err != nil {
			return err
		}
		project.ProjectID = preview.After
		for index, registered := range registry.Projects {
			if registered.ID == project.ID {
				registry.Projects[index] = project
			}
		}
		return e.Store.Write(registry)
	})
	return project, err
}
