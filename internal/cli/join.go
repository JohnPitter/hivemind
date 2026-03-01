package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/joaopedro/hivemind/internal/models"
	"github.com/joaopedro/hivemind/internal/services"
	"github.com/spf13/cobra"
)

// donationOption represents a resource donation level.
type donationOption struct {
	Pct   int
	Label string
	Desc  string
}

var donationOptions = []donationOption{
	{25, "Light", "Keep most resources for local use"},
	{50, "Balanced", "Share half of your resources"},
	{75, "Generous", "Contribute most of your resources"},
	{100, "All-in", "Donate everything to the hive"},
}

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

			// Show detected resources
			fmt.Println()
			fmt.Println(BoldStyle.Render("  Detected resources:"))
			fmt.Printf("  %s %s (%s VRAM)\n",
				LabelStyle.Render("GPU:"),
				ValueStyle.Render(resources.GPUName),
				FormatVRAM(resources.VRAMFree),
			)
			fmt.Printf("  %s %s\n",
				LabelStyle.Render("RAM:"),
				FormatVRAM(resources.RAMFree),
			)
			fmt.Println()

			// Interactive donation selection
			donationPct := interactiveDonationSelect()
			resources.DonationPct = donationPct

			// Show donation summary
			fmt.Println()
			fmt.Printf("  %s %s VRAM | %s RAM\n",
				HighlightStyle.Render("Donating:"),
				ValueStyle.Render(FormatVRAM(resources.TotalUsableVRAM())),
				ValueStyle.Render(FormatVRAM(resources.TotalUsableRAM())),
			)
			fmt.Println()

			// Continue with remaining connection steps
			finalSteps := []string{
				"Receiving layer assignment...",
				"Loading model layers...",
			}
			for _, step := range finalSteps {
				fmt.Printf("  %s %s\n", SyncingStyle.Render("◌"), step)
				time.Sleep(200 * time.Millisecond)
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

// interactiveDonationSelect prompts the user to choose a donation percentage.
func interactiveDonationSelect() int {
	fmt.Println(BoldStyle.Render("  How much do you want to donate?"))
	fmt.Println()

	for i, opt := range donationOptions {
		num := HighlightStyle.Render(fmt.Sprintf("  [%d]", i+1))
		pct := ValueStyle.Render(fmt.Sprintf("%3d%%", opt.Pct))
		label := BoldStyle.Render(opt.Label)
		desc := DimStyle.Render("— " + opt.Desc)
		fmt.Printf("%s %s %s %s\n", num, pct, label, desc)
	}

	fmt.Println()
	fmt.Print(LabelStyle.Render("  Choose (1-" + strconv.Itoa(len(donationOptions)) + "): "))

	var input string
	fmt.Scanln(&input)

	choice, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || choice < 1 || choice > len(donationOptions) {
		fmt.Println(DimStyle.Render("  Defaulting to 50% (Balanced)"))
		return 50
	}

	return donationOptions[choice-1].Pct
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
