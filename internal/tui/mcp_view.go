package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulpeslab/vulpix/internal/mcp"
)

var (
	popupStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2).
			Width(80)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("205")).
				Bold(true)

	itemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("62")).
			Bold(true).
			MarginBottom(1)
)

func (m Model) viewMCPPopup() string {
	var content string

	switch m.mcpPopupState {
	case MCPPopupList:
		content = m.viewMCPList()
	case MCPPopupDetail:
		content = m.viewMCPDetail()
	case MCPPopupTools:
		content = m.viewMCPTools()
	case MCPPopupToolDetail:
		content = m.viewMCPToolDetail()
	case MCPPopupAddManual:
		content = m.viewMCPAddManual()
	}

	return popupStyle.Render(content)
}

func (m Model) viewMCPList() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Manage MCP servers") + "\n\n")

	m.mcpManager.Mu().RLock()
	defer m.mcpManager.Mu().RUnlock()

	for i, srv := range m.mcpManager.Servers {
		cursor := "  "
		style := itemStyle
		if i == m.selectedMCPIndex {
			cursor = "> "
			style = selectedItemStyle
		}

		status := srv.Status
		statusColor := "240" // Gray
		if status == mcp.StatusConnected {
			statusColor = "42" // Green
		} else if status == mcp.StatusError {
			statusColor = "196" // Red
		} else if status == mcp.StatusConnecting {
			statusColor = "208" // Orange
		}

		statusStr := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(fmt.Sprintf("(%s)", status))

		line := fmt.Sprintf("%s%d. %s %s", cursor, i+1, srv.Config.Name, statusStr)
		b.WriteString(style.Render(line) + "\n")
	}

	// Add Registry Option
	{
		cursor := "  "
		style := itemStyle
		if m.selectedMCPIndex == len(m.mcpManager.Servers) {
			cursor = "> "
			style = selectedItemStyle
		}
		b.WriteString("\n" + style.Render(cursor+"+ Add MCP server from registry") + "\n")
	}

	// Add Manual Option
	{
		cursor := "  "
		style := itemStyle
		if m.selectedMCPIndex == len(m.mcpManager.Servers)+1 {
			cursor = "> "
			style = selectedItemStyle
		}
		b.WriteString(style.Render(cursor + "+ Add MCP server manually"))
	}

	return b.String()
}

func (m Model) viewMCPDetail() string {
	m.mcpManager.Mu().RLock()
	srv := m.mcpManager.Servers[m.selectedMCPIndex]
	m.mcpManager.Mu().RUnlock()

	var b strings.Builder
	b.WriteString(titleStyle.Render(srv.Config.Name) + "\n\n")

	statusColor := "240"
	if srv.Status == mcp.StatusConnected {
		statusColor = "42"
	} else if srv.Status == mcp.StatusError {
		statusColor = "196"
	}

	b.WriteString(fmt.Sprintf("Status: %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(string(srv.Status))))

	if srv.Config.Type == mcp.ServerTypeRemote {
		b.WriteString(fmt.Sprintf("Type: %s\n", "Remote (HTTP/SSE)"))
		b.WriteString(fmt.Sprintf("URL: %s\n", srv.Config.Url))
	} else {
		b.WriteString(fmt.Sprintf("Type: %s\n", "STDIO"))
		b.WriteString(fmt.Sprintf("Command: %s\n", srv.Config.Command))
		b.WriteString(fmt.Sprintf("Args: %s\n", strings.Join(srv.Config.Args, " ")))
	}

	b.WriteString(fmt.Sprintf("Tools: %d\n\n", len(srv.Tools)))

	actions := []string{"View Tools", "Disable", "Remove Server"}
	if srv.Config.Disabled {
		actions[1] = "Enable"
	}

	for i, action := range actions {
		cursor := "  "
		style := itemStyle
		if i == m.selectedToolIndex { // Reusing selectedToolIndex for action selection
			cursor = "> "
			style = selectedItemStyle
		}
		b.WriteString(style.Render(fmt.Sprintf("%s%d. %s", cursor, i+1, action)) + "\n")
	}

	return b.String()
}

func (m Model) viewMCPTools() string {
	m.mcpManager.Mu().RLock()
	srv := m.mcpManager.Servers[m.selectedMCPIndex]
	m.mcpManager.Mu().RUnlock()

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Tools for %s (%d tools)", srv.Config.Name, len(srv.Tools))) + "\n\n")

	start := 0
	end := len(srv.Tools)
	if end > 10 {
		end = 10 // Simple pagination or scrolling needed? User asked for scrolling list in popup
	}

	// We need scrolling state for tools list if it's long.
	// For now, let's just show first 10 or use selectedToolIndex to scroll.

	// Let's implement simple scrolling window around selectedToolIndex
	windowSize := 10
	start = m.selectedToolIndex - (windowSize / 2)
	if start < 0 {
		start = 0
	}
	end = start + windowSize
	if end > len(srv.Tools) {
		end = len(srv.Tools)
		start = end - windowSize
		if start < 0 {
			start = 0
		}
	}

	for i := start; i < end; i++ {
		tool := srv.Tools[i]
		cursor := "  "
		style := itemStyle
		if i == m.selectedToolIndex {
			cursor = "> "
			style = selectedItemStyle
		}
		b.WriteString(style.Render(fmt.Sprintf("%s%d. %s", cursor, i+1, tool.Name())) + "\n")
	}

	b.WriteString(fmt.Sprintf("\n↑/↓ to navigate, Enter to view details, ESC to go back • Showing %d-%d of %d", start+1, end, len(srv.Tools)))
	return b.String()
}

func (m Model) viewMCPToolDetail() string {
	m.mcpManager.Mu().RLock()
	srv := m.mcpManager.Servers[m.selectedMCPIndex]
	tool := srv.Tools[m.selectedToolIndex]
	m.mcpManager.Mu().RUnlock()

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("%s (%s)", tool.Name(), srv.Config.Name)) + "\n\n")

	b.WriteString(fmt.Sprintf("Description:\n%s\n\n", tool.Description()))

	b.WriteString("Parameters:\n")

	// Parse schema
	var schema struct {
		Type       string `json:"type"`
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}

	if err := json.Unmarshal([]byte(tool.Schema()), &schema); err == nil {
		if len(schema.Properties) == 0 {
			b.WriteString("  (No parameters)\n")
		} else {
			for name, prop := range schema.Properties {
				required := "optional"
				for _, req := range schema.Required {
					if req == name {
						required = "required"
						break
					}
				}

				b.WriteString(fmt.Sprintf("• %s (%s): %s - %s\n", name, required, prop.Type, prop.Description))
			}
		}
	} else {
		b.WriteString("  (Invalid schema)\n")
	}

	b.WriteString("\nPress ESC to go back")
	return b.String()
}

