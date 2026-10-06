package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestCLIHelpAndJSONRegistry(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"project", "--help"}, {"doctor", "--help"}, {"env", "--help"}, {"diagnostics", "--help"}} {
		root := newCommand()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs(args)
		if err := root.ExecuteContext(t.Context()); err != nil || output.Len() == 0 {
			t.Fatal(args, err)
		}
	}
	root := newCommand()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetArgs([]string{"--json", "--data-dir", filepath.Join(t.TempDir(), "registry"), "project", "list"})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(output.Bytes()) || output.String() != "[]\n" {
		t.Fatal(output.String())
	}
}
func TestCLIFlagValidation(t *testing.T) {
	for _, args := range [][]string{{"doctor", "A", "--save", "preview.json"}, {"doctor", "A", "--restart"}, {"doctor", "A", "--preview", "--apply", "preview.json"}, {"project", "remove"}, {"start", "A", "--all"}} {
		root := newCommand()
		root.SetArgs(args)
		if err := root.ExecuteContext(t.Context()); err == nil {
			t.Fatal(args)
		}
	}
}
