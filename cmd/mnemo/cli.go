package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cameronpyne-smith/mnemo/internal/client"
	"github.com/cameronpyne-smith/mnemo/internal/config"
	"github.com/cameronpyne-smith/mnemo/internal/vault"
)

func newClient(configPath string) (*client.Client, config.Config, error) {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return nil, cfg, err
	}
	return client.New(cfg.Bind, cfg.Token), cfg, nil
}

func newAddCmd(configPath *string) *cobra.Command {
	var file, source string

	cmd := &cobra.Command{
		Use:   "add [text...]",
		Short: "Capture a dump into the vault inbox",
		RunE: func(cmd *cobra.Command, args []string) error {
			content, err := gatherContent(args, file, cmd.InOrStdin())
			if err != nil {
				return err
			}
			c, cfg, err := newClient(*configPath)
			if err != nil {
				return err
			}
			resp, err := c.Capture(content, source)
			if err == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "captured as %s\n", resp.Slug)
				return nil
			}

			slug, fallbackErr := captureDirect(cfg.Vault, content, source)
			if fallbackErr != nil {
				return fmt.Errorf("daemon unreachable (%v) and direct capture failed: %w", err, fallbackErr)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "daemon unreachable; captured directly to inbox as %s\n", slug)
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "capture the contents of a file")
	cmd.Flags().StringVar(&source, "source", "cli", "where this capture came from")
	return cmd
}

func gatherContent(args []string, file string, stdin io.Reader) (string, error) {
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", file, err)
		}
		return string(data), nil
	}
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("reading stdin: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", fmt.Errorf("nothing to capture: pass text, --file, or pipe stdin")
	}
	return string(data), nil
}

func captureDirect(vaultPath, content, source string) (string, error) {
	v, err := vault.Open(vaultPath)
	if err != nil {
		return "", err
	}
	n := vault.NewCapture(content, source, timeNow())
	if err := v.Write(n); err != nil {
		return "", err
	}
	return n.Slug, nil
}

func newSearchCmd(configPath *string) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search the vault",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient(*configPath)
			if err != nil {
				return err
			}
			resp, err := c.Search(strings.Join(args, " "), limit)
			if err != nil {
				return err
			}
			if len(resp.Results) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no matches")
				return nil
			}
			for _, r := range resp.Results {
				fmt.Fprintf(cmd.OutOrStdout(), "%-30s %s\n", r.Slug, r.Description)
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 10, "maximum results")
	return cmd
}

func newGetCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <slug>",
		Short: "Print a note",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient(*configPath)
			if err != nil {
				return err
			}
			note, err := c.Get(args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s (%s)\n%s\n", note.Slug, note.Folder, note.Description)
			if len(note.Tags) > 0 {
				fmt.Fprintf(out, "tags: %s\n", strings.Join(note.Tags, ", "))
			}
			fmt.Fprintf(out, "\n%s", note.Body)
			if len(note.Backlinks) > 0 {
				fmt.Fprintf(out, "\nbacklinks: %s\n", strings.Join(note.Backlinks, ", "))
			}
			return nil
		},
	}
}

func newStatusCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show daemon and vault status",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient(*configPath)
			if err != nil {
				return err
			}
			st, err := c.Status()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "notes: %d  hubs: %d  inbox: %d  archived: %d\n", st.Notes, st.Hubs, st.Inbox, st.Archived)
			if st.Filing.Enabled {
				fmt.Fprintf(out, "filing: %d processed, %d failed, %d in flight\n",
					st.Filing.Processed, st.Filing.Failed, st.Filing.InFlight)
			} else {
				fmt.Fprintln(out, "filing: disabled")
			}
			return nil
		},
	}
}

func newRenameCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <slug> <new-slug>",
		Short: "Rename a note, rewriting all inbound links",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := newClient(*configPath)
			if err != nil {
				return err
			}
			note, err := c.Rename(args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "renamed to %s (%d backlinks rewritten)\n", note.Slug, len(note.Backlinks))
			return nil
		},
	}
}
