package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prism.log")
	l, err := open(path, 100, 3)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Repeat("x", 39) + "\n" // 40 bytes
	for i := range 12 {
		if _, err := fmt.Fprint(l, line[:len(line)-3]+fmt.Sprintf("%02d\n", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	files, _ := filepath.Glob(path + "*")
	if len(files) != 3 {
		t.Fatalf("files = %v, want 3 (current + 2 rotated)", files)
	}
	for _, f := range files {
		if st, _ := os.Stat(f); st.Size() > 100 {
			t.Errorf("%s is %d bytes, over the limit", f, st.Size())
		}
	}
	cur, _ := os.ReadFile(path)
	if !strings.HasSuffix(string(cur), "11\n") {
		t.Errorf("newest line not in the current file: %q", cur)
	}
	old, _ := os.ReadFile(path + ".2")
	if strings.Contains(string(old), "00\n") {
		t.Error("oldest lines must have been dropped")
	}

	// Reopening appends to the existing file and keeps counting its size.
	l, err = open(path, 100, 3)
	if err != nil {
		t.Fatal(err)
	}
	if l.size != int64(len(cur)) {
		t.Errorf("size after reopen = %d, want %d", l.size, len(cur))
	}
	_ = l.Close()
}
