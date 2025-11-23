package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/vulpeslab/vulpix/pkg/core"
)

type Engine struct {
	provider     core.Provider
	tools        []core.Tool
	session      *SessionManager
	logger       *slog.Logger
	approvalChan chan bool
	running      bool
}

func NewEngine(provider core.Provider, tools []core.Tool, logger *slog.Logger) *Engine {
	return &Engine{
		provider:     provider,
		tools:        tools,
		session:      NewSessionManager(),
		logger:       logger,
		approvalChan: make(chan bool),
		running:      false,
	}
}

func (e *Engine) IsRunning() bool {
	return e.running
}

func (e *Engine) Approve(approved bool) {
	e.approvalChan <- approved
}

func (e *Engine) Session() *core.Session {
	return e.session.Current()
}

func (e *Engine) AddTools(newTools []core.Tool) {
	e.tools = append(e.tools, newTools...)
}

func (e *Engine) GetContextWindow(ctx context.Context, model string) (int, error) {
	return e.provider.GetContextWindow(ctx, model)
}

func (e *Engine) Chat(ctx context.Context, userInput string, mode core.Mode) (<-chan core.Action, error) {
	modeInstruction := ""
	switch mode {
	case core.ModePlan:
		modeInstruction = "[MODE: PLAN] You are in PLAN mode. Search the codebase and plan the task. Do NOT implement changes. Only use read-only tools.\n"
	case core.ModeAsk:
		modeInstruction = "[MODE: ASK] You are in ASK mode. Answer the user's question. Do NOT write code to files. Only use read-only tools.\n"
	}

	e.session.AddMessage("user", modeInstruction+userInput)
	actionChan := make(chan core.Action)
	e.running = true

	go func() {
		defer close(actionChan)
		defer func() { e.running = false }()
		defer func() {
			if r := recover(); r != nil {
				e.logger.Error("Panic recovered in agent loop", "panic", r)
				actionChan <- core.Action{
					Type:    "error",
					Content: fmt.Sprintf("Internal Error: %v", r),
				}
			}
		}()

		if err := e.runLoop(ctx, actionChan, mode); err != nil {
			e.logger.Error("Agent loop error", "error", err)
			actionChan <- core.Action{
				Type:    "error",
				Content: err.Error(),
			}
		}
	}()

	return actionChan, nil
}

