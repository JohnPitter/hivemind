package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/joaopedro/hivemind/internal/catalog"
	"github.com/joaopedro/hivemind/internal/models"
	"github.com/joaopedro/hivemind/internal/services"
	"github.com/spf13/cobra"
)

func createCmd(roomSvc services.RoomService) *cobra.Command {
	var modelID string
	var maxPeers int

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new room and become the host",
		Long:  "Start a new HiveMind room. Select a model, configure settings, and get an invite code to share with peers.",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println()
			fmt.Println(TitleStyle.Render("🐝 Create a new HiveMind room"))
			fmt.Println()

			// Model selection
			if modelID == "" {
				modelID = interactiveModelSelect()
			}

			if modelID == "" {
				return fmt.Errorf("no model selected")
			}

			// Determine model type from catalog
			modelType := models.ModelTypeLLM
			if m := catalog.Lookup(modelID); m != nil {
				modelType = m.Type
			}

			// Max peers
			if maxPeers == 0 {
				maxPeers = 5
			}

			fmt.Println()
			fmt.Println(DimStyle.Render("Creating room..."))

			room, err := roomSvc.Create(context.Background(), models.RoomConfig{
				ModelID:     modelID,
				ModelType:   modelType,
				MaxPeers:    maxPeers,
				AutoApprove: true,
			})
			if err != nil {
				return fmt.Errorf("failed to create room: %w", err)
			}

			// Display room info
			renderRoomCreated(room)

			// Warn if room is pending due to insufficient resources
			if room.State == models.RoomStatePending {
				renderPendingWarning(room)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&modelID, "model", "", "model ID (e.g., meta-llama/Llama-3-70B)")
	cmd.Flags().IntVar(&maxPeers, "max-peers", 5, "maximum number of peers")

	return cmd
}

// modelTypeOrder defines the display order for model type groups.
var modelTypeOrder = []models.ModelType{
	models.ModelTypeLLM,
	models.ModelTypeCode,
	models.ModelTypeDiffusion,
	models.ModelTypeMultimodal,
	models.ModelTypeEmbedding,
}

// modelTypeColors maps each model type to a display color.
var modelTypeColors = map[models.ModelType]lipgloss.Color{
	models.ModelTypeLLM:        ColorInfo,
	models.ModelTypeCode:       ColorSuccess,
	models.ModelTypeDiffusion:  ColorSecondary,
	models.ModelTypeMultimodal: ColorWarning,
	models.ModelTypeEmbedding:  ColorMuted,
}

func interactiveModelSelect() string {
	allModels := catalog.All()

	// Group models by type preserving catalog order within each group.
	groups := make(map[models.ModelType][]catalog.ModelRequirements)
	for _, m := range allModels {
		groups[m.Type] = append(groups[m.Type], m)
	}

	fmt.Println(BoldStyle.Render("Select a model:"))
	fmt.Println()

	// Flat index for user selection.
	var indexed []catalog.ModelRequirements
	num := 1

	for _, mt := range modelTypeOrder {
		modelsInGroup := groups[mt]
		if len(modelsInGroup) == 0 {
			continue
		}

		color, ok := modelTypeColors[mt]
		if !ok {
			color = ColorInfo
		}

		header := lipgloss.NewStyle().
			Bold(true).
			Foreground(color).
			Render("  ── " + strings.ToUpper(string(mt)) + " ──")
		fmt.Println(header)

		for _, m := range modelsInGroup {
			tag := lipgloss.NewStyle().
				Foreground(color).
				Render(strings.ToUpper(string(m.Type)))

			numStr := lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Bold(true).
				Render(fmt.Sprintf("  [%d]", num))

			name := BoldStyle.Render(m.Name)
			vramStr := FormatVRAM(m.MinVRAMMB)
			size := DimStyle.Render(fmt.Sprintf("(%s, %s)", m.ParameterSize, vramStr))

			fmt.Printf("%s %s %s %s\n", numStr, tag, name, size)
			indexed = append(indexed, m)
			num++
		}
		fmt.Println()
	}

	customNum := len(indexed) + 1
	fmt.Printf("  %s %s\n",
		HighlightStyle.Render(fmt.Sprintf("[%d]", customNum)),
		DimStyle.Render("Enter custom model ID"),
	)

	fmt.Println()
	fmt.Print(LabelStyle.Render("  Choose (1-" + strconv.Itoa(customNum) + "): "))

	var input string
	fmt.Scanln(&input)

	choice, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || choice < 1 || choice > customNum {
		fmt.Println(ErrorBoxStyle.Render("Invalid choice"))
		return ""
	}

	if choice == customNum {
		fmt.Print(LabelStyle.Render("  Model ID: "))
		fmt.Scanln(&input)
		return strings.TrimSpace(input)
	}

	return indexed[choice-1].ID
}

func renderRoomCreated(room *models.Room) {
	// Invite code box
	inviteBox := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#000000")).
		Background(ColorPrimary).
		Padding(0, 2).
		Render(room.InviteCode)

	content := fmt.Sprintf(
		"%s\n\n"+
			"%s %s\n"+
			"%s %s\n"+
			"%s %s\n"+
			"%s %d\n"+
			"%s %d\n\n"+
			"%s\n\n"+
			"  %s\n\n"+
			"%s",
		HighlightStyle.Render("✓ Room created successfully!"),
		LabelStyle.Render("Room ID:    "), ValueStyle.Render(room.ID),
		LabelStyle.Render("Model:      "), ValueStyle.Render(room.ModelID),
		LabelStyle.Render("Type:       "), ValueStyle.Render(string(room.ModelType)),
		LabelStyle.Render("Max Peers:  "), room.MaxPeers,
		LabelStyle.Render("Layers:     "), room.TotalLayers,
		LabelStyle.Render("Invite Code:"),
		inviteBox,
		DimStyle.Render("Share this code with peers: hivemind join "+room.InviteCode),
	)

	fmt.Println()
	fmt.Println(SuccessBoxStyle.Render(content))
	fmt.Println()
}

func renderPendingWarning(room *models.Room) {
	modelReqs := catalog.Lookup(room.ModelID)
	if modelReqs == nil {
		return
	}

	var hostVRAM int64
	for _, p := range room.Peers {
		hostVRAM += p.Resources.TotalUsableVRAM()
	}

	deficit := modelReqs.MinVRAMMB - hostVRAM
	deficitStr := FormatVRAM(deficit)
	requiredStr := FormatVRAM(modelReqs.MinVRAMMB)
	availStr := FormatVRAM(hostVRAM)

	warning := fmt.Sprintf(
		"%s\n\n"+
			"%s %s requires %s VRAM but you only have %s available.\n"+
			"%s Waiting 5 minutes for peers to contribute %s.\n",
		HighlightStyle.Render("⚠ Insufficient resources — room is PENDING"),
		LabelStyle.Render("Model"), ValueStyle.Render(modelReqs.Name),
		requiredStr, availStr,
		LabelStyle.Render("Status:"), deficitStr,
	)

	if suggested := catalog.SuggestLargestFitting(hostVRAM); suggested != nil && suggested.ID != room.ModelID {
		warning += fmt.Sprintf(
			"%s You could run %s (%s) with your current resources.\n",
			LabelStyle.Render("Suggestion:"),
			ValueStyle.Render(suggested.Name),
			FormatVRAM(suggested.MinVRAMMB),
		)
	}

	fmt.Println(WarningBoxStyle.Render(warning))
	fmt.Println()
}
