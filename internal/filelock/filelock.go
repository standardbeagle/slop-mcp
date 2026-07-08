// Package filelock provides a cross-process advisory file lock used to make
// load-modify-write sequences on shared JSON files (memory banks, OAuth tokens)
// safe when multiple slop-mcp processes run concurrently. In-process mutexes do
// not help there: two processes each read, modify, and atomically rename,
// silently clobbering each other's changes.
//
// The lock is held on a sidecar "<path>.lock" file. The OS releases the lock
// automatically when the holding fd is closed or the process exits, so a
// crashed process cannot leave a stale lock wedged.
package filelock

// Unlocker releases a held file lock.
type Unlocker func() error
