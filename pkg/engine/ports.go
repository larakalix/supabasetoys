package engine

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
)

// PlanPortChanges validates the complete resulting allocation without performing I/O.
// Availability is injected so the same policy can be tested independently of sockets.
func PlanPortChanges(config Config, updates map[string]uint16, reserved map[uint16]bool, available func(uint16) bool) ([]PortChange, error) {
	next := maps.Clone(config.Ports)
	changes := []PortChange{}
	for _, key := range slices.Sorted(maps.Keys(updates)) {
		before, ok := config.Ports[key]
		if !ok {
			return nil, fmt.Errorf("%s is not an enabled, supported port setting", key)
		}
		after := updates[key]
		if after == 0 {
			return nil, fmt.Errorf("%s must be between 1 and 65535", key)
		}
		next[key] = after
		if before != after {
			changes = append(changes, PortChange{Key: key, Before: before, After: after})
		}
	}
	seen := map[uint16]string{}
	for _, key := range slices.Sorted(maps.Keys(next)) {
		port := next[key]
		if other, ok := seen[port]; ok {
			return nil, fmt.Errorf("%s and %s both request port %d; choose separate ports", other, key, port)
		}
		seen[port] = key
		if reserved[port] || !available(port) {
			return nil, fmt.Errorf("%s port %d is occupied or reserved by another project; choose a free port", key, port)
		}
	}
	return changes, nil
}

func (e *Engine) PreviewPorts(ctx context.Context, id string, updates map[string]uint16) (RepairPreview, error) {
	registry, err := e.Store.Read()
	if err != nil {
		return RepairPreview{}, err
	}
	project, inventory, err := e.ready(ctx, registry, id)
	if err != nil {
		return RepairPreview{}, err
	}
	config, err := checkedConfig(registry, project)
	if err != nil {
		return RepairPreview{}, err
	}
	if project.Adapter.Kind != "standard" || config.Experimental {
		return RepairPreview{}, errors.New("experimental stacks manage their own ports; manual port changes are unavailable")
	}
	changes, err := e.manualChanges(registry, project, inventory, config, updates)
	if err != nil {
		return RepairPreview{}, err
	}
	return RepairPreview{Project: id, ConfigHash: Hash([]byte(config.Text)), Changes: changes, RestartRequired: projectRunning(inventory, project), Mode: "manual"}, nil
}

func projectRunning(inventory Inventory, project Project) bool {
	for _, service := range selected(inventory, project) {
		if service.State == "running" {
			return true
		}
	}
	return false
}
func (e *Engine) manualChanges(registry Registry, project Project, inventory Inventory, config Config, updates map[string]uint16) ([]PortChange, error) {
	reserved, owned, err := portReservations(registry, project, inventory)
	if err != nil {
		return nil, err
	}
	return PlanPortChanges(config, updates, reserved, func(port uint16) bool { return owned[port] || PortAvailable(port) })
}
func (e *Engine) reviewedChanges(registry Registry, project Project, inventory Inventory, config Config, preview RepairPreview) ([]PortChange, error) {
	if preview.Mode == "" {
		return e.changes(registry, project, inventory, config)
	}
	if preview.Mode != "manual" {
		return nil, errors.New("unsupported port preview mode")
	}
	updates := map[string]uint16{}
	for _, change := range preview.Changes {
		if _, duplicate := updates[change.Key]; duplicate {
			return nil, errors.New("duplicate port setting in preview")
		}
		if before, ok := config.Ports[change.Key]; !ok || before != change.Before {
			return nil, errors.New("port preview does not match current configuration")
		}
		updates[change.Key] = change.After
	}
	return e.manualChanges(registry, project, inventory, config, updates)
}
