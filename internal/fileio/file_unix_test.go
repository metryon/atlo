//go:build linux || darwin

package fileio

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestPipesAndDevicesCannotBlockValidation(t *testing.T) {
	pipe := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(pipe, 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{pipe, "/dev/null"} {
		done := make(chan error, 1)
		go func() {
			f, err := OpenRegular(path)
			if f != nil {
				f.Close()
			}
			done <- err
		}()
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("accepted special file")
			}
		case <-time.After(time.Second):
			t.Fatal("blocked opening special file")
		}
	}
	file := filepath.Join(t.TempDir(), "file")
	os.WriteFile(file, nil, 0600)
	link := file + "-link"
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	f, err := OpenRegular(link)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}
