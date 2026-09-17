// Package fileio opens regular files without allowing named pipes or devices
// to block local input validation. Symlinks to regular files are supported.
package fileio

import (
	"errors"
	"os"
)

func OpenRegular(path string) (*os.File, error) {
	f, err := open(path)
	if err != nil {
		return nil, errors.New("could not open input file")
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("input must be a regular file")
	}
	return f, nil
}
