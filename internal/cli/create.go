package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/joaopedro/hivemind/internal/models"
	"github.com/joaopedro/hivemind/internal/services"
	"github.com/spf13/cobra"
)

// Popular models for the selection menu.
var popularModels = []struct {
	ID       string
	Name     string
	Size     string
	Type     models.ModelType
	MinVRAM  string
}{
	{"meta-llama/Llama-3-70B", "Llama 3 70B", "70B params", models.ModelTypeLLM, "~40GB total"},
	{"meta-llama/Llama-3-8B", "Llama 3 8B", "8B params", models.ModelTypeLLM, "~6GB total"},
	{"mistralai/Mixtral-8x7B", "Mixtral 8x7B", "47B params", models.ModelTypeLLM, "~28GB total"},
	{"google/gemma-2-27b", "Gemma 2 27B", "27B params", models.ModelTypeLLM, "~18GB total"},
	{"stabilityai/stable-diffusion-xl", "Stable Diffusion XL", "3.5B params", models.ModelTypeDiffusion, "~8GB total"},
	{"black-forest-labs/FLUX.1-dev", "FLUX.1 Dev", "12B params", models.ModelTypeDiffusion, "~12GB total"},
}

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

			// Determine model type
			modelType := models.ModelTypeLLM
			for _, m := range popularModels {
				if m.ID == modelID {
					modelType = m.Type
					break
				}
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

			return nil
		},
	}

	cmd.Flags().StringVar(&modelID, "model", "", "model ID (e.g., meta-llama/Llama-3-70B)")
	cmd.Flags().IntVar(&maxPeers, "max-peers", 5, "maximum number of peers")

	return cmd
}

func interactiveModelSelect() string {
	fmt.Println(BoldStyle.Render("Select a model:"))
	fmt.Println()

	for i, m := range popularModels {
		typeTag := lipgloss.NewStyle().
			Foreground(ColorInfo).
			Render(strings.ToUpper(string(m.Type)))

		num := lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Render(fmt.Sprintf("  [%d]", i+1))

		name := BoldStyle.Render(m.Name)
		size := DimStyle.Render(fmt.Sprintf("(%s, %s)", m.Size, m.MinVRAM))

		fmt.Printf("%s %s %s %s\n", num, typeTag, name, size)
	}

	fmt.Println()
	customNum := len(popularModels) + 1
	fmt.Printf("  %s %s\n",
		HighlightStyle.Render(fmt.Sprintf("[%d]", customNum)),
		DimStyle.Render("Enter custom model ID"),
	)

	fmt.Println()
	fmt.Print(LabelStyle.Render("  Choose (1-"+strconv.Itoa(customNum)+"): "))

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

	return popularModels[choice-1].ID
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
