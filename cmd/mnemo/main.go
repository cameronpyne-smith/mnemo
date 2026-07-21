package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/cameronpyne-smith/mnemo/internal/config"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var configPath string

	root := &cobra.Command{
		Use:           "mnemo",
		Short:         "mnemo manages a second-brain vault of linked markdown notes",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", "path to config file")
	root.AddCommand(
		newInitCmd(&configPath),
		newServeCmd(&configPath),
		newAddCmd(&configPath),
		newSearchCmd(&configPath),
		newGetCmd(&configPath),
		newStatusCmd(&configPath),
		newRenameCmd(&configPath),
		newBackupCmd(&configPath),
	)
	return root
}

var timeNow = time.Now

func newInitCmd(configPath *string) *cobra.Command {
	var vaultPath string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create the vault folder skeleton, root hub, and config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			absVault, err := filepath.Abs(vaultPath)
			if err != nil {
				return fmt.Errorf("resolving vault path: %w", err)
			}
			if _, err := vault.Init(absVault); err != nil {
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
				cfg = config.Default()
			}
			cfg.Vault = absVault
			if err := config.Save(path, cfg); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "vault initialised at %s\nconfig written to %s\n", absVault, path)
			return nil
		},
	}
	cmd.Flags().StringVar(&vaultPath, "vault", "", "path to the vault directory")
	cmd.MarkFlagRequired("vault")
	return cmd
}
