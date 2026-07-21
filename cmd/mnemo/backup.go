package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cameronpyne-smith/mnemo/internal/config"
	"github.com/cameronpyne-smith/mnemo/internal/gitsync"
)

func newBackupCmd(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Manage git backup remotes for the vault",
	}
	cmd.AddCommand(newBackupInitCmd(configPath))
	return cmd
}

func newBackupInitCmd(configPath *string) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "init <path>",
		Short: "Create an append-only bare repo (e.g. on an external drive) and register it as a push remote",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			abs, err := filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("resolving %s: %w", args[0], err)
			}
			if err := gitsync.InitBare(abs); err != nil {
				return err
			}

			path := *configPath
			if path == "" {
				if path, err = config.DefaultPath(); err != nil {
					return err
				}
			}
			cfg, err := config.Load(path)
			if err != nil {
				return fmt.Errorf("%w (run 'mnemo init --vault <path>' first)", err)
			}
			if cfg.Git.Remotes == nil {
				cfg.Git.Remotes = make(map[string]string)
			}
			cfg.Git.Remotes[name] = abs
			if err := config.Save(path, cfg); err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "bare repo created at %s (append-only: history cannot be rewritten or deleted by pushers)\n", abs)
			fmt.Fprintf(out, "registered as remote %q in %s — restart the daemon to start pushing\n", name, path)
			fmt.Fprintln(out, "reminder: if this is a removable drive, enable BitLocker To Go on it")
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "backup", "git remote name for this target")
	return cmd
}
