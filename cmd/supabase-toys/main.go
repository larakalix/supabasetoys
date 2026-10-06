package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"supabasetoys/pkg/engine"
)

const version = "0.1.0"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	root := newCommand()
	if err := root.ExecuteContext(ctx); err != nil {
		machine, flagErr := root.PersistentFlags().GetBool("json")
		if flagErr != nil {
			machine = false
		}
		message := engine.Redact(err.Error())
		if machine {
			encoded, encodeErr := json.Marshal(map[string]string{"error": message})
			if encodeErr == nil {
				fmt.Fprintln(root.ErrOrStderr(), string(encoded))
			}
		} else {
			fmt.Fprintln(root.ErrOrStderr(), "Error:", message)
		}
		os.Exit(1)
	}
}
func newCommand() *cobra.Command {
	var machine bool
	var dataDir string
	root := &cobra.Command{Use: "supabase-toys", Short: "Local Supabase projects, without cross-project surprises", Version: version, SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().BoolVar(&machine, "json", false, "Machine-readable JSON output")
	root.PersistentFlags().StringVar(&dataDir, "data-dir", "", "Isolated registry directory")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return err })
	write := func(value any) error {
		encoder := json.NewEncoder(root.OutOrStdout())
		if !machine {
			encoder.SetIndent("", "  ")
		}
		return encoder.Encode(value)
	}
	report := func(err error) error { return err }
	run := func(work commandWork) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			core, err := engine.New(dataDir)
			if err != nil {
				return report(err)
			}
			value, err := work(cmd.Context(), core, args)
			if err != nil {
				return report(err)
			}
			if value == nil {
				return nil
			}
			return report(write(value))
		}
	}
	project := &cobra.Command{Use: "project", Short: "Register folders; runtime data is preserved"}
	var name, stackID string
	add := &cobra.Command{Use: "add PATH", Args: cobra.ExactArgs(1), RunE: run(func(ctx context.Context, e *engine.Engine, args []string) (any, error) {
		return e.Add(ctx, args[0], name, stackID)
	})}
	add.Flags().StringVar(&name, "name", "", "Display name")
	add.Flags().StringVar(&stackID, "stack-id", "", "Full existing experimental Docker stack ID")
	project.AddCommand(add, &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: run(func(_ context.Context, e *engine.Engine, _ []string) (any, error) {
		registry, err := e.Registry()
		return registry.Projects, err
	})}, &cobra.Command{Use: "remove PROJECT", Args: cobra.ExactArgs(1), RunE: run(func(_ context.Context, e *engine.Engine, args []string) (any, error) {
		return map[string]string{"removed": args[0], "runtime_data": "preserved"}, e.Remove(args[0])
	})})
	var identitySave string
	identityPreview := &cobra.Command{Use: "identity-preview PROJECT NEW_ID", Args: cobra.ExactArgs(2), RunE: run(func(ctx context.Context, e *engine.Engine, args []string) (any, error) {
		preview, err := e.PreviewIdentity(ctx, args[0], args[1])
		if err != nil {
			return nil, err
		}
		if identitySave != "" {
			if err := saveJSON(identitySave, preview); err != nil {
				return nil, err
			}
		}
		return preview, nil
	})}
	identityPreview.Flags().StringVar(&identitySave, "save", "", "Save a reviewed unused-identity proposal")
	project.AddCommand(identityPreview, &cobra.Command{Use: "identity-apply PREVIEW", Args: cobra.ExactArgs(1), RunE: run(func(ctx context.Context, e *engine.Engine, args []string) (any, error) {
		var preview engine.IdentityPreview
		if err := readJSON(args[0], &preview); err != nil {
			return nil, err
		}
		return e.ApplyIdentity(ctx, preview)
	})})
	root.AddCommand(project)
	var supabaseCLI, dockerCLI, dockerEndpoint string
	settings := &cobra.Command{Use: "settings", Args: cobra.NoArgs, RunE: run(func(_ context.Context, e *engine.Engine, _ []string) (any, error) {
		registry, err := e.Registry()
		if err != nil {
			return nil, err
		}
		configured := registry.Settings
		if supabaseCLI != "" {
			configured.SupabaseCLI = supabaseCLI
		}
		if dockerCLI != "" {
			configured.DockerCLI = dockerCLI
		}
		if dockerEndpoint != "" {
			configured.DockerEndpoint = &dockerEndpoint
		}
		if supabaseCLI != "" || dockerCLI != "" || dockerEndpoint != "" {
			if err := e.Configure(configured); err != nil {
				return nil, err
			}
		}
		return configured, nil
	})}
	settings.Flags().StringVar(&supabaseCLI, "supabase-cli", "", "Selected Supabase executable")
	settings.Flags().StringVar(&dockerCLI, "docker-cli", "", "Selected Docker executable")
	settings.Flags().StringVar(&dockerEndpoint, "docker-endpoint", "", "Pinned local Docker endpoint")
	root.AddCommand(settings, &cobra.Command{Use: "inventory", Args: cobra.NoArgs, RunE: run(func(ctx context.Context, e *engine.Engine, _ []string) (any, error) { return e.Inventory(ctx) })})
	for _, operation := range []string{"start", "stop", "restart"} {
		root.AddCommand(&cobra.Command{Use: operation + " PROJECT", Args: cobra.ExactArgs(1), RunE: run(func(ctx context.Context, e *engine.Engine, args []string) (any, error) {
			return e.Lifecycle(ctx, args[0], operation, func(progress engine.OperationProgress) {
				if machine {
					encoded, _ := json.Marshal(progress)
					fmt.Fprintln(root.ErrOrStderr(), string(encoded))
				} else {
					fmt.Fprintln(root.ErrOrStderr(), progress.Message)
				}
			})
		})})
	}
	var resources bool
	status := &cobra.Command{Use: "status PROJECT", Args: cobra.ExactArgs(1), RunE: run(func(ctx context.Context, e *engine.Engine, args []string) (any, error) {
		return e.Status(ctx, args[0], resources)
	})}
	status.Flags().BoolVar(&resources, "resources", false, "Sample CPU and RAM for explicit project container IDs")
	root.AddCommand(status)
	var preview, restart bool
	var proposal, apply string
	doctor := &cobra.Command{Use: "doctor PROJECT", Args: cobra.ExactArgs(1), RunE: run(func(ctx context.Context, e *engine.Engine, args []string) (any, error) {
		if apply != "" {
			var reviewed engine.RepairPreview
			if err := readJSON(apply, &reviewed); err != nil {
				return nil, err
			}
			status, err := e.Status(ctx, args[0], false)
			if err != nil {
				return nil, err
			}
			if reviewed.Project != status.Project.ID {
				return nil, fmt.Errorf("preview belongs to a different project")
			}
			return e.ApplyRepair(ctx, reviewed, restart)
		}
		if preview {
			proposed, err := e.PreviewRepair(ctx, args[0])
			if err != nil {
				return nil, err
			}
			if proposal != "" {
				if err := saveJSON(proposal, proposed); err != nil {
					return nil, err
				}
			}
			return proposed, nil
		}
		state, err := e.Status(ctx, args[0], true)
		return map[string]any{"project": state.Project, "state": state.State, "supabase_version": state.SupabaseVersion, "diagnostics": state.Diagnostics, "resources_error": state.ResourcesError}, err
	})}
	doctor.Flags().BoolVar(&preview, "preview", false, "Preview conflict-only port repair")
	doctor.Flags().StringVar(&proposal, "save", "", "Save the proposal for review")
	doctor.Flags().StringVar(&apply, "apply", "", "Apply the reviewed proposal")
	doctor.Flags().BoolVar(&restart, "restart", false, "Approve restarting only this project during repair")
	doctor.MarkFlagsMutuallyExclusive("preview", "apply")
	doctor.PreRunE = func(cmd *cobra.Command, _ []string) error {
		if cmd.Flags().Changed("save") && !preview {
			return fmt.Errorf("--save requires --preview")
		}
		if restart && apply == "" {
			return fmt.Errorf("--restart requires --apply")
		}
		return nil
	}
	root.AddCommand(doctor)
	var service string
	var tail int
	var follow bool
	logs := &cobra.Command{Use: "logs PROJECT", Args: cobra.ExactArgs(1), RunE: run(func(ctx context.Context, e *engine.Engine, args []string) (any, error) {
		options := engine.LogOptions{Service: service, Tail: tail}
		for {
			next := fmt.Sprint(time.Now().Unix())
			text, err := e.Logs(ctx, args[0], options)
			if err != nil {
				return nil, err
			}
			if !follow {
				return map[string]string{"logs": text}, nil
			}
			if machine {
				if err := write(map[string]string{"logs": text}); err != nil {
					return nil, err
				}
			} else {
				if _, err := fmt.Fprint(root.OutOrStdout(), text); err != nil {
					return nil, err
				}
			}
			options.Since = next
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, nil
			case <-timer.C:
			}
		}
	})}
	logs.Flags().StringVar(&service, "service", "", "Selected project service name or ID")
	logs.Flags().IntVar(&tail, "tail", 100, "Recent lines per service (maximum 1000)")
	logs.Flags().BoolVar(&follow, "follow", false, "Follow bounded log batches")
	root.AddCommand(logs)
	var file string
	var mappings []string
	var writeEnv, showSecrets bool
	env := &cobra.Command{Use: "env PROJECT", Args: cobra.ExactArgs(1), RunE: run(func(ctx context.Context, e *engine.Engine, args []string) (any, error) {
		mapping := map[string]string{"API_URL": "SUPABASE_URL", "ANON_KEY": "SUPABASE_ANON_KEY"}
		if len(mappings) > 0 {
			mapping = map[string]string{}
			for _, entry := range mappings {
				source, target, found := strings.Cut(entry, "=")
				if !found {
					return nil, fmt.Errorf("use --map SOURCE=TARGET")
				}
				if _, exists := mapping[source]; exists {
					return nil, fmt.Errorf("duplicate source mapping")
				}
				mapping[source] = target
			}
		}
		proposal, err := e.PreviewEnv(ctx, args[0], file, mapping)
		if err != nil {
			return nil, err
		}
		if writeEnv {
			if err := e.ApplyEnv(ctx, proposal, mapping); err != nil {
				return nil, err
			}
		}
		if !showSecrets {
			for key, value := range proposal.Updates {
				hidden := engine.Privileged(key) || engine.PrivilegedValue(value) || strings.Contains(key, "KEY") || strings.Contains(key, "TOKEN")
				for source, target := range mapping {
					hidden = hidden || (target == key && (strings.Contains(source, "KEY") || engine.Privileged(source)))
				}
				if hidden {
					proposal.Updates[key] = "[REDACTED]"
				}
			}
		}
		return map[string]any{"preview": proposal, "written": writeEnv}, nil
	})}
	env.Flags().StringVar(&file, "file", ".env.local", "Relative project environment file")
	env.Flags().StringArrayVar(&mappings, "map", []string{}, "SOURCE=TARGET mapping, repeatable")
	env.Flags().BoolVar(&writeEnv, "write", false, "Explicitly apply the environment proposal")
	env.Flags().BoolVar(&showSecrets, "show-secrets", false, "Reveal credentials in explicit output")
	root.AddCommand(env)
	diagnostics := &cobra.Command{Use: "diagnostics"}
	var output string
	var includeLogs bool
	export := &cobra.Command{Use: "export PROJECT", Args: cobra.ExactArgs(1), RunE: run(func(ctx context.Context, e *engine.Engine, args []string) (any, error) {
		report, err := e.DiagnosticReport(ctx, args[0], includeLogs)
		if err != nil {
			return nil, err
		}
		if output != "" {
			return map[string]string{"exported": output}, engine.AtomicWrite(output, []byte(report))
		}
		var value any
		if err := json.Unmarshal([]byte(report), &value); err != nil {
			return nil, fmt.Errorf("invalid diagnostic report: %w", err)
		}
		return value, nil
	})}
	export.Flags().StringVar(&output, "output", "", "Write redacted JSON report")
	export.Flags().BoolVar(&includeLogs, "include-logs", false, "Include bounded redacted logs")
	diagnostics.AddCommand(export)
	root.AddCommand(diagnostics)
	// Cobra validation errors occur before RunE; keep machine-readable error output.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return report(err) })
	root.AddCommand(portsCommand(run))
	return root
}
func saveJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode preview: %w", err)
	}
	return engine.AtomicWrite(path, data)
}
func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read preview: %w", err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("invalid preview: %w", err)
	}
	return nil
}
