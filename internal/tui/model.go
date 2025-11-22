package tui

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/vulpeslab/vulpix/internal/agent"
	"github.com/vulpeslab/vulpix/internal/rag"
	"github.com/vulpeslab/vulpix/pkg/core"
)

type Command struct {
	Name        string
	Description string
}

var leadingWhitespaceRegex = regexp.MustCompile(`^(?:(?:\x1b\[[0-9;?]*[a-zA-Z])|[\s\n])+`)

var commands = []Command{
	{"/help", "Show help message"},
	{"/clear", "Clear chat history"},
	{"/exit", "Exit Vulpix"},
	{"/auth", "Configure authentication"},
	{"/index", "Index the current codebase"},
	{"/summarize", "Summarize context and start fresh"},
}

type HistoryItem struct {
	Type    string
	Content string
	Name    string
}

type Model struct {
	viewport           viewport.Model
	textarea           textarea.Model
	spinner            spinner.Model
	processing         bool
	lastError          error
	ready              bool
	engine             *agent.Engine
	ragEngine          *rag.Engine
	actionChan         <-chan core.Action
	history            []HistoryItem
	ctx                context.Context
	cancel             context.CancelFunc
	waitingForApproval bool
	renderer           *glamour.TermRenderer
	progressChan       <-chan string

	// Auto-complete state
	showSuggestions  bool
	suggestionIdx    int
	filteredCommands []Command

	// Status info
	modelName            string
	contextWindow        int
	mode                 core.Mode
	yoloMode             bool
	tokenUsage           core.Usage
	needsSummarize       bool
	collapseReasoning    bool
	truncateToolResponse bool

	// Double-press protection
	lastCancelTime time.Time
	lastExitTime   time.Time
}

const Version = "0.1.0"

// NewModel creates a new TUI model
func NewModel(engine *agent.Engine, ragEngine *rag.Engine, modelName string, autoApprove bool, collapseReasoning bool, truncateToolResponse bool) Model {
	ta := textarea.New()
	ta.Placeholder = "Ask Vulpix..."
	ta.Focus()

	ta.Prompt = "> "
	ta.CharLimit = 0 // Unlimited
	ta.SetWidth(30)
	ta.SetHeight(1) // Simplified height

	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.ShowLineNumbers = false

	vp := viewport.New(30, 5)
	// Initial welcome message
	history := []HistoryItem{{Type: "assistant", Content: fmt.Sprintf("# Vulpix v%s\n", Version)}}

	ctx, cancel := context.WithCancel(context.Background())

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(80),
	)

	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return Model{
		textarea:             ta,
		viewport:             vp,
		engine:               engine,
		ragEngine:            ragEngine,
		ctx:                  ctx,
		cancel:               cancel,
		renderer:             renderer,
		history:              history,
		modelName:            modelName,
		contextWindow:        0, // Will be fetched in Init
		spinner:              s,
		processing:           false,
		mode:                 core.ModeAgent,
		yoloMode:             autoApprove,
		collapseReasoning:    collapseReasoning,
		truncateToolResponse: truncateToolResponse,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.fetchContextWindow(),
	)
}

func (m Model) fetchContextWindow() tea.Cmd {
	return func() tea.Msg {
		cw, err := m.engine.GetContextWindow(m.ctx, m.modelName)
		if err != nil {
			return contextWindowMsg{err: err}
		}
		return contextWindowMsg{size: cw}
	}
}

type contextWindowMsg struct {
	size int
	err  error
}

type actionMsg core.Action

type doneMsg struct{}

type indexingResultMsg struct {
	err     error
	message string
}

type clearStatusMsg struct{}

func waitForAction(sub <-chan core.Action) tea.Cmd {
	return func() tea.Msg {
		action, ok := <-sub
		if !ok {
			return doneMsg{}
		}
		return actionMsg(action)
	}
}

