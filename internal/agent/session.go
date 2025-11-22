package agent

import (
	"time"

	"github.com/google/uuid"
	"github.com/vulpeslab/vulpix/pkg/core"
)

type SessionManager struct {
	currentSession *core.Session
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		currentSession: NewSession(),
	}
}

func NewSession() *core.Session {
	return &core.Session{
		ID:        uuid.New().String(),
		Messages:  make([]core.Message, 0),
		Context:   make(map[string]any),
		CreatedAt: time.Now(),
	}
}

func (sm *SessionManager) Current() *core.Session {
	return sm.currentSession
}

func (sm *SessionManager) AddMessage(role, content string) core.Message {
	msg := core.Message{
		ID:        uuid.New().String(),
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}
	sm.currentSession.Messages = append(sm.currentSession.Messages, msg)
	return msg
}

func (sm *SessionManager) AddToolCallMessage(toolCalls []core.ToolCall) core.Message {
	msg := core.Message{
		ID:        uuid.New().String(),
		Role:      "assistant",
		ToolCalls: toolCalls,
		Timestamp: time.Now(),
	}
	sm.currentSession.Messages = append(sm.currentSession.Messages, msg)
	return msg
}

func (sm *SessionManager) AddToolResultMessage(toolCallID, result string) core.Message {
	msg := core.Message{
		ID:         uuid.New().String(),
		Role:       "tool",
		Content:    result,
		ToolCallID: toolCallID,
		Timestamp:  time.Now(),
	}
	sm.currentSession.Messages = append(sm.currentSession.Messages, msg)
	return msg
}

func (sm *SessionManager) Reset() {
	sm.currentSession = NewSession()
}
