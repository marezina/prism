package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"prism/internal/logfile"
)

// newLogger logs to <logs>/prism.log, rotated 5 × 5 MB (ARCHITECTURE §6),
// and to stderr. Tokens and comment bodies are never logged (CLAUDE.md §4).
func newLogger(dir string, debug bool) (*slog.Logger, func(), error) {
	f, err := logfile.Open(filepath.Join(dir, "prism.log"))
	if err != nil {
		return nil, nil, err
	}
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	h := slog.NewTextHandler(io.MultiWriter(f, os.Stderr), &slog.HandlerOptions{Level: level})
	return slog.New(h), func() { _ = f.Close() }, nil
}