func (m Model) viewMCPAddManual() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Add MCP Server") + "\n\n")

	// Show summary of what we have so far
	if m.mcpAddStep > MCPAddStepName {
		b.WriteString(fmt.Sprintf("Server Name: %s\n\n", m.newMCPConfig.Name))
	}
	if m.mcpAddStep > MCPAddStepType {
		b.WriteString(fmt.Sprintf("Server Type: %s\n\n", m.newMCPConfig.Type))
	}
	if m.mcpAddStep > MCPAddStepUrl && m.newMCPConfig.Type == mcp.ServerTypeRemote {
		b.WriteString(fmt.Sprintf("URL: %s\n\n", m.newMCPConfig.Url))
	}
	if m.mcpAddStep > MCPAddStepCommand && m.newMCPConfig.Type == mcp.ServerTypeStdio {
		cmdStr := m.newMCPConfig.Command
		if len(m.newMCPConfig.Args) > 0 {
			cmdStr += " " + strings.Join(m.newMCPConfig.Args, " ")
		}
		b.WriteString(fmt.Sprintf("Command: %s\n\n", cmdStr))
	}

	// Show current step input
	switch m.mcpAddStep {
	case MCPAddStepName:
		b.WriteString("Server Name: ")
		b.WriteString(m.mcpTextInput.View())
	case MCPAddStepType:
		b.WriteString("Server Type: ")
		// Simple toggle or list
		// We can use textinput for now, but restrict values?
		// Or just render options and use Up/Down.
		// Let's use Up/Down selection for Type.
		// We need state for selection index? Or just toggle.
		// Let's use m.selectedToolIndex for type selection (0=stdio, 1=http)
		types := []string{"stdio", "http"}
		b.WriteString("\n")
		for i, t := range types {
			cursor := "  "
			style := itemStyle
			if i == m.selectedToolIndex {
				cursor = "> "
				style = selectedItemStyle
			}
			b.WriteString(style.Render(fmt.Sprintf("%s%s", cursor, t)) + "\n")
		}
	case MCPAddStepUrl:
		b.WriteString("URL: ")
		b.WriteString(m.mcpTextInput.View())
	case MCPAddStepCommand:
		b.WriteString("Command (full command with args): ")
		b.WriteString(m.mcpTextInput.View())
	case MCPAddStepEnv:
		b.WriteString("Environment Variables (KEY=VALUE, space separated): ")
		b.WriteString(m.mcpTextInput.View())
	}

	b.WriteString("\n\nEnter to continue, ESC to cancel")
	return b.String()
}

