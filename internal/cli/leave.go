package cli

import (
	"context"
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/joaopedro/hivemind/internal/models"
	"github.com/joaopedro/hivemind/internal/services"
	"github.com/spf13/cobra"
)

func leaveCmd(roomSvc services.RoomService) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "leave",
		Short: "Leave the current room",
		Long:  "Leave the room gracefully. Your layers will be redistributed among remaining peers.",
		RunE: func(cmd *cobra.Command, args []string) error {
			room := roomSvc.CurrentRoom()
			if room == nil {
				fmt.Println()
				fmt.Println(InfoBoxStyle.Render(DimStyle.Render("Not in any room.")))
				fmt.Println()
				return nil
			}

			if !force {
				fmt.Println()
				fmt.Printf("  Leave room %s (%s)?\n",
					ValueStyle.Render(room.ID),
					ValueStyle.Render(room.ModelID),
				)
				fmt.Print(LabelStyle.Render("  Confirm (y/N): "))

				var input string
				fmt.Scanln(&input)

				if input != "y" && input != "Y" && input != "yes" {
					fmt.Println(DimStyle.Render("  Cancelled."))
					return nil
				}
			}

			if err := roomSvc.Leave(context.Background(), room.ID); err != nil {
				if err == models.ErrNotInRoom {
					fmt.Println(DimStyle.Render("  Not in any room."))
					return nil
				}
				return fmt.Errorf("failed to leave room: %w", err)
			}

			fmt.Println()
			fmt.Println(SuccessBoxStyle.Render(
				HighlightStyle.Render("✓ Left the room successfully.\n\n") +
					DimStyle.Render("Your layers have been unloaded and resources freed."),
			))
			fmt.Println()

			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip confirmation prompt")

	return cmd
}

func stopCmd(roomSvc services.RoomService) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop hosting and close the room",
		Long:  "Stop the room entirely. All peers will be disconnected and the room will be closed.",
		RunE: func(cmd *cobra.Command, args []string) error {
			room := roomSvc.CurrentRoom()
			if room == nil {
				fmt.Println()
				fmt.Println(InfoBoxStyle.Render(DimStyle.Render("Not hosting any room.")))
				fmt.Println()
				return nil
			}

			if !force {
				fmt.Println()
				fmt.Printf("  %s\n",
					lipgloss.NewStyle().Foreground(ColorDanger).Bold(true).Render(
						fmt.Sprintf("⚠ This will disconnect all %d peers and close the room.", len(room.Peers)-1),
					),
				)
				fmt.Print(LabelStyle.Render("  Type 'stop' to confirm: "))

				var input string
				fmt.Scanln(&input)

				if input != "stop" {
					fmt.Println(DimStyle.Render("  Cancelled."))
					return nil
				}
			}

			if err := roomSvc.Stop(context.Background(), room.ID); err != nil {
				return fmt.Errorf("failed to stop room: %w", err)
			}

			fmt.Println()
			fmt.Println(SuccessBoxStyle.Render(
				HighlightStyle.Render("✓ Room closed.\n\n") +
					DimStyle.Render("All peers have been disconnected. Model unloaded."),
			))
			fmt.Println()

			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip confirmation prompt")

	return cmd
}
