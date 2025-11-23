package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vulpeslab/vulpix/pkg/core"
)

type Client struct {
	client  *mcp.Client
	session *mcp.ClientSession
}

// MCPTool implements core.Tool interface
type MCPTool struct {
	name        string
	description string
	schema      string
	client      *Client
}

func (t *MCPTool) Name() string        { return t.name }
func (t *MCPTool) Description() string { return t.description }
func (t *MCPTool) Schema() string      { return t.schema }
func (t *MCPTool) IsDangerous() bool   { return true } // Assume all MCP tools are dangerous for now
func (t *MCPTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	return t.client.CallTool(ctx, t.name, args)
}

func NewClient(ctx context.Context, command string, args []string, env map[string]string) (*Client, error) {
	cmd := exec.Command(command, args...)
	if len(env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	transport := &mcp.CommandTransport{
		Command: cmd,
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "vulpix",
		Version: "0.1.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MCP server: %w", err)
	}

	return &Client{
		client:  client,
		session: session,
	}, nil
}

func (c *Client) Close() error {
	return c.session.Close()
}

func (c *Client) ListTools(ctx context.Context) ([]core.Tool, error) {
	res, err := c.session.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}

	var tools []core.Tool
	for _, t := range res.Tools {
		schemaBytes, err := json.Marshal(t.InputSchema)
		if err != nil {
			continue
		}

		tools = append(tools, &MCPTool{
			name:        t.Name,
			description: t.Description,
			schema:      string(schemaBytes),
			client:      c,
		})
	}
	return tools, nil
}

func (c *Client) CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	res, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, content := range res.Content {
		switch tc := content.(type) {
		case *mcp.TextContent:
			sb.WriteString(tc.Text)
		}
	}

	if res.IsError {
		return sb.String(), fmt.Errorf("tool execution failed: %s", sb.String())
	}

	return sb.String(), nil
}
