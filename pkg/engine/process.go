package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type CommandSpec struct {
	Executable string
	Args       []string
	Dir        string
	Env        map[string]string
	Timeout    time.Duration
}
type Output struct {
	Success bool
	Stdout  string
	Stderr  string
}

// Runner is the process boundary used by the engine and compatibility fixtures.
type Runner interface {
	Run(context.Context, CommandSpec) (Output, error)
}
type SystemRunner struct{}

var _ Runner = SystemRunner{}

func command(executable string, args ...string) CommandSpec {
	return CommandSpec{Executable: executable, Args: args, Env: map[string]string{}, Timeout: 20 * time.Second}
}

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(data []byte) (int, error) {
	remaining := 4*1024*1024 - b.Len()
	if remaining > 0 {
		if _, err := b.Buffer.Write(data[:min(remaining, len(data))]); err != nil {
			return 0, err
		}
	}
	return len(data), nil
}
func (SystemRunner) Run(ctx context.Context, spec CommandSpec) (Output, error) {
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return Output{}, fmt.Errorf("operation cancelled; refresh project status: %w", err)
	}
	cmd := exec.CommandContext(ctx, spec.Executable, spec.Args...)
	cmd.Dir = spec.Dir
	// Drain bounded buffers and cap waits for pipes inherited by child processes.
	cmd.WaitDelay = time.Second
	configureProcess(cmd)
	env := map[string]string{}
	for _, pair := range os.Environ() {
		key, value, _ := strings.Cut(pair, "=")
		switch key {
		case "DOCKER_CONTEXT", "DOCKER_HOST", "SUPABASE_WORKDIR", "SUPABASE_PROJECT_ID", "SUPABASE_EXPERIMENTAL_STACK", "SUPABASE_STACK", "SUPABASE_ACCESS_TOKEN":
			continue
		}
		if strings.HasPrefix(key, "SUPABASE_") && strings.HasSuffix(key, "PORT") {
			continue
		}
		env[key] = value
	}
	for key, value := range spec.Env {
		env[key] = value
	}
	env["SUPA_TELEMETRY"] = "off"
	cmd.Env = make([]string, 0, len(env))
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	var stdout, stderr boundedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	output := Output{Success: err == nil, Stdout: stdout.String(), Stderr: stderr.String()}
	if ctx.Err() != nil {
		return output, fmt.Errorf("command timed out or was cancelled; refresh status because some services may have started: %w", ctx.Err())
	}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return output, fmt.Errorf("cannot run %q; install it or select its executable in Settings: %w", spec.Executable, err)
	}
	return output, nil
}
func require(output Output, err error) (string, error) {
	if err != nil {
		return "", err
	}
	if !output.Success {
		return "", errors.New(Redact(output.Stderr[:min(len(output.Stderr), 4000)]))
	}
	return output.Stdout, nil
}
