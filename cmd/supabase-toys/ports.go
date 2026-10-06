package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"supabasetoys/pkg/engine"
)

type commandWork func(context.Context, *engine.Engine, []string) (any, error)
type commandRunner func(commandWork) func(*cobra.Command, []string) error

func portsCommand(run commandRunner) *cobra.Command {
	ports := &cobra.Command{Use: "ports", Short: "Show configured ports or review explicit port changes"}
	ports.AddCommand(&cobra.Command{Use: "list PROJECT", Short: "Show supported configured host ports", Args: cobra.ExactArgs(1), RunE: run(func(ctx context.Context, e *engine.Engine, args []string) (any, error) {
		status, err := e.Status(ctx, args[0], false)
		return status.ConfiguredPorts, err
	})})
	var updates []string
	preview := &cobra.Command{Use: "preview PROJECT", Short: "Preview explicit port changes without writing", Args: cobra.ExactArgs(1), RunE: run(func(ctx context.Context, e *engine.Engine, args []string) (any, error) {
		values, err := parsePortSettings(updates)
		if err != nil {
			return nil, err
		}
		return e.PreviewPorts(ctx, args[0], values)
	})}
	preview.Flags().StringArrayVar(&updates, "set", nil, "Port setting KEY=PORT (repeatable, e.g. api.port=55000)")
	_ = preview.MarkFlagRequired("set")
	var restart bool
	apply := &cobra.Command{Use: "apply PREVIEW_FILE", Short: "Apply a reviewed allocation with a configuration backup", Args: cobra.ExactArgs(1), RunE: run(func(ctx context.Context, e *engine.Engine, args []string) (any, error) {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return nil, err
		}
		var reviewed engine.RepairPreview
		if err := json.Unmarshal(data, &reviewed); err != nil {
			return nil, fmt.Errorf("invalid port preview: %w", err)
		}
		if reviewed.Mode != "manual" {
			return nil, fmt.Errorf("expected a manual port preview from ports preview")
		}
		return e.ApplyRepair(ctx, reviewed, restart)
	})}
	apply.Flags().BoolVar(&restart, "restart", false, "Approve restarting only the selected project")
	ports.AddCommand(preview, apply)
	return ports
}
func parsePortSettings(settings []string) (map[string]uint16, error) {
	values := map[string]uint16{}
	for _, setting := range settings {
		key, value, ok := strings.Cut(setting, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("expected KEY=PORT, got %q", setting)
		}
		port, err := strconv.ParseUint(value, 10, 16)
		if err != nil || port == 0 {
			return nil, fmt.Errorf("%s must be an integer between 1 and 65535", key)
		}
		if _, duplicate := values[key]; duplicate {
			return nil, fmt.Errorf("duplicate port setting %s", key)
		}
		values[key] = uint16(port)
	}
	return values, nil
}
