package fileio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegularFileAndDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input")
	if err := os.WriteFile(path, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := OpenRegular(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	for _, path := range []string{dir, filepath.Join(dir, "missing")} {
		if f, err := OpenRegular(path); err == nil {
			f.Close()
			t.Fatal("accepted non-file")
		}
	}
}
