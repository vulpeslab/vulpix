package mcp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vulpeslab/vulpix/pkg/core"
)

type ServerStatus string

const (
	StatusConnected    ServerStatus = "connected"
	StatusConnecting   ServerStatus = "connecting"
	StatusDisconnected ServerStatus = "disconnected"
	StatusError        ServerStatus = "error"
)

type ServerState struct {
	Config    ServerConfig
	Status    ServerStatus
	Client    *Client
	Tools     []core.Tool
	Error     error
	IsBuiltin bool
}

type Manager struct {
	Servers []*ServerState
	mu      sync.RWMutex
}

func (m *Manager) Mu() *sync.RWMutex {
	return &m.mu
}

func NewManager() *Manager {
	return &Manager{
		Servers: []*ServerState{},
	}
}

func (m *Manager) LoadServers() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var newServers []*ServerState

	// Add Built-in Exa
	newServers = append(newServers, &ServerState{
		Config: ServerConfig{
			Name: "exa-search",
			Type: ServerTypeRemote,
			Url:  "https://mcp.exa.ai/mcp?tools=web_search_exa,get_code_context_exa",
		},
		Status:    StatusDisconnected,
		IsBuiltin: true,
	})

	for _, srv := range cfg.Servers {
		newServers = append(newServers, &ServerState{
			Config: srv,
			Status: StatusDisconnected,
		})
	}

	m.Servers = newServers
	return nil
}

func (m *Manager) ConnectAll(ctx context.Context) {
	m.mu.Lock()
	servers := m.Servers // Copy slice to avoid holding lock during connect
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, s := range servers {
		if s.Config.Disabled {
			continue
		}
		wg.Add(1)
		go func(srv *ServerState) {
			defer wg.Done()
			m.ConnectServer(ctx, srv)
		}(s)
	}
	wg.Wait()
}

func (m *Manager) ConnectServer(ctx context.Context, s *ServerState) error {
	m.mu.Lock()
	s.Status = StatusConnecting
	s.Error = nil
	m.mu.Unlock()

	var client *Client
	var err error

	if s.Config.Type == ServerTypeRemote {
		// Use npx mcp-remote
		client, err = NewClient(ctx, "npx", []string{"-y", "mcp-remote", s.Config.Url}, s.Config.Env)
	} else {
		client, err = NewClient(ctx, s.Config.Command, s.Config.Args, s.Config.Env)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err != nil {
		s.Status = StatusError
		s.Error = err
		return err
	}

	s.Client = client

	// List tools
	listCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	tools, err := client.ListTools(listCtx)
	if err != nil {
		s.Status = StatusError
		s.Error = err
		client.Close()
		s.Client = nil
		return err
	}

	s.Tools = tools
	s.Status = StatusConnected
	return nil
}

func (m *Manager) DisconnectServer(s *ServerState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s.Client != nil {
		s.Client.Close()
		s.Client = nil
	}
	s.Status = StatusDisconnected
	s.Tools = nil
	return nil
}

func (m *Manager) GetAllTools() []core.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var tools []core.Tool
	for _, s := range m.Servers {
		if s.Status == StatusConnected {
			tools = append(tools, s.Tools...)
		}
	}
	return tools
}

func (m *Manager) RemoveServer(index int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index < 0 || index >= len(m.Servers) {
		return fmt.Errorf("invalid server index")
	}

	srv := m.Servers[index]
	if srv.IsBuiltin {
		return fmt.Errorf("cannot remove built-in server")
	}

	// Disconnect if connected
	if srv.Client != nil {
		srv.Client.Close()
	}

	// Remove from list
	m.Servers = append(m.Servers[:index], m.Servers[index+1:]...)

	return m.save()
}

func (m *Manager) ToggleServer(index int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index < 0 || index >= len(m.Servers) {
		return fmt.Errorf("invalid server index")
	}

	srv := m.Servers[index]
	if srv.IsBuiltin {
		return fmt.Errorf("cannot disable built-in server")
	}

	srv.Config.Disabled = !srv.Config.Disabled

	// If disabled, disconnect
	if srv.Config.Disabled && srv.Client != nil {
		srv.Client.Close()
		srv.Client = nil
		srv.Status = StatusDisconnected
		srv.Tools = nil
	}

	return m.save()
}

func (m *Manager) AddServer(config ServerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicates
	for _, s := range m.Servers {
		if s.Config.Name == config.Name {
			return fmt.Errorf("server with name %s already exists", config.Name)
		}
	}

	m.Servers = append(m.Servers, &ServerState{
		Config: config,
		Status: StatusDisconnected,
	})

	return m.save()
}

func (m *Manager) save() error {
	var cfg Config
	for _, s := range m.Servers {
		if !s.IsBuiltin {
			cfg.Servers = append(cfg.Servers, s.Config)
		}
	}
	return SaveConfig(&cfg)
}
