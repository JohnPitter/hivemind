package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/joaopedro/hivemind/internal/models"
	"github.com/joaopedro/hivemind/internal/services"
	"github.com/spf13/cobra"
)

func statusCmd(roomSvc services.RoomService) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show room status and peer information",
		Long:  "Display real-time information about the current room, connected peers, layer assignments, and resource usage.",
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := roomSvc.Status(context.Background())
			if err != nil {
				if err == models.ErrNotInRoom {
					fmt.Println()
					fmt.Println(InfoBoxStyle.Render(
						DimStyle.Render("Not in any room.\n\n") +
							"Run " + HighlightStyle.Render("hivemind create") + " to create a room\n" +
							"Run " + HighlightStyle.Render("hivemind join <code>") + " to join a room",
					))
					fmt.Println()
					return nil
				}
				return fmt.Errorf("failed to get status: %w", err)
			}

			fmt.Println()
			renderStatus(status)
			return nil
		},
	}
}

func renderStatus(status *models.RoomStatus) {
	room := status.Room

	// Room header
	header := fmt.Sprintf(
		"%s  %s\n\n"+
			"%s %s   %s %s   %s %s\n"+
			"%s %d/%d        %s %d        %s %.1f tok/s",
		TitleStyle.Render("🐝 "+room.ModelID),
		StatusIndicator(string(room.State)),
		LabelStyle.Render("Room:"), ValueStyle.Render(room.ID),
		LabelStyle.Render("Type:"), ValueStyle.Render(string(room.ModelType)),
		LabelStyle.Render("Uptime:"), ValueStyle.Render(status.Uptime),
		LabelStyle.Render("Peers:"), len(room.Peers), room.MaxPeers,
		LabelStyle.Render("Layers:"), room.TotalLayers,
		LabelStyle.Render("Speed:"), status.TokensPerSec,
	)

	fmt.Println(BoxStyle.Render(header))
	fmt.Println()

	// VRAM bar
	renderVRAMBar(status.UsedVRAM, status.TotalVRAM)
	fmt.Println()

	// Peers table
	renderPeersTable(room.Peers)
	fmt.Println()

	// Layer map
	renderLayerMap(room.Peers, room.TotalLayers)
	fmt.Println()
}

func renderVRAMBar(used, total int64) {
	if total == 0 {
		return
	}

	barWidth := 40
	filledWidth := int(float64(used) / float64(total) * float64(barWidth))
	if filledWidth > barWidth {
		filledWidth = barWidth
	}

	filled := lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Render(strings.Repeat("█", filledWidth))
	empty := DimStyle.Render(strings.Repeat("░", barWidth-filledWidth))

	pct := float64(used) / float64(total) * 100

	fmt.Printf("  %s  %s%s  %s\n",
		LabelStyle.Render("VRAM"),
		filled, empty,
		ValueStyle.Render(fmt.Sprintf("%.0f%% (%dMB / %dMB)", pct, used, total)),
	)
}

func renderPeersTable(peers []models.Peer) {
	fmt.Println(BoldStyle.Render("  Peers"))
	fmt.Println()

	// Header
	fmt.Printf("  %s %s %s %s %s %s %s\n",
		TableHeaderStyle.Width(12).Render("NAME"),
		TableHeaderStyle.Width(12).Render("IP"),
		TableHeaderStyle.Width(10).Render("STATUS"),
		TableHeaderStyle.Width(18).Render("GPU"),
		TableHeaderStyle.Width(10).Render("VRAM"),
		TableHeaderStyle.Width(12).Render("LAYERS"),
		TableHeaderStyle.Width(10).Render("LATENCY"),
	)

	divider := DimStyle.Render("  " + strings.Repeat("─", 84))
	fmt.Println(divider)

	for _, p := range peers {
		name := p.Name
		if p.IsHost {
			name += " ★"
		}

		layerStr := "—"
		if len(p.Layers) > 0 {
			layerStr = fmt.Sprintf("%d-%d", p.Layers[0], p.Layers[len(p.Layers)-1])
		}

		vramStr := fmt.Sprintf("%dGB", p.Resources.VRAMTotal/1024)

		latencyStr := "—"
		if p.Latency > 0 {
			latencyStr = fmt.Sprintf("%.0fms", p.Latency)
		}

		fmt.Printf("  %s %s %s %s %s %s %s\n",
			TableCellStyle.Width(12).Render(name),
			TableCellStyle.Width(12).Render(p.IP),
			TableCellStyle.Width(10).Render(StatusIndicator(string(p.State))),
			TableCellStyle.Width(18).Render(truncate(p.Resources.GPUName, 16)),
			TableCellStyle.Width(10).Render(vramStr),
			TableCellStyle.Width(12).Render(layerStr),
			TableCellStyle.Width(10).Render(latencyStr),
		)
	}
}

func renderLayerMap(peers []models.Peer, totalLayers int) {
	if totalLayers == 0 {
		return
	}

	fmt.Println(BoldStyle.Render("  Layer Distribution"))
	fmt.Println()

	mapWidth := 60
	for _, p := range peers {
		if len(p.Layers) == 0 {
			continue
		}

		startPct := float64(p.Layers[0]) / float64(totalLayers)
		endPct := float64(p.Layers[len(p.Layers)-1]+1) / float64(totalLayers)
		barStart := int(startPct * float64(mapWidth))
		barEnd := int(endPct * float64(mapWidth))

		if barEnd <= barStart {
			barEnd = barStart + 1
		}

		bar := strings.Repeat(" ", barStart)

		color := ColorPrimary
		if p.IsHost {
			color = ColorSuccess
		}

		bar += lipgloss.NewStyle().
			Foreground(color).
			Render(strings.Repeat("█", barEnd-barStart))

		bar += strings.Repeat(" ", mapWidth-barEnd)

		name := p.Name
		if len(name) > 10 {
			name = name[:10]
		}

		fmt.Printf("  %-12s [%s] L%d-%d\n",
			name, bar, p.Layers[0], p.Layers[len(p.Layers)-1])
	}

	// Scale
	fmt.Printf("  %-12s [%-*s]\n", "", mapWidth,
		DimStyle.Render(fmt.Sprintf("0%s%d", strings.Repeat(" ", mapWidth-4), totalLayers)))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
