package main

import (
	"fmt"
	"os"

	"github.com/joaopedro/hivemind/internal/cli"
	"github.com/joaopedro/hivemind/internal/config"
	"github.com/joaopedro/hivemind/internal/logger"
	"github.com/joaopedro/hivemind/internal/services"
	"github.com/spf13/cobra"
)

var (
	Version = "0.1.0"
	cfgFile string
	verbose bool
)

func main() {
	// Initialize mock services (replaced by real services in Phase 4)
	roomSvc := services.NewMockRoomService()
	infSvc := services.NewMockInferenceService(roomSvc)

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

	rootCmd.AddCommand(versionCmd())
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
