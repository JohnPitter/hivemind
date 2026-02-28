package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/joaopedro/hivemind/internal/models"
	"github.com/joaopedro/hivemind/internal/services"
	"github.com/spf13/cobra"
)

func chatCmd(infSvc services.InferenceService, roomSvc services.RoomService) *cobra.Command {
	return &cobra.Command{
		Use:   "chat",
		Short: "Start an interactive chat session",
		Long:  "Chat with the AI model running in the room. Responses are generated via distributed inference across all peers.",
		RunE: func(cmd *cobra.Command, args []string) error {
			room := roomSvc.CurrentRoom()
			if room == nil {
				fmt.Println()
				fmt.Println(ErrorBoxStyle.Render("Not in any room. Run 'hivemind create' or 'hivemind join' first."))
				fmt.Println()
				return nil
			}

			fmt.Println()
			fmt.Println(TitleStyle.Render("🐝 HiveMind Chat"))
			fmt.Printf("  %s %s   %s %d peers\n",
				LabelStyle.Render("Model:"), ValueStyle.Render(room.ModelID),
				LabelStyle.Render("Network:"), len(room.Peers),
			)
			fmt.Println()
			fmt.Println(DimStyle.Render("  Type your message and press Enter. Type /quit to exit."))
			fmt.Println(DimStyle.Render("  " + strings.Repeat("─", 60)))
			fmt.Println()

			var history []models.ChatMessage
			scanner := bufio.NewScanner(os.Stdin)

			for {
				fmt.Print(HighlightStyle.Render("  you › "))

				if !scanner.Scan() {
					break
				}

				input := strings.TrimSpace(scanner.Text())
				if input == "" {
					continue
				}

				if input == "/quit" || input == "/exit" || input == "/q" {
					fmt.Println()
					fmt.Println(DimStyle.Render("  Goodbye! 👋"))
					fmt.Println()
					break
				}

				if input == "/clear" {
					history = nil
					fmt.Println(DimStyle.Render("  History cleared."))
					fmt.Println()
					continue
				}

				if input == "/help" {
					fmt.Println()
					fmt.Println(DimStyle.Render("  /quit   — exit chat"))
					fmt.Println(DimStyle.Render("  /clear  — clear history"))
					fmt.Println(DimStyle.Render("  /help   — show this help"))
					fmt.Println()
					continue
				}

				history = append(history, models.ChatMessage{
					Role:    "user",
					Content: input,
				})

				req := models.ChatRequest{
					Model:       room.ModelID,
					Messages:    history,
					Temperature: 0.7,
					MaxTokens:   2048,
					Stream:      true,
				}

				fmt.Print(OnlineStyle.Render("  ai  › "))

				// Stream response
				ch := make(chan models.ChatChunk, 100)
				errCh := make(chan error, 1)

				go func() {
					errCh <- infSvc.ChatCompletionStream(context.Background(), req, ch)
				}()

				var fullResponse strings.Builder
				for chunk := range ch {
					for _, choice := range chunk.Choices {
						if choice.Delta.Content != "" {
							fmt.Print(choice.Delta.Content)
							fullResponse.WriteString(choice.Delta.Content)
						}
					}
				}

				if err := <-errCh; err != nil {
					fmt.Println()
					fmt.Println(ErrorBoxStyle.Render(fmt.Sprintf("Error: %v", err)))
					continue
				}

				fmt.Println()
				fmt.Println()

				history = append(history, models.ChatMessage{
					Role:    "assistant",
					Content: fullResponse.String(),
				})
			}

			return nil
		},
	}
}