func (m Model) renderHistory() string {
	var b strings.Builder
	for _, item := range m.history {
		switch item.Type {
		case "user":
			prefix := lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true).Render("> ")
			// Render markdown but trim leading newlines and spaces so it sits next to the prefix
			rendered := leadingWhitespaceRegex.ReplaceAllStringFunc(m.renderMarkdown(item.Content), func(s string) string {
				var sb strings.Builder
				for _, r := range s {
					if r != ' ' && r != '\n' && r != '\t' && r != '\r' {
						sb.WriteRune(r)
					}
				}
				return sb.String()
			})
			b.WriteString(fmt.Sprintf("%s%s", prefix, rendered))

		case "assistant":
			content := item.Content
			// Skip icon for welcome message
			if strings.HasPrefix(content, "# Vulpix v") {
				b.WriteString(m.renderMarkdown(content))
				continue
			}

			prefix := lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Render("✦ ")
			rendered := leadingWhitespaceRegex.ReplaceAllStringFunc(m.renderMarkdown(content), func(s string) string {
				var sb strings.Builder
				for _, r := range s {
					if r != ' ' && r != '\n' && r != '\t' && r != '\r' {
						sb.WriteRune(r)
					}
				}
				return sb.String()
			})
			b.WriteString(fmt.Sprintf("%s%s", prefix, rendered))

		case "tool_start":
			b.WriteString(fmt.Sprintf("%s\n\n", ToolExecStyle.Render(fmt.Sprintf("[Executing %s...]", item.Name))))
		case "tool_result":
			content := item.Content
			if m.truncateToolResponse {
				lines := strings.Split(content, "\n")
				if len(lines) > 3 {
					truncated := strings.Join(lines[:3], "\n")
					remaining := len(lines) - 3
					content = fmt.Sprintf("%s\n--- %d more lines ---", truncated, remaining)
				}
			}
			res := fmt.Sprintf("**Tool Result**:\n```\n%s\n```\n", content)
			b.WriteString(m.renderMarkdown(res))
		case "error":
			b.WriteString(fmt.Sprintf("%s\n\n", ErrorStyle.Render(fmt.Sprintf("[Error: %s]", item.Content))))
		case "approval_request":
			b.WriteString(fmt.Sprintf("%s\n\n", ApprovalPromptStyle.Render(fmt.Sprintf("Allow execution of %s? (y/n)", item.Name))))
		case "approved":
			b.WriteString(fmt.Sprintf("%s\n\n", SuccessStyle.Render("[Approved]")))
		case "denied":
			b.WriteString(fmt.Sprintf("%s\n\n", ErrorStyle.Render("[Denied]")))
		case "cancelled":
			b.WriteString("[Cancelled by user]\n\n")
		case "reasoning":
			// Gray "°" for thinking
			header := ReasoningStyle.Render("° Thinking...")

			if m.collapseReasoning {
				hint := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" (Ctrl + O to show full reasoning)")
				b.WriteString(fmt.Sprintf("%s%s\n\n", header, hint))
			} else {
				// Indent the content
				width := m.viewport.Width - 4
				if width < 10 {
					width = 10
				}
				wrapped := wordwrap.String(strings.TrimSpace(item.Content), width)
				indentedContent := "    " + strings.ReplaceAll(wrapped, "\n", "\n    ")
				// Add extra newline at the end for spacing before response
				b.WriteString(fmt.Sprintf("%s\n%s\n\n", header, ReasoningStyle.Render(indentedContent)))
			}
		}
	}
	return b.String()
}

func (m Model) renderMarkdown(text string) string {
	if m.renderer == nil {
		return text
	}
	out, err := m.renderer.Render(text)
	if err != nil {
		return text
	}
	return out
}

type progressMsg string

func waitForProgress(sub <-chan string) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-sub
		if !ok {
			return nil
		}
		return progressMsg(msg)
	}
}

