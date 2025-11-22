package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vulpeslab/vulpix/internal/agent"
	"github.com/vulpeslab/vulpix/internal/config"
	"github.com/vulpeslab/vulpix/internal/logger"
	"github.com/vulpeslab/vulpix/internal/mcp"
	"github.com/vulpeslab/vulpix/internal/provider"
	"github.com/vulpeslab/vulpix/internal/rag"
	"github.com/vulpeslab/vulpix/internal/tools"
	"github.com/vulpeslab/vulpix/internal/tui"
	"github.com/vulpeslab/vulpix/pkg/core"
)

func main() {
	// Parse flags
	logLevelFlag := flag.String("log-level", "", "Set log level (DEBUG, INFO, WARNING, ERROR)")
	flag.Parse()

	// 1. Load Config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Determine log level
	logLevel := cfg.LogLevel
	if *logLevelFlag != "" {
		logLevel = strings.ToUpper(*logLevelFlag)
	}
	if logLevel == "" {
		logLevel = "INFO"
	}

	// Check for CLI commands
	// Note: flag.Parse() consumes flags, so os.Args might be different if flags are used.
	// However, the auth command doesn't use flags, it uses subcommands.
	// We need to be careful not to break "auth login".
	// If "auth" is passed, flag.Parse() might treat it as an argument.
	args := flag.Args()
	if len(args) > 1 && args[0] == "auth" && args[1] == "login" {
		p := tea.NewProgram(tui.NewAuthModel(cfg))
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error running auth: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 2. Setup Logger
	log := logger.NewLogger(logLevel)

	// 3. Initialize Provider
	prov, err := provider.NewProvider(cfg)
	if err != nil {
		log.Error("Failed to initialize provider", "error", err)
		// Don't exit, use ErrorProvider to allow TUI to start
		prov = provider.NewErrorProvider(err)
	}

	// Initialize Embedding Provider
	embProv, err := provider.NewEmbeddingProvider(cfg)
	if err != nil {
		log.Warn("Failed to initialize embedding provider", "error", err)
		embProv = provider.NewErrorProvider(err)
	}

	// 4. Initialize RAG Engine
	cwd, _ := os.Getwd()
	dbPath, err := config.GetIndexPath(cwd)
	if err != nil {
		log.Warn("Failed to get index path", "error", err)
		dbPath = "vulpix_chromem.db"
	}

	ragEngine, err := rag.NewEngine(dbPath, cwd, embProv)
	if err != nil {
		log.Warn("Failed to initialize RAG engine (search will be disabled)", "error", err)
	} else {
		// Index in background? Or on demand?
		// For now, let's just have it ready.
		// go ragEngine.Index(context.Background(), cwd)
	}

	// 5. Initialize Tools
	toolList := []core.Tool{
		&tools.WriteFileTool{},
		&tools.ReadFileTool{},
	}

	if ragEngine != nil {
		toolList = append(toolList, tools.NewSearchTool(ragEngine))
	}

	// 6. Initialize Exa MCP
	log.Info("Connecting to Exa MCP...")
	exaCtx := context.Background()
	// Use npx mcp-remote to connect to the hosted Exa MCP server
	// This avoids implementing the complex HTTP/SSE transport manually and leverages the official client.
	// We enable specific tools as requested.
	exaUrl := "https://mcp.exa.ai/mcp?tools=web_search_exa,get_code_context_exa"
	exaClient, err := mcp.NewClient(exaCtx, "npx", []string{"-y", "mcp-remote", exaUrl})
	if err != nil {
		log.Warn("Failed to connect to Exa MCP", "error", err)
	} else {
		// List tools with a timeout
		// npx might take a while to install/start, so give it more time
		listCtx, listCancel := context.WithTimeout(exaCtx, 30*time.Second)
		exaTools, err := exaClient.ListTools(listCtx)
		listCancel()

		if err != nil {
			log.Warn("Failed to list Exa tools", "error", err)
		} else {
			toolList = append(toolList, exaTools...)
			log.Info("Added Exa MCP tools", "count", len(exaTools))
		}
	}

	// 7. Initialize Agent Engine
	engine := agent.NewEngine(prov, toolList, log)

	// 8. Initialize TUI
	model := tui.NewModel(engine, ragEngine, cfg.Model, cfg.AutoApprove, cfg.CollapseReasoning, cfg.TruncateToolResponse)

	// 9. Run
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Error("Error running TUI", "error", err)
		os.Exit(1)
	}
}
