package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/joaopedro/hivemind/internal/models"
	"github.com/joaopedro/hivemind/internal/services"
	"github.com/spf13/cobra"
)

func joinCmd(roomSvc services.RoomService) *cobra.Command {
	return &cobra.Command{
		Use:   "join [invite-code]",
		Short: "Join an existing room",
		Long:  "Join a HiveMind room using an invite code. Your machine will contribute resources to the shared model.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inviteCode := args[0]

			fmt.Println()
			fmt.Println(TitleStyle.Render("🐝 Joining HiveMind room"))
			fmt.Println()

			// Simulate connection steps with progress
			steps := []string{
				"Connecting to signaling server...",
				"Validating invite code...",
				"Exchanging WireGuard keys...",
				"Establishing mesh connection...",
				"Syncing room state...",
				"Detecting local resources...",
				"Receiving layer assignment...",
				"Loading model layers...",
			}

			for _, step := range steps {
				fmt.Printf("  %s %s\n", SyncingStyle.Render("◌"), step)
				time.Sleep(200 * time.Millisecond)
			}

			// Mock resources for joining
			resources := models.ResourceSpec{
				GPUName:   "NVIDIA RTX 3060",
				VRAMTotal: 12288,
				VRAMFree:  10240,
				RAMTotal:  32768,
				RAMFree:   24576,
				CUDAAvail: true,
				Platform:  "Windows",
			}

			room, err := roomSvc.Join(context.Background(), inviteCode, resources)
			if err != nil {
				return fmt.Errorf("failed to join room: %w", err)
			}

			fmt.Println()
			renderRoomJoined(room)

			return nil
		},
	}
}

func renderRoomJoined(room *models.Room) {
	// Find self
	var selfPeer *models.Peer
	for i := range room.Peers {
		if room.Peers[i].ID == "self" {
			selfPeer = &room.Peers[i]
			break
		}
	}

	layerRange := "none"
	if selfPeer != nil && len(selfPeer.Layers) > 0 {
		layerRange = fmt.Sprintf("%d-%d (%d layers)",
			selfPeer.Layers[0],
			selfPeer.Layers[len(selfPeer.Layers)-1],
			len(selfPeer.Layers))
	}

	content := fmt.Sprintf(
		"%s\n\n"+
			"%s %s\n"+
			"%s %s\n"+
			"%s %d/%d\n"+
			"%s %s\n\n"+
			"%s\n"+
			"%s",
		HighlightStyle.Render("✓ Successfully joined room!"),
		LabelStyle.Render("Model:       "), ValueStyle.Render(room.ModelID),
		LabelStyle.Render("Room:        "), ValueStyle.Render(room.ID),
		LabelStyle.Render("Peers:       "), len(room.Peers), room.MaxPeers,
		LabelStyle.Render("Your layers: "), ValueStyle.Render(layerRange),
		DimStyle.Render("Run 'hivemind status' to see room details"),
		DimStyle.Render("Run 'hivemind chat' to start chatting"),
	)

	fmt.Println(SuccessBoxStyle.Render(content))
	fmt.Println()
}
