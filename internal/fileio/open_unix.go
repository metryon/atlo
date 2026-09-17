//go:build linux || darwin

package fileio

import (
	"os"
	"syscall"
)

func open(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}
