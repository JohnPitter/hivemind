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
	Version = "1.0.0"
	cfgFile string
	verbose bool
)

func main() {
	useMock := os.Getenv("HIVEMIND_MOCK") == "true"

	// Load config early for service construction
	cfg, err := config.Load(cfgFile)
	if err != nil {
		cfg = &config.Config{}
		cfg.API.RateLimit = 60
		cfg.API.MaxBodyBytes = 10 * 1024 * 1024
		cfg.Signaling.URL = "http://localhost:7777"
		cfg.Signaling.Port = 7777
		cfg.Mesh.WireGuardPort = 51820
		cfg.Mesh.GRPCPort = 50052
		cfg.Worker.GRPCPort = 50051
		cfg.Worker.MaxRestarts = 3
	}

	var roomSvc services.RoomService
	var peerRegistry *infra.PeerRegistry

	if useMock {
		roomSvc = services.NewMockRoomService()
		logger.Init(logger.LevelInfo)
		logger.Info("using mock services", "reason", "HIVEMIND_MOCK=true")
	} else {
		peerID := infra.GetOrCreatePeerID()

		// Peer endpoint from config or env
		endpoint := cfg.Peer.Endpoint
		if endpoint == "" {
			endpoint = infra.GetLocalIP() + ":51820"
		}

		wgManager := infra.NewWireGuardManager(cfg.Mesh.ConfigDir)
		sigClient := infra.NewSignalingClient(cfg.Signaling.URL)
		peerRegistry = infra.NewPeerRegistry(peerID, cfg.Mesh.GRPCPort)

		// Initialize NAT traversal if enabled
		var natTraversal *infra.NATTraversal
		if cfg.NAT.Enabled {
			var turnCfg []infra.TURNConfig
			if cfg.NAT.TURNServer != "" {
				turnCfg = append(turnCfg, infra.TURNConfig{
					Server: cfg.NAT.TURNServer,
					User:   cfg.NAT.TURNUser,
					Pass:   cfg.NAT.TURNPass,
				})
			}
			natTraversal = infra.NewNATTraversal(cfg.NAT.STUNServers, turnCfg...)
			logger.Info("NAT traversal enabled",
				"component", "nat",
				"stun_servers", fmt.Sprintf("%v", cfg.NAT.STUNServers),
				"turn_configured", cfg.NAT.TURNServer != "",
			)
		}

		roomSvc = services.NewRealRoomService(services.RealRoomConfig{
			LocalPeerID:   peerID,
			Endpoint:      endpoint,
			WireGuardPort: cfg.Mesh.WireGuardPort,
			GRPCPort:      cfg.Mesh.GRPCPort,
		}, sigClient, wgManager, peerRegistry, natTraversal)
	}

	var infSvc services.InferenceService
	if useMock {
		infSvc = services.NewMockInferenceService(roomSvc)
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
			Port:        cfg.Worker.GRPCPort,
			PythonCmd:   pythonCmd,
			WorkerDir:   workerDir,
			MaxRestarts: cfg.Worker.MaxRestarts,
		})
		realInf := services.NewRealInferenceService(roomSvc, wm)
		if peerRegistry != nil {
			peerID := infra.GetOrCreatePeerID()
			realInf.SetLocalPeerID(peerID)

			// Wire distributed inference for multi-peer generation
			distInf := services.NewDistributedInferenceService(
				wm.Client,
				peerRegistry,
				roomSvc,
				peerID,
			)
			realInf.SetDistributedInference(distInf)
			logger.Info("distributed inference wired", "peer_id", peerID)
		}
		infSvc = realInf
	}

	rootCmd := &cobra.Command{
		Use:   "hivemind",
		Short: "Distributed P2P AI inference",
		Long:  "HiveMind — Share bare metal resources to run large AI models cooperatively via tensor parallelism.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			loadedCfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			level := logger.LevelInfo
			if verbose {
				level = logger.LevelDebug
			}
			logger.Init(level)
			logger.Debug("config loaded", "path", loadedCfg.ConfigPath)

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
	rootCmd.AddCommand(cli.ServeCmd(webFS, roomSvc, infSvc, cfg))
	rootCmd.AddCommand(cli.WebCmd(webFS, roomSvc, infSvc))
	rootCmd.AddCommand(cli.HealthCheckCmd())
	rootCmd.AddCommand(cli.SignalingCmd())
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