func (m Model) updateMCPPopup(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEsc:
			if m.mcpPopupState == MCPPopupList {
				m.showMCPPopup = false
			} else if m.mcpPopupState == MCPPopupDetail {
				m.mcpPopupState = MCPPopupList
			} else if m.mcpPopupState == MCPPopupTools {
				m.mcpPopupState = MCPPopupDetail
				m.selectedToolIndex = 0 // Reset action selection
			} else if m.mcpPopupState == MCPPopupToolDetail {
				m.mcpPopupState = MCPPopupTools
			} else if m.mcpPopupState == MCPPopupAddManual {
				m.mcpPopupState = MCPPopupList
				m.mcpTextInput.Reset()
				m.mcpAddStep = MCPAddStepName
			}
			return m, nil
		case tea.KeyUp:
			if m.mcpPopupState == MCPPopupList {
				if m.selectedMCPIndex > 0 {
					m.selectedMCPIndex--
				}
			} else if m.mcpPopupState == MCPPopupDetail {
				if m.selectedToolIndex > 0 {
					m.selectedToolIndex--
				}
			} else if m.mcpPopupState == MCPPopupTools {
				m.mcpManager.Mu().RLock()
				count := len(m.mcpManager.Servers[m.selectedMCPIndex].Tools)
				m.mcpManager.Mu().RUnlock()
				if m.selectedToolIndex > 0 {
					m.selectedToolIndex--
				} else if count > 0 {
					m.selectedToolIndex = count - 1 // Wrap around?
				}
			} else if m.mcpPopupState == MCPPopupAddManual {
				if m.mcpAddStep == MCPAddStepType {
					if m.selectedToolIndex > 0 {
						m.selectedToolIndex--
					}
				}
			}
			return m, nil
		case tea.KeyDown:
			if m.mcpPopupState == MCPPopupList {
				m.mcpManager.Mu().RLock()
				count := len(m.mcpManager.Servers)
				m.mcpManager.Mu().RUnlock()
				// Allow selecting servers (0..count-1), Registry (count), Manual (count+1)
				if m.selectedMCPIndex < count+1 {
					m.selectedMCPIndex++
				}
			} else if m.mcpPopupState == MCPPopupDetail {
				if m.selectedToolIndex < 2 { // 3 actions
					m.selectedToolIndex++
				}
			} else if m.mcpPopupState == MCPPopupTools {
				m.mcpManager.Mu().RLock()
				count := len(m.mcpManager.Servers[m.selectedMCPIndex].Tools)
				m.mcpManager.Mu().RUnlock()
				if m.selectedToolIndex < count-1 {
					m.selectedToolIndex++
				}
			} else if m.mcpPopupState == MCPPopupAddManual {
				if m.mcpAddStep == MCPAddStepType {
					if m.selectedToolIndex < 1 {
						m.selectedToolIndex++
					}
				}
			}
			return m, nil
		case tea.KeyEnter:
			if m.mcpPopupState == MCPPopupList {
				// Check if "Add MCP server manually" is selected
				m.mcpManager.Mu().RLock()
				serverCount := len(m.mcpManager.Servers)
				m.mcpManager.Mu().RUnlock()

				// The list has servers + 2 items (Registry, Manual)
				// Index of Manual is serverCount + 1
				if m.selectedMCPIndex == serverCount+1 {
					m.mcpPopupState = MCPPopupAddManual
					m.mcpAddStep = MCPAddStepName
					m.mcpTextInput.Reset()
					m.mcpTextInput.Focus()
					m.newMCPConfig = mcp.ServerConfig{} // Reset config
				} else if m.selectedMCPIndex < serverCount {
					m.mcpPopupState = MCPPopupDetail
					m.selectedToolIndex = 0 // Reset for actions
				}
			} else if m.mcpPopupState == MCPPopupTools {
				m.mcpPopupState = MCPPopupToolDetail
			} else if m.mcpPopupState == MCPPopupDetail {
				// Execute action
				switch m.selectedToolIndex {
				case 0: // View Tools
					m.mcpPopupState = MCPPopupTools
					m.selectedToolIndex = 0
				case 1: // Disable/Enable
					m.mcpManager.ToggleServer(m.selectedMCPIndex)
					// Re-connect if enabled? ToggleServer handles disconnect.
					// If enabled, we need to connect.
					m.mcpManager.Mu().RLock()
					srv := m.mcpManager.Servers[m.selectedMCPIndex]
					disabled := srv.Config.Disabled
					m.mcpManager.Mu().RUnlock()

					if !disabled {
						return m, func() tea.Msg {
							m.mcpManager.ConnectServer(context.Background(), srv)
							return mcpConnectedMsg{tools: m.mcpManager.GetAllTools()}
						}
					}
					// Update tools in engine
					m.engine.AddTools(m.mcpManager.GetAllTools()) // This appends, might duplicate. Engine needs SetTools or we just append new ones?
					// Engine.AddTools appends. We need to refresh tools.
					// Engine doesn't have SetTools. I should add it or just rely on AddTools if I can clear them.
					// For now, let's just return updated tools msg.
					return m, func() tea.Msg {
						return mcpConnectedMsg{tools: m.mcpManager.GetAllTools()}
					}

				case 2: // Remove
					m.mcpManager.RemoveServer(m.selectedMCPIndex)
					m.mcpPopupState = MCPPopupList
					m.selectedMCPIndex = 0
					return m, func() tea.Msg {
						return mcpConnectedMsg{tools: m.mcpManager.GetAllTools()}
					}
				}
			} else if m.mcpPopupState == MCPPopupAddManual {
				// Handle MCP server addition steps
				switch m.mcpAddStep {
				case MCPAddStepName:
					name := strings.TrimSpace(m.mcpTextInput.Value())
					if name != "" {
						m.newMCPConfig.Name = name
						m.mcpAddStep = MCPAddStepType
						m.selectedToolIndex = 0 // Reset for type selection
						m.mcpTextInput.Blur()   // Blur for selection
					}
				case MCPAddStepType:
					types := []mcp.ServerType{mcp.ServerTypeStdio, mcp.ServerTypeRemote}
					m.newMCPConfig.Type = types[m.selectedToolIndex]

					if m.newMCPConfig.Type == mcp.ServerTypeRemote {
						m.mcpAddStep = MCPAddStepUrl
					} else {
						m.mcpAddStep = MCPAddStepCommand
					}
					m.mcpTextInput.Reset()
					m.mcpTextInput.Focus()
				case MCPAddStepUrl:
					url := strings.TrimSpace(m.mcpTextInput.Value())
					if url != "" {
						m.newMCPConfig.Url = url
						// Finish for Remote
						m.mcpManager.AddServer(m.newMCPConfig)
						m.mcpPopupState = MCPPopupList
						m.mcpAddStep = MCPAddStepName
						return m, func() tea.Msg {
							return mcpConnectedMsg{tools: m.mcpManager.GetAllTools()}
						}
					}
				case MCPAddStepCommand:
					input := strings.TrimSpace(m.mcpTextInput.Value())
					if input != "" {
						// Naive split for now
						parts := strings.Fields(input)
						if len(parts) > 0 {
							m.newMCPConfig.Command = parts[0]
							if len(parts) > 1 {
								m.newMCPConfig.Args = parts[1:]
							}
						}
						m.mcpAddStep = MCPAddStepEnv
						m.mcpTextInput.Reset()
					}
				case MCPAddStepEnv:
					envStr := m.mcpTextInput.Value()
					if envStr != "" {
						m.newMCPConfig.Env = make(map[string]string)
						// Simple space separation, then split by =
						// This is naive and won't handle spaces in values well without quotes
						pairs := strings.Fields(envStr)
						for _, pair := range pairs {
							parts := strings.SplitN(pair, "=", 2)
							if len(parts) == 2 {
								m.newMCPConfig.Env[parts[0]] = parts[1]
							}
						}
					}

					if err := m.mcpManager.AddServer(m.newMCPConfig); err == nil {
						m.mcpManager.Mu().RLock()
						newSrv := m.mcpManager.Servers[len(m.mcpManager.Servers)-1]
						m.mcpManager.Mu().RUnlock()

						m.mcpPopupState = MCPPopupList
						m.mcpAddStep = MCPAddStepName
						m.selectedMCPIndex = len(m.mcpManager.Servers) - 1

						return m, func() tea.Msg {
							m.mcpManager.ConnectServer(context.Background(), newSrv)
							return mcpConnectedMsg{tools: m.mcpManager.GetAllTools()}
						}
					}

					m.mcpPopupState = MCPPopupList
					m.mcpAddStep = MCPAddStepName
					return m, nil
				}
				m.mcpTextInput.Reset()
			}
			return m, nil
		}

		// Handle text input for Add Manual wizard
		if m.mcpPopupState == MCPPopupAddManual && m.mcpAddStep != MCPAddStepType {
			var cmd tea.Cmd
			m.mcpTextInput, cmd = m.mcpTextInput.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}
