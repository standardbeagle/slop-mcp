// Package atomicfile provides atomic, crash-durable file writes via a uniquely
// named temporary file in the target directory: write, fsync the file, rename,
// then fsync the directory. Unique temp names (os.CreateTemp) prevent concurrent
// processes writing the same path from clobbering each other's in-flight temp
// files; the fsyncs make the result durable against power loss, not just atomic
// against concurrent readers.
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
	// Flush file data to disk before the rename so a crash cannot leave the
	// renamed path pointing at unwritten/empty blocks (the classic ext4
	// truncate-on-crash pattern). Without this the rename is atomic only
	// against concurrent readers, not against power loss.
	if err := f.Sync(); err != nil {
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
	// Fsync the directory so the rename itself is durable (the directory entry
	// change is otherwise only in the page cache). Best-effort: some platforms
	// disallow opening/syncing a directory, so a failure here is not fatal.
	if dh, derr := os.Open(dir); derr == nil {
		_ = dh.Sync()
		dh.Close()
	}
	return nil
}
