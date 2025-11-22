package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulpeslab/vulpix/internal/config"
	"github.com/vulpeslab/vulpix/pkg/core"
)

type authState int

const (
	authSelectProvider authState = iota
	authInputKey
	authSuccess
)

type AuthModel struct {
	state     authState
	providers []string
	cursor    int
	textInput textinput.Model
	selected  string
	config    *core.Config
	err       error
}

func NewAuthModel(cfg *core.Config) AuthModel {
	ti := textinput.New()
	ti.Placeholder = "Enter API Key"
	ti.EchoMode = textinput.EchoPassword
	ti.CharLimit = 200
	ti.Width = 50

	// Get providers from config defaults or hardcoded list of supported ones
	// We want to show all supported providers, even if not currently in the user's config map
	supported := []string{"openai", "anthropic", "openrouter", "chutes", "nahcrof", "nebius"}

	return AuthModel{
		state:     authSelectProvider,
		providers: supported,
		textInput: ti,
		config:    cfg,
	}
}

func (m AuthModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m AuthModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		}

		switch m.state {
		case authSelectProvider:
			switch msg.String() {
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.providers)-1 {
					m.cursor++
				}
			case "enter":
				m.selected = m.providers[m.cursor]
				m.state = authInputKey
				m.textInput.Focus()
				return m, nil
			}

		case authInputKey:
			switch msg.Type {
			case tea.KeyEnter:
				apiKey := m.textInput.Value()
				if apiKey != "" {
					// Save config
					if m.config.Providers == nil {
						m.config.Providers = make(map[string]core.ProviderConfig)
					}

					// Get existing config to preserve other fields like BaseURL
					provConfig := m.config.Providers[m.selected]
					provConfig.APIKey = apiKey

					// If BaseURL is missing, set default (though loader handles this usually, explicit is good)
					if provConfig.BaseURL == "" {
						defaults := config.DefaultConfig()
						if def, ok := defaults.Providers[m.selected]; ok {
							provConfig.BaseURL = def.BaseURL
						}
					}

					m.config.Providers[m.selected] = provConfig

					if err := config.SaveConfig(m.config); err != nil {
						m.err = err
					} else {
						m.state = authSuccess
						return m, tea.Quit
					}
				}
			}
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m AuthModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\nPress Esc to quit.", m.err)
	}

	var s strings.Builder

	switch m.state {
	case authSelectProvider:
		s.WriteString("Select a provider to authenticate:\n\n")
		for i, p := range m.providers {
			cursor := " "
			if m.cursor == i {
				cursor = ">"
			}
			s.WriteString(fmt.Sprintf("%s %s\n", cursor, p))
		}
		s.WriteString("\n(Press Enter to select, Esc to quit)")

	case authInputKey:
		s.WriteString(fmt.Sprintf("Enter API Key for %s:\n\n", m.selected))
		s.WriteString(m.textInput.View())
		s.WriteString("\n\n(Press Enter to save, Esc to quit)")

	case authSuccess:
		s.WriteString(fmt.Sprintf("Successfully saved API key for %s!\n", m.selected))
	}

	return lipgloss.NewStyle().Margin(1, 2).Render(s.String())
}