func (e *Engine) runLoop(ctx context.Context, actionChan chan<- core.Action, mode core.Mode) error {
	// Filter tools based on mode
	var allowedTools []core.Tool
	if mode == core.ModeAgent {
		allowedTools = e.tools
	} else {
		for _, t := range e.tools {
			if !t.IsDangerous() {
				allowedTools = append(allowedTools, t)
			}
		}
	}

	maxTurns := 10 // Prevent infinite loops
	for i := 0; i < maxTurns; i++ {
		// 1. Think (Stream completion)
		stream, err := e.provider.StreamCompletion(ctx, e.session.Current().Messages, allowedTools)
		if err != nil {
			return err
		}

		var fullContent strings.Builder
		toolCallsMap := make(map[int]*core.ToolCall)

		for event := range stream {
			switch event.Type {
			case "content":
				fullContent.WriteString(event.Content)
				actionChan <- core.Action{
					Type:    "message",
					Content: event.Content,
				}
			case "reasoning":
				actionChan <- core.Action{
					Type:    "reasoning",
					Content: event.Content,
				}
			case "usage":
				actionChan <- core.Action{
					Type:  "usage",
					Usage: event.Usage,
				}
			case "tool_call":
				for i, tc := range event.ToolCalls {
					if _, exists := toolCallsMap[i]; !exists {
						toolCallsMap[i] = &core.ToolCall{
							ID:        tc.ID,
							Name:      tc.Name,
							Arguments: make(map[string]any),
						}
					}
					// Accumulate arguments string
					if raw, ok := tc.Arguments["_raw"].(string); ok {
						if toolCallsMap[i].Arguments["_raw"] == nil {
							toolCallsMap[i].Arguments["_raw"] = ""
						}
						toolCallsMap[i].Arguments["_raw"] = toolCallsMap[i].Arguments["_raw"].(string) + raw
					}
				}
			}
		}

		content := fullContent.String()

		// Check for XML tool calls if no native tool calls found
		if len(toolCallsMap) == 0 {
			xmlToolCalls := parseXMLToolCalls(content)
			if len(xmlToolCalls) > 0 {
				for i, tc := range xmlToolCalls {
					tCopy := tc
					toolCallsMap[i] = &tCopy
				}
				// Clean up content
				reSection := regexp.MustCompile(`</?tool_calls_section_begin/>[\s\S]*?</?tool_calls_section_end/>`)
				content = reSection.ReplaceAllString(content, "")
				content = strings.TrimSpace(content)
			}
		}

		if content != "" {
			e.session.AddMessage("assistant", content)
		}

		// 2. Execute Tools
		if len(toolCallsMap) > 0 {
			var toolCalls []core.ToolCall
			for i := 0; i < len(toolCallsMap); i++ {
				if tc, ok := toolCallsMap[i]; ok {
					// Parse JSON arguments
					if raw, ok := tc.Arguments["_raw"].(string); ok {
						var args map[string]any
						if err := json.Unmarshal([]byte(raw), &args); err == nil {
							tc.Arguments = args
						} else {
							e.logger.Error("Failed to parse tool arguments", "error", err, "raw", raw)
						}
					}
					toolCalls = append(toolCalls, *tc)
				}
			}

			e.session.AddToolCallMessage(toolCalls)

			for _, tc := range toolCalls {
				// Find tool
				var tool core.Tool
				for _, t := range e.tools {
					if t.Name() == tc.Name {
						tool = t
						break
					}
				}

				var result string
				var err error
				if tool != nil {
					if tool.IsDangerous() {
						actionChan <- core.Action{
							Type:      "approval_request",
							ToolCalls: []core.ToolCall{tc},
						}
						approved := <-e.approvalChan
						if !approved {
							result = "Error: User denied approval"
							e.session.AddToolResultMessage(tc.ID, result)
							actionChan <- core.Action{
								Type:    "tool_result",
								Content: result,
							}
							continue
						}
					}

					actionChan <- core.Action{
						Type:      "tool_start",
						ToolCalls: []core.ToolCall{tc},
					}
					result, err = tool.Execute(ctx, tc.Arguments)
					if err != nil {
						result = fmt.Sprintf("Error: %v", err)
					}
				} else {
					result = fmt.Sprintf("Error: Tool %s not found", tc.Name)
				}

				e.session.AddToolResultMessage(tc.ID, result)
				actionChan <- core.Action{
					Type:    "tool_result",
					Content: result,
				}
			}
		} else {
			// No tool calls, we are done with this turn
			break
		}
	}
	return nil
}

func (e *Engine) Summarize(ctx context.Context) (string, error) {
	prompt := `Please summarize the current conversation state to continue in a new context window.
You MUST include:
1. The EXACT user prompt(s) that initiated the current task.
2. A quick summary of what has been done so far.
3. Explicit details about what you were doing in this moment, including exactly what tool calls were made and why.
The goal is to allow you to resume work seamlessly in a new session.`

	// Create a temporary message list with the summarization prompt
	messages := append(e.session.Current().Messages, core.Message{
		Role:    "user",
		Content: prompt,
	})

	// Stream completion without tools to get the summary
	stream, err := e.provider.StreamCompletion(ctx, messages, nil)
	if err != nil {
		return "", err
	}

	var summary strings.Builder
	for event := range stream {
		if event.Type == "content" {
			summary.WriteString(event.Content)
		}
	}

	return summary.String(), nil
}

func (e *Engine) ResetSession(summary string) {
	e.session.Reset()
	// Add the summary as the first message.
	// We use "user" role because it acts as the prompt for the new session.
	e.session.AddMessage("user", "Previous Context Summary:\n"+summary+"\n\nPlease continue from where you left off.")
}

func (e *Engine) ClearSession() {
	e.session.Reset()
}

func parseXMLToolCalls(content string) []core.ToolCall {
	var toolCalls []core.ToolCall
	// Regex for a single tool call:
	// <tool_call_begin/>\s*(?P<name>[\w\.]+):(?P<id>\w+)\s*<tool_call_argument_begin/>\s*(?P<args>\{.*?\})\s*<tool_call_end/>
	re := regexp.MustCompile(`<tool_call_begin/>\s*([\w\.\-]+):(\w+)\s*<tool_call_argument_begin/>\s*(\{.*?\})\s*<tool_call_end/>`)
	matches := re.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		name := match[1]
		id := match[2]
		argsJSON := match[3]

		// Clean up name
		name = strings.TrimPrefix(name, "functions.")

		var args map[string]any
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			continue
		}

		toolCalls = append(toolCalls, core.ToolCall{
			ID:        id,
			Name:      name,
			Arguments: args,
		})
	}

	return toolCalls
}