type summarizeMsg struct {
	summary string
	err     error
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)

	// Handle auto-complete navigation before textarea update
	if m.showSuggestions {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyUp:
				if m.suggestionIdx > 0 {
					m.suggestionIdx--
				}
				return m, nil
			case tea.KeyDown:
				if m.suggestionIdx < len(m.filteredCommands)-1 {
					m.suggestionIdx++
				}
				return m, nil
			case tea.KeyEnter, tea.KeyTab:
				if len(m.filteredCommands) > 0 {
					cmd := m.filteredCommands[m.suggestionIdx]
					m.textarea.SetValue(cmd.Name + " ")
					m.textarea.SetCursor(len(cmd.Name) + 1)
					m.showSuggestions = false
					return m, nil
				}
			case tea.KeyEsc:
				m.showSuggestions = false
				return m, nil
			}
		}
	}

	m.textarea, tiCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)

	// Check for auto-complete trigger
	input := m.textarea.Value()
	if strings.HasPrefix(input, "/") && !strings.Contains(input, " ") {
		m.filteredCommands = []Command{}
		for _, cmd := range commands {
			if strings.HasPrefix(cmd.Name, input) {
				m.filteredCommands = append(m.filteredCommands, cmd)
			}
		}
		if len(m.filteredCommands) > 0 {
			m.showSuggestions = true
			// Reset index if out of bounds or keep it? Resetting is safer.
			if m.suggestionIdx >= len(m.filteredCommands) {
				m.suggestionIdx = 0
			}
		} else {
			m.showSuggestions = false
		}
	} else {
		m.showSuggestions = false
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyShiftTab:
			switch m.mode {
			case core.ModeAgent:
				m.mode = core.ModePlan
			case core.ModePlan:
				m.mode = core.ModeAsk
			case core.ModeAsk:
				m.mode = core.ModeAgent
			}
			return m, nil
		case tea.KeyCtrlC:
			if time.Since(m.lastExitTime) < 3*time.Second {
				m.cancel()
				return m, tea.Quit
			}
			m.lastExitTime = time.Now()
			return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		case tea.KeyCtrlY:
			m.yoloMode = !m.yoloMode
			return m, nil
		case tea.KeyCtrlO:
			m.collapseReasoning = !m.collapseReasoning
			m.viewport.SetContent(m.renderHistory())
			return m, nil
		case tea.KeyCtrlT:
			m.truncateToolResponse = !m.truncateToolResponse
			m.viewport.SetContent(m.renderHistory())
			return m, nil
		case tea.KeyEsc:
			if m.engine.IsRunning() {
				if time.Since(m.lastCancelTime) < 3*time.Second {
					m.cancel()
					// Reset context for next run
					m.ctx, m.cancel = context.WithCancel(context.Background())
					m.history = append(m.history, HistoryItem{Type: "cancelled"})
					m.viewport.SetContent(m.renderHistory())
					m.viewport.GotoBottom()
					m.processing = false
					return m, nil
				}
				m.lastCancelTime = time.Now()
				return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
					return clearStatusMsg{}
				})
			}
			return m, tea.Quit
		case tea.KeyEnter:
			if m.waitingForApproval {
				return m, nil
			}
			if !msg.Alt {
				input := m.textarea.Value()
				if input == "" {
					break
				}
				m.textarea.Reset()

				// Handle commands
				if strings.HasPrefix(input, "/") {
					parts := strings.Fields(input)
					switch parts[0] {
					case "/exit":
						return m, tea.Quit
					case "/clear":
						m.engine.ClearSession()
						m.history = []HistoryItem{{Type: "assistant", Content: fmt.Sprintf("# Vulpix v%s\n", Version)}}
						m.viewport.SetContent(m.renderHistory())
						// Reset token usage
						m.tokenUsage = core.Usage{}
						return m, nil
					case "/index":
						if m.ragEngine == nil {
							m.history = append(m.history, HistoryItem{Type: "error", Content: "RAG Engine not initialized"})
							m.viewport.SetContent(m.renderHistory())
							m.viewport.GotoBottom()
							return m, nil
						}
						m.history = append(m.history, HistoryItem{Type: "assistant", Content: "Indexing codebase..."})
						m.viewport.SetContent(m.renderHistory())
						m.viewport.GotoBottom()
						m.processing = true

						progressChan := make(chan string)
						m.progressChan = progressChan

						return m, tea.Batch(
							func() tea.Msg {
								defer close(progressChan)
								cwd, _ := os.Getwd()
								err := m.ragEngine.Index(m.ctx, cwd, progressChan)
								if err != nil {
									return indexingResultMsg{err: err}
								}
								return indexingResultMsg{message: "Indexing completed successfully."}
							},
							waitForProgress(progressChan),
							m.spinner.Tick,
						)
					case "/summarize":
						m.processing = true
						m.history = append(m.history, HistoryItem{Type: "assistant", Content: "Summarizing context..."})
						m.viewport.SetContent(m.renderHistory())
						m.viewport.GotoBottom()
						return m, tea.Batch(
							func() tea.Msg {
								summary, err := m.engine.Summarize(m.ctx)
								return summarizeMsg{summary: summary, err: err}
							},
							m.spinner.Tick,
						)
					}
				}

				m.history = append(m.history, HistoryItem{Type: "user", Content: input})
				m.viewport.SetContent(m.renderHistory())
				m.viewport.GotoBottom()

				var err error
				m.actionChan, err = m.engine.Chat(m.ctx, input, m.mode)
				if err != nil {
					m.lastError = err
					return m, nil
				}
				m.processing = true
				return m, tea.Batch(waitForAction(m.actionChan), m.spinner.Tick)
			}
		case tea.KeyRunes:
			if m.waitingForApproval {
				switch msg.String() {
				case "y", "Y":
					m.waitingForApproval = false
					m.history = append(m.history, HistoryItem{Type: "approved"})
					m.viewport.SetContent(m.renderHistory())
					m.viewport.GotoBottom()
					go m.engine.Approve(true)
					return m, waitForAction(m.actionChan)
				case "n", "N":
					m.waitingForApproval = false
					m.history = append(m.history, HistoryItem{Type: "denied"})
					m.viewport.SetContent(m.renderHistory())
					m.viewport.GotoBottom()
					go m.engine.Approve(false)
					return m, waitForAction(m.actionChan)
				}
			}
		}
	case spinner.TickMsg:
		if !m.processing {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case progressMsg:
		// Update the last history item if it's an assistant message (which we used for "Indexing codebase...")
		// Or append a new one? Updating is cleaner for progress.
		// Actually, let's just append a log line or update the status line.
		// Since we want to show "actual status", maybe we can update the last message.
		if len(m.history) > 0 {
			lastIdx := len(m.history) - 1
			if m.history[lastIdx].Type == "assistant" && strings.Contains(m.history[lastIdx].Content, "Indexing") {
				m.history[lastIdx].Content = fmt.Sprintf("Indexing codebase...\n> %s", string(msg))
				m.viewport.SetContent(m.renderHistory())
				m.viewport.GotoBottom()
			}
		}
		// Continue waiting for progress
		// We need to know the channel to wait on.
		// The issue is waitForProgress needs the channel.
		// We can't easily get the channel back here unless we store it in the model.
		// But we created it in the command.
		// A better way is to have the command return a struct that contains the channel,
		// or just have the command loop and send messages.
		// But tea.Cmd is a function that returns a Msg.
		// The `waitForProgress` pattern I used above is slightly flawed because it only returns ONE message.
		// It needs to re-subscribe.
		// But it doesn't have the channel.

		// Let's fix this by storing the progress channel in the model if we want to use this pattern,
		// OR, we can use a different pattern where the command itself loops and sends messages?
		// No, tea.Cmd is one-shot.

		// Correct approach: Store the channel in the model.
		return m, waitForProgress(m.progressChan)

	case summarizeMsg:
		m.processing = false
		if msg.err != nil {
			m.history = append(m.history, HistoryItem{Type: "error", Content: fmt.Sprintf("Summarization failed: %v", msg.err)})
		} else {
			m.engine.ResetSession(msg.summary)
			m.history = append(m.history, HistoryItem{Type: "assistant", Content: "Conversation summarized. Context reset."})
			// Reset token usage
			m.tokenUsage = core.Usage{}
		}
		m.viewport.SetContent(m.renderHistory())
		m.viewport.GotoBottom()
		return m, nil

	case indexingResultMsg:
		m.processing = false
		if msg.err != nil {
			m.history = append(m.history, HistoryItem{Type: "error", Content: fmt.Sprintf("Indexing failed: %v", msg.err)})
		} else {
			m.history = append(m.history, HistoryItem{Type: "assistant", Content: msg.message})
		}
		m.viewport.SetContent(m.renderHistory())
		m.viewport.GotoBottom()
		return m, nil

	case clearStatusMsg:
		// Just triggers a re-render to clear the status message
		return m, nil

	case doneMsg:
		m.processing = false
		if m.needsSummarize {
			m.needsSummarize = false
			m.processing = true
			m.history = append(m.history, HistoryItem{Type: "assistant", Content: "Context low (<20%). Auto-summarizing..."})
			m.viewport.SetContent(m.renderHistory())
			m.viewport.GotoBottom()
			return m, tea.Batch(
				func() tea.Msg {
					summary, err := m.engine.Summarize(m.ctx)
					return summarizeMsg{summary: summary, err: err}
				},
				m.spinner.Tick,
			)
		}
		return m, nil
	case actionMsg:
		switch msg.Type {
		case "message":
			// Append to last assistant message if possible, or create new
			if len(m.history) > 0 && m.history[len(m.history)-1].Type == "assistant" {
				m.history[len(m.history)-1].Content += msg.Content
			} else {
				m.history = append(m.history, HistoryItem{Type: "assistant", Content: msg.Content})
			}
		case "tool_start":
			m.history = append(m.history, HistoryItem{Type: "tool_start", Name: msg.ToolCalls[0].Name})
		case "approval_request":
			if m.yoloMode {
				go m.engine.Approve(true)
			} else {
				m.waitingForApproval = true
				m.history = append(m.history, HistoryItem{Type: "approval_request", Name: msg.ToolCalls[0].Name})
			}
		case "tool_result":
			m.history = append(m.history, HistoryItem{Type: "tool_result", Content: msg.Content})
		case "error":
			m.history = append(m.history, HistoryItem{Type: "error", Content: msg.Content})
		case "reasoning":
			if len(m.history) > 0 && m.history[len(m.history)-1].Type == "reasoning" {
				m.history[len(m.history)-1].Content += msg.Content
			} else {
				m.history = append(m.history, HistoryItem{Type: "reasoning", Content: msg.Content})
			}
		case "usage":
			m.tokenUsage.InputTokens += msg.Usage.InputTokens
			m.tokenUsage.OutputTokens += msg.Usage.OutputTokens
			m.tokenUsage.TotalTokens += msg.Usage.TotalTokens
			// If TotalTokens is not provided, calculate it
			if msg.Usage.TotalTokens == 0 {
				m.tokenUsage.TotalTokens = m.tokenUsage.InputTokens + m.tokenUsage.OutputTokens
			}

			if m.contextWindow > 0 {
				used := m.tokenUsage.TotalTokens
				remaining := m.contextWindow - used
				percentLeft := (float64(remaining) / float64(m.contextWindow)) * 100
				if percentLeft < 20 {
					m.needsSummarize = true
				}
			}
		}
		m.viewport.SetContent(m.renderHistory())
		m.viewport.GotoBottom()
		return m, waitForAction(m.actionChan)

	case tea.WindowSizeMsg:
		// Textarea (1 line content + 2 border) = 3
		// Status bar = 1
		footerHeight := 4
		verticalMarginHeight := footerHeight

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
			m.viewport.YPosition = 0
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMarginHeight
		}

		// Update renderer width
		m.renderer, _ = glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(msg.Width-4), // Padding
		)

		m.textarea.SetWidth(msg.Width - 2) // Account for borders
		m.viewport.SetContent(m.renderHistory())

	case contextWindowMsg:
		if msg.err == nil {
			m.contextWindow = msg.size
		}
	}

	return m, tea.Batch(tiCmd, vpCmd)
}

