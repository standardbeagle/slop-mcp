// Package atomicfile provides atomic file writes via a uniquely named
// temporary file in the target directory followed by rename. Unique temp
// names (os.CreateTemp) prevent concurrent processes writing the same path
// from clobbering each other's in-flight temp files.
package atomicfile

import (
	"os"
	"path/filepath"
)

// WriteFile atomically writes data to path with the given permissions.
// The temp file is created in the same directory as path (rename is only
// atomic within a filesystem) and removed on any failure.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	// os.CreateTemp creates the file 0600; widen/keep per caller intent.
	if err := os.Chmod(tmp, perm); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
