package cli

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/joaopedro/hivemind/internal/api"
	"github.com/joaopedro/hivemind/internal/config"
	"github.com/joaopedro/hivemind/internal/logger"
	"github.com/joaopedro/hivemind/internal/services"
	"github.com/spf13/cobra"
)

// ServeCmd creates the HTTP API server command.
func ServeCmd(webFS fs.FS, roomSvc services.RoomService, infSvc services.InferenceService, cfg *config.Config) *cobra.Command {
	var host string
	var port int

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP API server",
		Long:  "Launch the HiveMind API server with OpenAI-compatible endpoints, web dashboard, and health monitoring.",
		RunE: func(cmd *cobra.Command, args []string) error {
			srv := api.NewServer(port, webFS, roomSvc, infSvc, cfg)

			addr := fmt.Sprintf("%s:%d", host, port)

			fmt.Println()
			fmt.Println(TitleStyle.Render("🐝 HiveMind API Server"))
			fmt.Println()

			content := fmt.Sprintf(
				"%s %s\n%s %s\n%s %s\n\n%s",
				LabelStyle.Render("API:"),
				HighlightStyle.Render(fmt.Sprintf("http://%s", addr)),
				LabelStyle.Render("Dashboard:"),
				HighlightStyle.Render(fmt.Sprintf("http://%s/", addr)),
				LabelStyle.Render("Health:"),
				HighlightStyle.Render(fmt.Sprintf("http://%s/health", addr)),
				DimStyle.Render("Press Ctrl+C to stop"),
			)
			fmt.Println(InfoBoxStyle.Render(content))
			fmt.Println()

			logger.Info("server starting", "addr", addr)

			// Graceful shutdown
			stop := make(chan os.Signal, 1)
			signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

			errCh := make(chan error, 1)
			go func() {
				errCh <- http.ListenAndServe(addr, srv.Handler())
			}()

			select {
			case err := <-errCh:
				return fmt.Errorf("server error: %w", err)
			case sig := <-stop:
				logger.Info("shutting down", "signal", sig.String())
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&host, "host", "0.0.0.0", "bind address")
	cmd.Flags().IntVarP(&port, "port", "p", 8080, "server port")

	return cmd
}
