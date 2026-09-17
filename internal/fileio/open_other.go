//go:build !linux && !darwin

package fileio

import (
	"errors"
	"os"
)

func open(path string) (*os.File, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("input must be a regular file")
	}
	return os.Open(path)
}
