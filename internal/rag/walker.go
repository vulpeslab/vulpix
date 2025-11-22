package rag

import (
	"os"
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
	"github.com/vulpeslab/vulpix/internal/logger"
)

type Walker struct {
	rootPath string
	ignore   *ignore.GitIgnore
}

func NewWalker(rootPath string) (*Walker, error) {
	w := &Walker{rootPath: rootPath}
	if err := w.loadIgnore(); err != nil {
		// If no .gitignore, just ignore .git
		w.ignore = ignore.CompileIgnoreLines(".git")
	}
	return w, nil
}

func (w *Walker) loadIgnore() error {
	ignorePath := filepath.Join(w.rootPath, ".gitignore")
	if _, err := os.Stat(ignorePath); os.IsNotExist(err) {
		return err
	}

	w.ignore, _ = ignore.CompileIgnoreFile(ignorePath)
	return nil
}

func (w *Walker) Walk(fn func(path string) error) error {
	logger.Log.Debug("Starting walk", "root", w.rootPath)
	return filepath.Walk(w.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Log.Error("Error walking path", "path", path, "error", err)
			return err
		}

		relPath, err := filepath.Rel(w.rootPath, path)
		if err != nil {
			return err
		}

		if info.IsDir() {
			if w.ignore != nil && w.ignore.MatchesPath(relPath) {
				logger.Log.Debug("Ignoring directory", "path", relPath)
				return filepath.SkipDir
			}
			if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				logger.Log.Debug("Ignoring hidden directory", "path", relPath)
				return filepath.SkipDir
			}
			return nil
		}

		if w.ignore != nil && w.ignore.MatchesPath(relPath) {
			logger.Log.Debug("Ignoring file", "path", relPath)
			return nil
		}

		// Extension filter
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go", ".md", ".txt", ".js", ".ts", ".py", ".java", ".c", ".cpp", ".h", ".rs", ".json", ".yaml", ".yml", ".toml":
			logger.Log.Debug("Visiting file", "path", path)
			return fn(path)
		}

		return nil
	})
}
