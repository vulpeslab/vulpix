# Vulpix AI Agent Guide

This document guides AI agents working on the Vulpix codebase. Vulpix is a terminal-based AI coding agent written in Go.

## 🏗 Architecture Overview

Vulpix follows the standard Go project layout:

- **`cmd/vulpix/`**: Application entry point. Wires components (Config, Logger, Provider, RAG, Tools) and starts the TUI.
- **`pkg/core/`**: Defines the domain domain models and interfaces (`Provider`, `Tool`, `Session`, `Message`). **Read this first** to understand the contracts.
- **`internal/`**: Private application code.
  - **`tui/`**: Terminal UI using [Bubble Tea](https://github.com/charmbracelet/bubbletea). Follows The Elm Architecture (Model, Update, View).
  - **`provider/`**: LLM and Embedding implementations (OpenAI, Anthropic).
  - **`rag/`**: RAG engine using [chromem-go](https://github.com/philippgille/chromem-go) for local code indexing and search.
  - **`agent/`**: Core agent orchestration logic.
  - **`tools/`**: Tool implementations (Filesystem, Search).
  - **`mcp/`**: Model Context Protocol client integration.

## 🛠 Critical Workflows

### Build & Run
- **Build**: `make build` (outputs to `bin/vulpix`)
- **Run**: `go run ./cmd/vulpix`
- **Cross-compile**: `make build-all` (Linux, macOS, Windows)

### Testing & Quality
- **Test**: `make test` (runs `go test -v -race ./...`)
- **Coverage**: `make test-coverage` (generates HTML report)
- **Lint**: `make lint` (requires `golangci-lint`)

## 🧩 Development Patterns

### TUI Development (Bubble Tea)
- **Models**: State is stored in `tui/model.go`.
- **Updates**: State transitions happen in `Update()` methods.
- **Views**: UI rendering is in `View()` methods, styled with [Lip Gloss](https://github.com/charmbracelet/lipgloss).
- **Commands**: Side effects (like API calls) return `tea.Cmd`.

### Adding New Tools
1. Create a struct in `internal/tools/` implementing `core.Tool`.
2. Register it in `cmd/vulpix/main.go` in the `toolList`.

### Adding LLM Providers
1. Implement `core.Provider` interface in `internal/provider/`.
2. Add initialization logic in `internal/provider/factory.go` (or similar).

### RAG & Embeddings
- The RAG engine indexes the current working directory.
- It uses a local vector DB (`vulpix_chromem.db`).
- Embeddings are abstracted via `core.EmbeddingProvider`.

## 📦 Dependencies & Integration

- **UI**: `charmbracelet/bubbletea`, `lipgloss`, `glamour` (Markdown rendering).
- **LLM**: `sashabaranov/go-openai` (used for OpenAI compatible APIs).
- **Vector DB**: `philippgille/chromem-go`.
- **MCP**: `modelcontextprotocol/go-sdk`.

## ⚠️ Common Pitfalls
- **Configuration**: Config is loaded via `internal/config`. Ensure environment variables or config files are set up for providers (e.g., `OPENAI_API_KEY`).
- **Concurrency**: The TUI runs on the main thread. Long-running tasks (like LLM streaming) must be handled via `tea.Cmd` to avoid freezing the UI.