func (m Model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	var errorView string
	if m.lastError != nil {
		errorView = fmt.Sprintf("\n%s", ErrorStyle.Render(fmt.Sprintf("Error: %v", m.lastError)))
	}

	// Textarea with border
	textareaView := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
		Render(m.textarea.View())

	// Status Bar
	var contextLeftStr string
	if m.contextWindow > 0 {
		used := m.tokenUsage.TotalTokens
		remaining := m.contextWindow - used
		percent := (float64(remaining) / float64(m.contextWindow)) * 100
		if percent < 0 {
			percent = 0
		}
		contextLeftStr = fmt.Sprintf("%.1f%%", percent)
	} else {
		contextLeftStr = ErrorStyle.Render("Unknown")
	}

	statusContent := fmt.Sprintf("Mode: %s | Model: %s | Context Left: %s", m.mode, m.modelName, contextLeftStr)
	if m.processing {
		statusContent += fmt.Sprintf(" | %s Processing...", m.spinner.View())
	}
	statusBar := StatusBarStyle.
		Width(m.viewport.Width).
		Render(statusContent)

	// Yolo Status Bar
	var yoloStatusBar string

	// Check for warnings first
	if time.Since(m.lastExitTime) < 3*time.Second {
		yoloStatusBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")). // Red
			Bold(true).
			Align(lipgloss.Right).
			Width(m.viewport.Width).
			Render("Press CTRL+C again to close vulpix")
	} else if time.Since(m.lastCancelTime) < 3*time.Second {
		yoloStatusBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208")). // Orange
			Bold(true).
			Align(lipgloss.Right).
			Width(m.viewport.Width).
			Render("Press ESC again to interrupt")
	} else if m.yoloMode {
		yoloText := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("YOLO mode")
		toggleText := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" (ctrl + y to toggle)")
		yoloStatusBar = lipgloss.NewStyle().
			Align(lipgloss.Right).
			Width(m.viewport.Width).
			Render(yoloText + toggleText)
	}

	// Render suggestions overlay
	if m.showSuggestions {
		var suggestions []string
		for i, cmd := range m.filteredCommands {
			style := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
			if i == m.suggestionIdx {
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
			}
			suggestions = append(suggestions, style.Render(fmt.Sprintf("%-10s %s", cmd.Name, cmd.Description)))
		}
		suggestionBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Background(lipgloss.Color("235")).
			Padding(0, 1).
			Render(strings.Join(suggestions, "\n"))

		// We want to render this *above* the textarea.
		views := []string{m.viewport.View()}
		if yoloStatusBar != "" {
			views = append(views, yoloStatusBar)
		}
		views = append(views, suggestionBox, textareaView, statusBar)
		if errorView != "" {
			views = append(views, errorView)
		}
		return lipgloss.JoinVertical(lipgloss.Left, views...)
	}

	views := []string{m.viewport.View()}
	if yoloStatusBar != "" {
		views = append(views, yoloStatusBar)
	}
	views = append(views, textareaView, statusBar)
	if errorView != "" {
		views = append(views, errorView)
	}
	return lipgloss.JoinVertical(lipgloss.Left, views...)
}
