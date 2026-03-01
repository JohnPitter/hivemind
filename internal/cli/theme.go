package cli

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// Color palette
var (
	ColorPrimary   = lipgloss.Color("#F59E0B") // Amber/honey — bee theme
	ColorSecondary = lipgloss.Color("#8B5CF6") // Purple
	ColorSuccess   = lipgloss.Color("#10B981") // Green
	ColorDanger    = lipgloss.Color("#EF4444") // Red
	ColorWarning   = lipgloss.Color("#F59E0B") // Amber
	ColorInfo      = lipgloss.Color("#3B82F6") // Blue
	ColorMuted     = lipgloss.Color("#6B7280") // Gray
	ColorBg        = lipgloss.Color("#1C1917") // Dark bg
	ColorText      = lipgloss.Color("#F5F5F4") // Light text
	ColorDim       = lipgloss.Color("#78716C") // Dim text
)

// Reusable styles
var (
	// Title / headers
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			MarginBottom(1)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)

	// Logo / branding
	LogoStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)

	// Boxes
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 2)

	InfoBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorInfo).
			Padding(1, 2)

	SuccessBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorSuccess).
			Padding(1, 2)

	ErrorBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorDanger).
			Padding(1, 2)

	WarningBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorWarning).
				Padding(1, 2)

	// Labels
	LabelStyle = lipgloss.NewStyle().
			Foreground(ColorDim)

	ValueStyle = lipgloss.NewStyle().
			Foreground(ColorText).
			Bold(true)

	// Status indicators
	OnlineStyle = lipgloss.NewStyle().
			Foreground(ColorSuccess).
			Bold(true)

	OfflineStyle = lipgloss.NewStyle().
			Foreground(ColorDanger).
			Bold(true)

	SyncingStyle = lipgloss.NewStyle().
			Foreground(ColorWarning).
			Bold(true)

	// Table
	TableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorPrimary).
				Padding(0, 1)

	TableCellStyle = lipgloss.NewStyle().
			Padding(0, 1)

	// Misc
	DimStyle = lipgloss.NewStyle().
			Foreground(ColorDim)

	BoldStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorText)

	HighlightStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)
)

// Logo returns the HiveMind ASCII art logo.
func Logo() string {
	logo := `
  ██╗  ██╗██╗██╗   ██╗███████╗
  ██║  ██║██║██║   ██║██╔════╝
  ███████║██║██║   ██║█████╗
  ██╔══██║██║╚██╗ ██╔╝██╔══╝
  ██║  ██║██║ ╚████╔╝ ███████╗
  ╚═╝  ╚═╝╚═╝  ╚═══╝  ╚══════╝
  ███╗   ███╗██╗███╗   ██╗██████╗
  ████╗ ████║██║████╗  ██║██╔══██╗
  ██╔████╔██║██║██╔██╗ ██║██║  ██║
  ██║╚██╔╝██║██║██║╚██╗██║██║  ██║
  ██║ ╚═╝ ██║██║██║ ╚████║██████╔╝
  ╚═╝     ╚═╝╚═╝╚═╝  ╚═══╝╚═════╝`
	return LogoStyle.Render(logo)
}

// FormatVRAM formats VRAM in MB to a human-readable string (MB or GB).
func FormatVRAM(mb int64) string {
	if mb >= 1024 {
		gb := float64(mb) / 1024.0
		if gb == float64(int64(gb)) {
			return fmt.Sprintf("~%dGB", int64(gb))
		}
		return fmt.Sprintf("~%.1fGB", gb)
	}
	return fmt.Sprintf("~%dMB", mb)
}

// StatusIndicator returns a colored status dot.
func StatusIndicator(state string) string {
	switch state {
	case "ready", "active", "online":
		return OnlineStyle.Render("● online")
	case "connecting", "syncing", "loading":
		return SyncingStyle.Render("◌ syncing")
	case "offline", "closed":
		return OfflineStyle.Render("○ offline")
	default:
		return DimStyle.Render("? unknown")
	}
}
