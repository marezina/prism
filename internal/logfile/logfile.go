// Package logfile is a size-rotated log file: prism.log, prism.log.1 … .N
// (ARCHITECTURE §6: logs/prism.log rotated 5 × 5 MB).
package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	// MaxSize is the size at which the current file is rotated.
	MaxSize = 5 << 20
	// Keep is the number of files kept, including the current one.
	Keep = 5
)

// File is an io.WriteCloser that rotates at maxSize, keeping keep files.
type File struct {
	path    string
	maxSize int64
	keep    int

	mu   sync.Mutex
	f    *os.File
	size int64
}

// Open opens (or creates) path for appending with the default limits.
func Open(path string) (*File, error) { return open(path, MaxSize, Keep) }

func open(path string, maxSize int64, keep int) (*File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	l := &File{path: path, maxSize: maxSize, keep: keep}
	if err := l.openCurrent(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *File) openCurrent() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("stat log: %w", err)
	}
	l.f, l.size = f, st.Size()
	return nil
}

// Write appends p, rotating first when p would push the file over the limit.
func (l *File) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.size > 0 && l.size+int64(len(p)) > l.maxSize {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := l.f.Write(p)
	l.size += int64(n)
	return n, err
}

// rotate shifts prism.log.(k) → .(k+1), dropping the oldest, and starts a new file.
func (l *File) rotate() error {
	if err := l.f.Close(); err != nil {
		return fmt.Errorf("close log: %w", err)
	}
	_ = os.Remove(fmt.Sprintf("%s.%d", l.path, l.keep-1))
	for i := l.keep - 2; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", l.path, i), fmt.Sprintf("%s.%d", l.path, i+1))
	}
	if err := os.Rename(l.path, l.path+".1"); err != nil {
		return fmt.Errorf("rotate log: %w", err)
	}
	return l.openCurrent()
}

// Close closes the current file.
func (l *File) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.Close()
}
