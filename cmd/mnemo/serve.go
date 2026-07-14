package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/cameronpyne-smith/mnemo/internal/agent"
	"github.com/cameronpyne-smith/mnemo/internal/config"
	"github.com/cameronpyne-smith/mnemo/internal/ollama"
	"github.com/cameronpyne-smith/mnemo/internal/server"
	"github.com/cameronpyne-smith/mnemo/internal/store"
)

func newServeCmd(configPath *string) *cobra.Command {
	var noFiling bool

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the mnemo daemon: HTTP API, inbox filing agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(*configPath)
			if err != nil {
				return err
			}
			log := slog.New(slog.NewTextHandler(os.Stderr, nil))

			st, err := store.Open(cfg.Vault)
			if err != nil {
				return err
			}
			status, err := st.Status()
			if err != nil {
				return err
			}
			log.Info("vault opened", "path", cfg.Vault, "notes", status.Notes, "hubs", status.Hubs, "inbox", status.Inbox)

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()

			var worker *agent.Worker
			if !noFiling {
				filer := &agent.Filer{
					Store: st,
					LLM:   ollama.New(cfg.Ollama.BaseURL),
					Model: cfg.Ollama.AgentModel,
					Log:   log,
				}
				worker = agent.NewWorker(filer, log)
				go worker.Run(ctx)
				log.Info("filing agent running", "model", cfg.Ollama.AgentModel, "ollama", cfg.Ollama.BaseURL)
			} else {
				log.Info("filing agent disabled")
			}

			srv := &http.Server{
				Addr:    cfg.Bind,
				Handler: server.New(st, worker, cfg.Token),
			}
			errCh := make(chan error, 1)
			go func() {
				log.Info("listening", "addr", cfg.Bind)
				errCh <- srv.ListenAndServe()
			}()

			select {
			case err := <-errCh:
				return err
			case <-ctx.Done():
				log.Info("shutting down")
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
					return err
				}
				return nil
			}
		},
	}
	cmd.Flags().BoolVar(&noFiling, "no-filing", false, "serve the API without the filing agent")
	return cmd
}

func loadConfig(configPath string) (config.Config, error) {
	path := configPath
	if path == "" {
		var err error
		if path, err = config.DefaultPath(); err != nil {
			return config.Config{}, err
		}
	}
	cfg, err := config.Load(path)
	if err != nil {
		return cfg, fmt.Errorf("%w (run 'mnemo init --vault <path>' first)", err)
	}
	return cfg, nil
}
