package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"supabasetoys/pkg/engine"
)

func readAccountToken(cmd *cobra.Command) (string, error) {
	input, ok := cmd.InOrStdin().(*os.File)
	if !ok || !term.IsTerminal(int(input.Fd())) {
		return "", errors.New("account add requires an interactive terminal for hidden token input; use the desktop app otherwise")
	}
	fmt.Fprint(cmd.ErrOrStderr(), "Supabase scoped personal access token (hidden): ")
	token, err := term.ReadPassword(int(input.Fd()))
	fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", errors.New("could not read token securely")
	}
	defer clear(token)
	return string(token), nil
}
func accountCommand(run commandRunner) *cobra.Command {
	account := &cobra.Command{Use: "account", Short: "Connect named Supabase profiles; tokens stay in the OS credential store"}
	var session bool
	add := &cobra.Command{Use: "add LABEL", Args: cobra.ExactArgs(1)}
	add.RunE = func(cmd *cobra.Command, args []string) error {
		token, err := readAccountToken(cmd)
		if err != nil {
			return err
		}
		return run(func(ctx context.Context, e *engine.Engine, _ []string) (any, error) {
			return e.AddAccount(ctx, args[0], token, session)
		})(cmd, args)
	}
	add.Flags().BoolVar(&session, "session-only", false, "Validate and display inventory without saving credentials; this CLI session ends after the command")
	account.AddCommand(add, &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: run(func(_ context.Context, e *engine.Engine, _ []string) (any, error) { return e.Accounts() })}, &cobra.Command{Use: "remove PROFILE", Args: cobra.ExactArgs(1), Short: "Disconnect locally; revoke the token separately in Supabase account settings", RunE: run(func(_ context.Context, e *engine.Engine, args []string) (any, error) {
		return map[string]string{"disconnected": args[0], "token_revocation": "Revoke in https://supabase.com/dashboard/account/tokens if no longer needed", "local_data": "preserved"}, e.RemoveAccount(args[0])
	})})
	return account
}
func cloudCommand(run commandRunner) *cobra.Command {
	cloud := &cobra.Command{Use: "cloud", Short: "Read-only hosted project inventory"}
	var account string
	var refresh bool
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: run(func(ctx context.Context, e *engine.Engine, _ []string) (any, error) {
		return e.CloudList(ctx, account, refresh)
	})}
	list.Flags().StringVar(&account, "account", "", "Account profile label or ID")
	list.Flags().BoolVar(&refresh, "refresh", false, "Refresh metadata from Supabase; retain stale cache on failure")
	_ = list.MarkFlagRequired("account")
	cloud.AddCommand(list)
	return cloud
}
func associationCommands(run commandRunner) []*cobra.Command {
	var account, ref string
	associate := &cobra.Command{Use: "associate LOCAL", Args: cobra.ExactArgs(1), Short: "Associate a local registration with an accessible hosted project; no config changes", RunE: run(func(ctx context.Context, e *engine.Engine, args []string) (any, error) {
		return e.Associate(ctx, args[0], account, ref)
	})}
	associate.Flags().StringVar(&account, "account", "", "Account label or ID used to verify access")
	associate.Flags().StringVar(&ref, "cloud-ref", "", "Hosted project reference")
	_ = associate.MarkFlagRequired("account")
	_ = associate.MarkFlagRequired("cloud-ref")
	return []*cobra.Command{associate, {Use: "unassociate LOCAL", Args: cobra.ExactArgs(1), RunE: run(func(_ context.Context, e *engine.Engine, args []string) (any, error) {
		return map[string]bool{"ok": true}, e.Unassociate(args[0])
	})}, {Use: "association-suggestions LOCAL", Args: cobra.ExactArgs(1), RunE: run(func(_ context.Context, e *engine.Engine, args []string) (any, error) {
		return e.AssociationSuggestions(args[0])
	})}}
}
