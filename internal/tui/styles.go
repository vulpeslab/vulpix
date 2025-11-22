package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	primaryColor   = lipgloss.Color("#7D56F4") // Vulpix Purple
	secondaryColor = lipgloss.Color("#F4A261") // Orange
	errorColor     = lipgloss.Color("#E76F51") // Red
	successColor   = lipgloss.Color("#2A9D8F") // Green
	subtleColor    = lipgloss.Color("#6C757D") // Gray

	// Styles
	TitleStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true).
			Padding(0, 1)

	UserMessageStyle = lipgloss.NewStyle().
				Foreground(secondaryColor).
				Bold(true).
				SetString("> ")

	AssistantMessageStyle = lipgloss.NewStyle().
				Foreground(primaryColor).
				Bold(true).
				SetString("Vulpix: ")

	ToolExecStyle = lipgloss.NewStyle().
			Foreground(subtleColor).
			Italic(true)

	ToolResultStyle = lipgloss.NewStyle().
			Foreground(subtleColor)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true)

	ApprovalPromptStyle = lipgloss.NewStyle().
				Background(errorColor).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true).
				Padding(0, 1)

	ReasoningStyle = lipgloss.NewStyle().
			Foreground(subtleColor).
			Italic(true)

	StatusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 2)
)
