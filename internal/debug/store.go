package debug

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/standardbeagle/slop-mcp/internal/atomicfile"
	"github.com/standardbeagle/slop-mcp/internal/filelock"
)

var (
	ErrInvalidToken = errors.New("invalid debug token")
	ErrExpiredToken = errors.New("debug token expired")
)

type RevisionConflictError struct{ Expected, Current int64 }

func (e *RevisionConflictError) Error() string {
	return fmt.Sprintf("debug revision conflict: expected %d, current %d", e.Expected, e.Current)
}

type Config struct {
	Dir      string
	TokenTTL time.Duration
	Now      func() time.Time
}

type Store struct{ cfg Config }

type Session struct {
	Token     string    `json:"token"`
	Revision  int64     `json:"revision"`
	ExpiresAt time.Time `json:"expires_at"`
	Data      []byte    `json:"-"`
}

type manifest struct {
	Family    string    `json:"family"`
	Revision  int64     `json:"revision"`
	TokenHash string    `json:"token_hash"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewStore(cfg Config) *Store {
	if cfg.TokenTTL <= 0 {
		cfg.TokenTTL = 24 * time.Hour
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Store{cfg: cfg}
}

func (s *Store) Create(data []byte) (Session, error) {
	familyBytes, err := randomBytes(16)
	if err != nil {
		return Session{}, err
	}
	secret, err := randomBytes(32)
	if err != nil {
		return Session{}, err
	}
	family := hex.EncodeToString(familyBytes)
	token := encodeToken(familyBytes, secret)
	dir := s.familyDir(family)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Session{}, fmt.Errorf("create debug family: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return Session{}, err
	}
	expires := s.cfg.Now().Add(s.cfg.TokenTTL)
	m := manifest{Family: family, Revision: 1, TokenHash: hashSecret(secret), ExpiresAt: expires}
	if err := s.writeRevision(dir, 1, data); err != nil {
		return Session{}, err
	}
	if err := writeJSON(filepath.Join(dir, "manifest.json"), m); err != nil {
		return Session{}, err
	}
	return Session{Token: token, Revision: 1, ExpiresAt: expires, Data: clone(data)}, nil
}

func (s *Store) Load(token string) (Session, error) {
	family, secret, err := decodeToken(token)
	if err != nil {
		return Session{}, ErrInvalidToken
	}
	dir := s.familyDir(family)
	unlock, err := filelock.Lock(s.lockPath(family))
	if err != nil {
		return Session{}, err
	}
	defer func() { _ = unlock() }()
	m, err := s.readAuthorized(dir, secret)
	if err != nil {
		return Session{}, err
	}
	data, err := os.ReadFile(revisionPath(dir, m.Revision))
	if err != nil {
		return Session{}, fmt.Errorf("read debug revision: %w", err)
	}
	return Session{Token: token, Revision: m.Revision, ExpiresAt: m.ExpiresAt, Data: data}, nil
}

// Claim atomically consumes a token/revision before a resume can perform
// external effects. The returned token exclusively owns the claimed revision.
func (s *Store) Claim(token string, expected int64) (Session, error) {
	family, secret, err := decodeToken(token)
	if err != nil {
		return Session{}, ErrInvalidToken
	}
	dir := s.familyDir(family)
	unlock, err := filelock.Lock(s.lockPath(family))
	if err != nil {
		return Session{}, err
	}
	defer func() { _ = unlock() }()
	m, err := s.readAuthorized(dir, secret)
	if err != nil {
		return Session{}, err
	}
	if m.Revision != expected {
		return Session{}, &RevisionConflictError{Expected: expected, Current: m.Revision}
	}
	data, err := os.ReadFile(revisionPath(dir, m.Revision))
	if err != nil {
		return Session{}, err
	}
	m.Revision++
	m.ExpiresAt = s.cfg.Now().Add(s.cfg.TokenTTL)
	if err := s.writeRevision(dir, m.Revision, data); err != nil {
		return Session{}, err
	}
	if err := writeJSON(filepath.Join(dir, "manifest.json"), m); err != nil {
		return Session{}, err
	}
	return Session{Token: token, Revision: m.Revision, ExpiresAt: m.ExpiresAt, Data: data}, nil
}

func (s *Store) Update(token string, expected int64, data []byte) (Session, error) {
	family, secret, err := decodeToken(token)
	if err != nil {
		return Session{}, ErrInvalidToken
	}
	dir := s.familyDir(family)
	unlock, err := filelock.Lock(s.lockPath(family))
	if err != nil {
		return Session{}, err
	}
	defer func() { _ = unlock() }()
	m, err := s.readManifest(dir)
	if err != nil {
		return Session{}, ErrInvalidToken
	}
	if m.Revision != expected {
		return Session{}, &RevisionConflictError{Expected: expected, Current: m.Revision}
	}
	if err := s.authorize(m, secret); err != nil {
		return Session{}, err
	}
	newSecret, err := randomBytes(32)
	if err != nil {
		return Session{}, err
	}
	next := m.Revision + 1
	if err := s.writeRevision(dir, next, data); err != nil {
		return Session{}, err
	}
	m.Revision, m.TokenHash, m.ExpiresAt = next, hashSecret(newSecret), s.cfg.Now().Add(s.cfg.TokenTTL)
	if err := writeJSON(filepath.Join(dir, "manifest.json"), m); err != nil {
		return Session{}, err
	}
	familyBytes, _ := hex.DecodeString(family)
	return Session{Token: encodeToken(familyBytes, newSecret), Revision: next, ExpiresAt: m.ExpiresAt, Data: clone(data)}, nil
}

func (s *Store) Abort(token string) error {
	family, secret, err := decodeToken(token)
	if err != nil {
		return ErrInvalidToken
	}
	dir := s.familyDir(family)
	unlock, err := filelock.Lock(s.lockPath(family))
	if err != nil {
		return err
	}
	if _, err := s.readAuthorized(dir, secret); err != nil {
		_ = unlock()
		return err
	}
	removeErr := os.RemoveAll(dir)
	unlockErr := unlock()
	if removeErr != nil {
		return removeErr
	}
	return unlockErr
}

func (s *Store) readAuthorized(dir string, secret []byte) (manifest, error) {
	m, err := s.readManifest(dir)
	if err != nil {
		return manifest{}, ErrInvalidToken
	}
	if err := s.authorize(m, secret); err != nil {
		return manifest{}, err
	}
	if m.Family != filepath.Base(dir) {
		return manifest{}, ErrInvalidToken
	}
	return m, nil
}

func (s *Store) readManifest(dir string) (manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return manifest{}, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return manifest{}, err
	}
	return m, nil
}

func (s *Store) authorize(m manifest, secret []byte) error {
	want, err := hex.DecodeString(m.TokenHash)
	if err != nil {
		return ErrInvalidToken
	}
	got := sha256.Sum256(secret)
	if len(want) != len(got) || subtle.ConstantTimeCompare(want, got[:]) != 1 {
		return ErrInvalidToken
	}
	if !s.cfg.Now().Before(m.ExpiresAt) {
		return ErrExpiredToken
	}
	return nil
}

func (s *Store) familyDir(family string) string { return filepath.Join(s.cfg.Dir, family) }
func (s *Store) lockPath(family string) string  { return filepath.Join(s.cfg.Dir, family) }
func (s *Store) writeRevision(dir string, revision int64, data []byte) error {
	return atomicfile.WriteFile(revisionPath(dir, revision), data, 0o600)
}
func revisionPath(dir string, revision int64) string {
	return filepath.Join(dir, fmt.Sprintf("revision-%06d.json", revision))
}

func randomBytes(n int) ([]byte, error) { b := make([]byte, n); _, err := rand.Read(b); return b, err }
func hashSecret(secret []byte) string {
	sum := sha256.Sum256(secret)
	return hex.EncodeToString(sum[:])
}
func encodeToken(family, secret []byte) string {
	return base64.RawURLEncoding.EncodeToString(append(clone(family), secret...))
}
func decodeToken(token string) (string, []byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(b) != 48 {
		return "", nil, ErrInvalidToken
	}
	return hex.EncodeToString(b[:16]), clone(b[16:]), nil
}
func clone(b []byte) []byte { return append([]byte(nil), b...) }
func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(path, data, 0o600)
}
