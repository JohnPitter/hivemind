package cli

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

// HealthCheckCmd creates the health check command for Docker healthchecks.
func HealthCheckCmd() *cobra.Command {
	var port int
	var quiet bool

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Check if the API server is healthy",
		Long:  "Performs a health check against the running API server. Useful for Docker HEALTHCHECK and monitoring.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := &http.Client{Timeout: 5 * time.Second}
			url := fmt.Sprintf("http://127.0.0.1:%d/health", port)

			resp, err := client.Get(url)
			if err != nil {
				if !quiet {
					fmt.Printf("❌ Server unreachable: %v\n", err)
				}
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				if !quiet {
					fmt.Printf("❌ Unhealthy (HTTP %d)\n", resp.StatusCode)
				}
				return fmt.Errorf("unhealthy: HTTP %d", resp.StatusCode)
			}

			if !quiet {
				fmt.Println("✅ Healthy")
			}
			return nil
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8080, "server port to check")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "suppress output (exit code only)")

	return cmd
}
