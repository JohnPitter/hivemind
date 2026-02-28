package main

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/joaopedro/hivemind/internal/cli"
	"github.com/joaopedro/hivemind/internal/config"
	"github.com/joaopedro/hivemind/internal/infra"
	"github.com/joaopedro/hivemind/internal/logger"
	"github.com/joaopedro/hivemind/internal/services"
	webpkg "github.com/joaopedro/hivemind/web"
	"github.com/spf13/cobra"
)

var (
	Version = "0.2.0"
	cfgFile string
	verbose bool
)

func main() {
	// Initialize services — real inference is the default, mock is opt-in
	roomSvc := services.NewMockRoomService()

	var infSvc services.InferenceService
	if os.Getenv("HIVEMIND_MOCK") == "true" {
		infSvc = services.NewMockInferenceService(roomSvc)
		logger.Init(logger.LevelInfo)
		logger.Info("using mock inference service", "reason", "HIVEMIND_MOCK=true")
	} else {
		pythonCmd := "python3"
		workerDir := "/app/worker"
		if dir := os.Getenv("HIVEMIND_WORKER_DIR"); dir != "" {
			workerDir = dir
		}
		if cmd := os.Getenv("HIVEMIND_PYTHON_CMD"); cmd != "" {
			pythonCmd = cmd
		}

		wm := infra.NewWorkerManager(infra.WorkerConfig{
			Port:        50051,
			PythonCmd:   pythonCmd,
			WorkerDir:   workerDir,
			MaxRestarts: 3,
		})
		infSvc = services.NewRealInferenceService(roomSvc, wm)
	}

	rootCmd := &cobra.Command{
		Use:   "hivemind",
		Short: "Distributed P2P AI inference",
		Long:  "HiveMind — Share bare metal resources to run large AI models cooperatively via tensor parallelism.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			level := logger.LevelInfo
			if verbose {
				level = logger.LevelDebug
			}
			logger.Init(level)
			logger.Debug("config loaded", "path", cfg.ConfigPath)

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(cli.Logo())
			fmt.Println()
			return cmd.Help()
		},
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.hivemind/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")

	webFS, _ := fs.Sub(webpkg.Dist, "dist")

	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(cli.ServeCmd(webFS, roomSvc, infSvc))
	rootCmd.AddCommand(cli.WebCmd(webFS, roomSvc, infSvc))
	rootCmd.AddCommand(cli.HealthCheckCmd())
	cli.RegisterCommands(rootCmd, roomSvc, infSvc)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print HiveMind version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("HiveMind v%s\n", Version)
		},
	}
}
