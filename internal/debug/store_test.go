package debug

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreCreateLoadUpdateAbort(t *testing.T) {
	store := NewStore(Config{Dir: t.TempDir(), TokenTTL: time.Hour})
	session, err := store.Create([]byte(`{"status":"failed"}`))
	require.NoError(t, err)
	assert.NotContains(t, session.Token, store.cfg.Dir)
	assert.Equal(t, int64(1), session.Revision)

	loaded, err := store.Load(session.Token)
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"failed"}`, string(loaded.Data))

	next, err := store.Update(session.Token, 1, []byte(`{"status":"paused"}`))
	require.NoError(t, err)
	assert.Equal(t, int64(2), next.Revision)
	_, err = store.Load(session.Token)
	assert.ErrorIs(t, err, ErrInvalidToken)

	loaded, err = store.Load(next.Token)
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"paused"}`, string(loaded.Data))
	require.NoError(t, store.Abort(next.Token))
	_, err = store.Load(next.Token)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestStoreRejectsExpiredAndStaleRevision(t *testing.T) {
	now := time.Unix(1000, 0)
	store := NewStore(Config{Dir: t.TempDir(), TokenTTL: time.Minute, Now: func() time.Time { return now }})
	session, err := store.Create([]byte(`{}`))
	require.NoError(t, err)

	_, err = store.Update(session.Token, 2, []byte(`{}`))
	var conflict *RevisionConflictError
	require.ErrorAs(t, err, &conflict)
	assert.Equal(t, int64(1), conflict.Current)

	now = now.Add(2 * time.Minute)
	_, err = store.Load(session.Token)
	assert.ErrorIs(t, err, ErrExpiredToken)
}

func TestStoreConcurrentUpdateHasSingleWinner(t *testing.T) {
	store := NewStore(Config{Dir: t.TempDir(), TokenTTL: time.Hour})
	session, err := store.Create([]byte(`{"revision":1}`))
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Update(session.Token, 1, []byte(`{"revision":2}`))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	wins, conflicts := 0, 0
	for err := range errs {
		if err == nil {
			wins++
			continue
		}
		var conflict *RevisionConflictError
		if errors.As(err, &conflict) {
			conflicts++
			continue
		}
		t.Fatalf("unexpected error: %v", err)
	}
	assert.Equal(t, 1, wins)
	assert.Equal(t, 1, conflicts)
}

func TestStoreConcurrentClaimHasSingleWinner(t *testing.T) {
	store := NewStore(Config{Dir: t.TempDir(), TokenTTL: time.Hour})
	session, err := store.Create([]byte(`{"state":"failed"}`))
	require.NoError(t, err)

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := store.Claim(session.Token, session.Revision); results <- err }()
	}
	wg.Wait()
	close(results)
	wins, rejected := 0, 0
	for err := range results {
		if err == nil {
			wins++
		} else {
			rejected++
		}
	}
	assert.Equal(t, 1, wins)
	assert.Equal(t, 1, rejected)
}

func TestClaimRemainsInspectableAfterExecutorFailure(t *testing.T) {
	store := NewStore(Config{Dir: t.TempDir(), TokenTTL: time.Hour})
	session, err := store.Create([]byte(`{"state":"failed"}`))
	require.NoError(t, err)
	claimed, err := store.Claim(session.Token, session.Revision)
	require.NoError(t, err)
	assert.Equal(t, session.Token, claimed.Token)

	recovered, err := store.Load(session.Token)
	require.NoError(t, err)
	assert.Equal(t, claimed.Revision, recovered.Revision)
	assert.JSONEq(t, string(session.Data), string(recovered.Data))
}

func TestStoreFilesAreOwnerOnlyAndTokenIsNotStored(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(Config{Dir: dir, TokenTTL: time.Hour})
	session, err := store.Create([]byte(`{"secret":"state"}`))
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	familyDir := filepath.Join(dir, entries[0].Name())
	info, err := os.Stat(familyDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())

	err = filepath.Walk(familyDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		assert.False(t, bytes.Contains(data, []byte(session.Token)))
		return nil
	})
	require.NoError(t, err)
}
