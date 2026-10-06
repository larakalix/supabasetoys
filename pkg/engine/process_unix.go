//go:build !windows

package engine

import "os/exec"

func configureProcess(_ *exec.Cmd) {}
