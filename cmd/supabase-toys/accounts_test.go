package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"supabasetoys/pkg/engine"
)

func TestAccountAddRefusesNonInteractiveTokenInput(t *testing.T) {
	root := newCommand()
	root.SetIn(strings.NewReader("sbp_private\n"))
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--data-dir", t.TempDir(), "account", "add", "Personal"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") || strings.Contains(err.Error(), "sbp_private") {
		t.Fatal(err)
	}
}
func TestAccountTokenFlagIsNotAvailable(t *testing.T) {
	root := newCommand()
	root.SetArgs([]string{"account", "add", "Personal", "--token", "sbp_private"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown flag") || strings.Contains(err.Error(), "sbp_private") {
		t.Fatal(err)
	}
}
func TestCloudCommandPassesExplicitAccountAndRefresh(t *testing.T) {
	var selected []string
	run := commandRunner(func(work commandWork) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			e, err := engine.New(t.TempDir())
			if err != nil {
				return err
			}
			_, err = work(context.Background(), e, args)
			selected = append(selected, cmd.Flag("account").Value.String(), cmd.Flag("refresh").Value.String())
			return err
		}
	})
	cmd := cloudCommand(run)
	cmd.SetArgs([]string{"list", "--account", "Work", "--refresh"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("unknown account accepted")
	}
	if len(selected) != 2 || selected[0] != "Work" || selected[1] != "true" {
		t.Fatal(selected)
	}
}
func TestAccountAndAssociationCommandHelp(t *testing.T) {
	for _, args := range [][]string{{"account", "--help"}, {"cloud", "list", "--help"}, {"project", "associate", "--help"}} {
		root := newCommand()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if output.Len() == 0 {
			t.Fatal("help empty")
		}
	}
}
