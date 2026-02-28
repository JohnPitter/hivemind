package cli

import (
	"github.com/joaopedro/hivemind/internal/services"
	"github.com/spf13/cobra"
)

// RegisterCommands adds all CLI subcommands to the root command.
func RegisterCommands(root *cobra.Command, roomSvc services.RoomService, infSvc services.InferenceService) {
	root.AddCommand(
		createCmd(roomSvc),
		joinCmd(roomSvc),
		statusCmd(roomSvc),
		chatCmd(infSvc, roomSvc),
		leaveCmd(roomSvc),
		stopCmd(roomSvc),
	)
}
