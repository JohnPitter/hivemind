package cli

import (
	"fmt"
	"io/fs"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/joaopedro/hivemind/internal/handlers"
	"github.com/joaopedro/hivemind/internal/services"
	"github.com/spf13/cobra"
)

// WebCmd creates the web dashboard command.
func WebCmd(webFS fs.FS, roomSvc services.RoomService, infSvc services.InferenceService) *cobra.Command {
	var port int
	var noBrowser bool

	cmd := &cobra.Command{
		Use:   "web",
		Short: "Start the web dashboard",
		Long:  "Launch the HiveMind web dashboard in your browser for real-time room monitoring and chat.",
		RunE: func(cmd *cobra.Command, args []string) error {
			handler := handlers.NewWebHandler(webFS, roomSvc, infSvc)

			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)

			addr := fmt.Sprintf("127.0.0.1:%d", port)
			url := fmt.Sprintf("http://%s", addr)

			fmt.Println()
			fmt.Println(TitleStyle.Render("🐝 HiveMind Dashboard"))
			fmt.Println()

			content := fmt.Sprintf(
				"%s %s\n\n%s",
				LabelStyle.Render("Dashboard:"),
				HighlightStyle.Render(url),
				DimStyle.Render("Press Ctrl+C to stop"),
			)
			fmt.Println(InfoBoxStyle.Render(content))
			fmt.Println()

			if !noBrowser {
				openBrowser(url)
			}

			return http.ListenAndServe(addr, mux)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 3000, "dashboard port")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "don't open browser automatically")

	return cmd
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
