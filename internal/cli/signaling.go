package cli

import (
	"github.com/joaopedro/hivemind/internal/infra"
	"github.com/spf13/cobra"
)

// SignalingCmd creates the signaling/rendezvous server command.
func SignalingCmd() *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "signaling",
		Short: "Run the HiveMind signaling/rendezvous server",
		Long:  "Start the signaling server for room discovery and WireGuard key exchange between peers.",
		RunE: func(cmd *cobra.Command, args []string) error {
			srv := infra.NewSignalingServer(port)
			return srv.Start(cmd.Context())
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 7777, "signaling server port")

	return cmd
}
