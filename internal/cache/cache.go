package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/standardbeagle/slop-mcp/internal/config"
)

// CacheSchemaVersion is the current cache file schema version.
const CacheSchemaVersion = 1

// CachedToolInfo mirrors registry.ToolInfo for storage without import cycles.
type CachedToolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	MCPName     string         `json:"mcp_name"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

// CacheEntry holds cached tool metadata for a single MCP server.
type CacheEntry struct {
	ConfigHash    string           `json:"config_hash"`    // SHA-256 of identity fields
	ServerName    string           `json:"server_name"`    // From InitializeResult.ServerInfo.Name
	ServerVersion string           `json:"server_version"` // From InitializeResult.ServerInfo.Version
	Tools         []CachedToolInfo `json:"tools"`          // Full tool list with schemas
	CachedAt      time.Time        `json:"cached_at"`
}

// CacheFile is the on-disk structure for the tool cache.
type CacheFile struct {
	Version int                    `json:"version"` // Schema version
	Entries map[string]*CacheEntry `json:"entries"` // Keyed by MCP name
}

// Store provides thread-safe access to the disk-based tool metadata cache.
type Store struct {
	path string
	mu   sync.RWMutex
}

// NewStore creates a Store at the default location (~/.config/slop-mcp/cache/tools.json).
func NewStore() *Store {
	return &Store{path: defaultCachePath()}
}

// NewStoreWithPath creates a Store at the given path (for testing).
func NewStoreWithPath(path string) *Store {
	return &Store{path: path}
}

// defaultCachePath returns ~/.config/slop-mcp/cache/tools.json.
func defaultCachePath() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "slop-mcp", "cache", "tools.json")
}

// ConfigHash computes a deterministic hash of the identity fields of an MCPConfig.
// Only fields that affect which server we connect to are included:
// Type, Command, Args, URL, Headers, Env.
// Volatile fields (Name, Timeout, MaxRetries, etc.) are excluded.
func ConfigHash(cfg config.MCPConfig) string {
	h := sha256.New()

	// Type
	h.Write([]byte("type:"))
	h.Write([]byte(cfg.Type))
	h.Write([]byte("\n"))

	// Command
	h.Write([]byte("command:"))
	h.Write([]byte(cfg.Command))
	h.Write([]byte("\n"))

	// Args (order-sensitive)
	h.Write([]byte("args:"))
	h.Write([]byte(strings.Join(cfg.Args, "\x00")))
	h.Write([]byte("\n"))

	// URL
	h.Write([]byte("url:"))
	h.Write([]byte(cfg.URL))
	h.Write([]byte("\n"))

	// Headers (sorted keys for determinism)
	h.Write([]byte("headers:"))
	h.Write([]byte(sortedMapString(cfg.Headers)))
	h.Write([]byte("\n"))

	// Env (sorted keys for determinism)
	h.Write([]byte("env:"))
	h.Write([]byte(sortedMapString(cfg.Env)))
	h.Write([]byte("\n"))

	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// sortedMapString returns a deterministic string representation of a map.
func sortedMapString(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(m[k])
		b.WriteByte('\x00')
	}
	return b.String()
}

// Load reads the cache file from disk.
// Returns an empty CacheFile if the file doesn't exist.
func (s *Store) Load() (*CacheFile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.loadUnlocked()
}

func (s *Store) loadUnlocked() (*CacheFile, error) {
	if s.path == "" {
		return emptyCacheFile(), nil
	}

	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return emptyCacheFile(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read cache file: %w", err)
	}

	var cf CacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		// Corrupt cache: return empty rather than error
		return emptyCacheFile(), nil
	}

	// Version mismatch: discard
	if cf.Version != CacheSchemaVersion {
		return emptyCacheFile(), nil
	}

	if cf.Entries == nil {
		cf.Entries = make(map[string]*CacheEntry)
	}

	return &cf, nil
}

// Save atomically writes the cache file to disk (temp file + rename).
func (s *Store) Save(cf *CacheFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.saveUnlocked(cf)
}

func (s *Store) saveUnlocked(cf *CacheFile) error {
	if s.path == "" {
		return fmt.Errorf("cache path not configured")
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %w", err)
	}

	// Atomic write: write to temp file, then rename
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write cache temp file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp) // best-effort cleanup
		return fmt.Errorf("rename cache file: %w", err)
	}

	return nil
}

// GetEntry loads the cache and returns the entry for the given MCP name.
// Returns nil if not found.
func (s *Store) GetEntry(name string) (*CacheEntry, error) {
	cf, err := s.Load()
	if err != nil {
		return nil, err
	}
	return cf.Entries[name], nil
}

// SetEntry loads the cache, sets the entry, and saves.
func (s *Store) SetEntry(name string, entry *CacheEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cf, err := s.loadUnlocked()
	if err != nil {
		return err
	}

	cf.Entries[name] = entry
	return s.saveUnlocked(cf)
}

// IsValid checks if a cached entry exists and its ConfigHash matches the current config.
func (s *Store) IsValid(name string, cfg config.MCPConfig) bool {
	entry, err := s.GetEntry(name)
	if err != nil || entry == nil {
		return false
	}
	return entry.ConfigHash == ConfigHash(cfg)
}

func emptyCacheFile() *CacheFile {
	return &CacheFile{
		Version: CacheSchemaVersion,
		Entries: make(map[string]*CacheEntry),
	}
}
