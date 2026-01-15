package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var Version = "0.1.0"

// Bank represents a memory bank structure.
type Bank struct {
	Meta    BankMeta          `json:"_meta"`
	Entries map[string]*Entry `json:"entries"`
}

// BankMeta contains metadata about the bank.
type BankMeta struct {
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Entry represents a single memory entry.
type Entry struct {
	Value     any       `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	TTL       *int64    `json:"ttl,omitempty"` // seconds, nil = no expiry
}

// IsExpired checks if the entry has expired.
func (e *Entry) IsExpired() bool {
	if e.TTL == nil {
		return false
	}
	expiresAt := e.UpdatedAt.Add(time.Duration(*e.TTL) * time.Second)
	return time.Now().After(expiresAt)
}

// Result types for JSON output.
type ReadResult struct {
	Value    any           `json:"value,omitempty"`
	Metadata *EntryMeta    `json:"metadata,omitempty"`
	Bank     *BankSummary  `json:"bank,omitempty"`
	Error    *ErrorResult  `json:"error,omitempty"`
}

type EntryMeta struct {
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	TTL       *int64 `json:"ttl,omitempty"`
}

type BankSummary struct {
	Name      string   `json:"name"`
	Keys      []string `json:"keys"`
	KeyCount  int      `json:"key_count"`
	UpdatedAt string   `json:"updated_at"`
}

type WriteResult struct {
	Success   bool   `json:"success"`
	Key       string `json:"key"`
	Bank      string `json:"bank"`
	Operation string `json:"operation"` // "create" or "update"
}

type ListResult struct {
	Banks []BankInfo `json:"banks,omitempty"`
	Keys  []KeyInfo  `json:"keys,omitempty"`
}

type BankInfo struct {
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	KeyCount  int    `json:"key_count"`
	UpdatedAt string `json:"updated_at"`
	SizeBytes int64  `json:"size_bytes"`
}

type KeyInfo struct {
	Key       string `json:"key"`
	UpdatedAt string `json:"updated_at"`
	TTL       *int64 `json:"ttl,omitempty"`
}

type SearchResult struct {
	Matches []SearchMatch `json:"matches"`
	Total   int           `json:"total"`
}

type SearchMatch struct {
	Bank  string `json:"bank"`
	Key   string `json:"key"`
	Scope string `json:"scope"`
	Value any    `json:"value,omitempty"`
}

type ErrorResult struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Bank    string `json:"bank,omitempty"`
	Key     string `json:"key,omitempty"`
	Scope   string `json:"scope,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "read":
		err = cmdRead(args)
	case "write":
		err = cmdWrite(args)
	case "delete":
		err = cmdDelete(args)
	case "list":
		err = cmdList(args)
	case "query":
		err = cmdQuery(args)
	case "search":
		err = cmdSearch(args)
	case "version":
		fmt.Printf("memory-cli version %s\n", Version)
	case "help", "-h", "--help":
		printUsage()
	default:
		printError("UNKNOWN_COMMAND", fmt.Sprintf("Unknown command: %s", cmd), "", "", "")
		os.Exit(1)
	}

	if err != nil {
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`memory-cli - Structured file-based memory banks

Usage: memory-cli <command> [arguments]

Commands:
  read   <bank> [key]           Read from a memory bank
  write  <bank> <key> <value>   Write to a memory bank
  delete <bank> [key]           Delete entry or bank
  list   [bank]                 List banks or keys
  query  <bank> [key] <filter>  Query with jq-style filter (basic)
  search <pattern>              Search across banks
  version                       Show version
  help                          Show this help

Flags:
  --scope <user|project>   Storage scope (default: auto-detect)
  --default <json>         Default value if key not found (read)
  --ttl <seconds>          Time-to-live for entry (write)
  --stdin                  Read value from stdin (write)
  --raw                    Raw string output
  --verbose                Include metadata

Storage Locations:
  User:    ~/.config/slop-mcp/memory/<bank>.json
  Project: .slop-mcp/memory/<bank>.json

Examples:
  memory-cli write session context '{"topic": "testing"}'
  memory-cli read session context
  memory-cli list
  memory-cli delete session context`)
}

func printError(code, message, bank, key, scope string) {
	result := ReadResult{
		Error: &ErrorResult{
			Code:    code,
			Message: message,
			Bank:    bank,
			Key:     key,
			Scope:   scope,
		},
	}
	output, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(output))
}

func printJSON(v any) {
	output, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(output))
}

// Storage paths
func getUserMemoryDir() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "slop-mcp", "memory")
}

func getProjectMemoryDir() string {
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, ".slop-mcp", "memory")
}

func getBankPath(bankName, scope string) string {
	var dir string
	switch scope {
	case "user":
		dir = getUserMemoryDir()
	case "project":
		dir = getProjectMemoryDir()
	default:
		// Auto-detect: prefer project if exists
		projectDir := getProjectMemoryDir()
		projectPath := filepath.Join(projectDir, bankName+".json")
		if _, err := os.Stat(projectPath); err == nil {
			return projectPath
		}
		// Check user
		userDir := getUserMemoryDir()
		userPath := filepath.Join(userDir, bankName+".json")
		if _, err := os.Stat(userPath); err == nil {
			return userPath
		}
		// Default to project for new banks
		return filepath.Join(projectDir, bankName+".json")
	}
	return filepath.Join(dir, bankName+".json")
}

func loadBank(path string) (*Bank, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var bank Bank
	if err := json.Unmarshal(data, &bank); err != nil {
		return nil, err
	}

	return &bank, nil
}

func saveBank(path string, bank *Bank) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Update metadata
	bank.Meta.UpdatedAt = time.Now()

	// Marshal and write atomically
	data, err := json.MarshalIndent(bank, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file first, then rename (atomic)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

func newBank() *Bank {
	now := time.Now()
	return &Bank{
		Meta: BankMeta{
			Version:   1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Entries: make(map[string]*Entry),
	}
}

// Validate bank name
var bankNameRegex = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

func validateBankName(name string) error {
	if len(name) > 64 {
		return fmt.Errorf("bank name too long (max 64 characters)")
	}
	if !bankNameRegex.MatchString(name) {
		return fmt.Errorf("invalid bank name: must start with lowercase letter and contain only lowercase letters, numbers, hyphens, and underscores")
	}
	if strings.HasPrefix(name, "_") {
		return fmt.Errorf("bank names starting with underscore are reserved")
	}
	return nil
}

// Parse flags
func parseFlags(args []string) (positional []string, flags map[string]string) {
	flags = make(map[string]string)
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			key := strings.TrimPrefix(args[i], "--")
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				flags[key] = args[i+1]
				i++
			} else {
				flags[key] = "true"
			}
		} else {
			positional = append(positional, args[i])
		}
	}
	return
}

// Commands
func cmdRead(args []string) error {
	positional, flags := parseFlags(args)

	if len(positional) < 1 {
		printError("INVALID_ARGS", "Usage: memory-cli read <bank> [key]", "", "", "")
		return fmt.Errorf("invalid args")
	}

	bankName := positional[0]
	if err := validateBankName(bankName); err != nil {
		printError("INVALID_BANK_NAME", err.Error(), bankName, "", "")
		return err
	}

	scope := flags["scope"]
	path := getBankPath(bankName, scope)

	bank, err := loadBank(path)
	if err != nil {
		printError("LOAD_ERROR", err.Error(), bankName, "", scope)
		return err
	}

	if bank == nil {
		// Check for default
		if defaultVal, ok := flags["default"]; ok {
			var v any
			json.Unmarshal([]byte(defaultVal), &v)
			printJSON(ReadResult{Value: v})
			return nil
		}
		printError("BANK_NOT_FOUND", fmt.Sprintf("Memory bank '%s' does not exist", bankName), bankName, "", scope)
		return fmt.Errorf("bank not found")
	}

	// If no key specified, return entire bank
	if len(positional) < 2 {
		keys := make([]string, 0, len(bank.Entries))
		for k, e := range bank.Entries {
			if !e.IsExpired() {
				keys = append(keys, k)
			}
		}
		printJSON(ReadResult{
			Bank: &BankSummary{
				Name:      bankName,
				Keys:      keys,
				KeyCount:  len(keys),
				UpdatedAt: bank.Meta.UpdatedAt.Format(time.RFC3339),
			},
		})
		return nil
	}

	key := positional[1]
	entry, ok := bank.Entries[key]
	if !ok || entry.IsExpired() {
		if defaultVal, ok := flags["default"]; ok {
			var v any
			json.Unmarshal([]byte(defaultVal), &v)
			printJSON(ReadResult{Value: v})
			return nil
		}
		printError("KEY_NOT_FOUND", fmt.Sprintf("Key '%s' not found in bank '%s'", key, bankName), bankName, key, scope)
		return fmt.Errorf("key not found")
	}

	result := ReadResult{
		Value: entry.Value,
	}

	if flags["verbose"] == "true" {
		result.Metadata = &EntryMeta{
			CreatedAt: entry.CreatedAt.Format(time.RFC3339),
			UpdatedAt: entry.UpdatedAt.Format(time.RFC3339),
			TTL:       entry.TTL,
		}
	}

	printJSON(result)
	return nil
}

func cmdWrite(args []string) error {
	positional, flags := parseFlags(args)

	if len(positional) < 2 {
		printError("INVALID_ARGS", "Usage: memory-cli write <bank> <key> <value>", "", "", "")
		return fmt.Errorf("invalid args")
	}

	bankName := positional[0]
	key := positional[1]

	if err := validateBankName(bankName); err != nil {
		printError("INVALID_BANK_NAME", err.Error(), bankName, "", "")
		return err
	}

	// Get value from positional or stdin
	var valueStr string
	if flags["stdin"] == "true" {
		data, err := os.ReadFile("/dev/stdin")
		if err != nil {
			printError("STDIN_ERROR", err.Error(), bankName, key, "")
			return err
		}
		valueStr = string(data)
	} else if len(positional) >= 3 {
		valueStr = positional[2]
	} else {
		printError("INVALID_ARGS", "Value required: provide as argument or use --stdin", bankName, key, "")
		return fmt.Errorf("value required")
	}

	// Parse value as JSON
	var value any
	if err := json.Unmarshal([]byte(valueStr), &value); err != nil {
		// Treat as raw string
		value = valueStr
	}

	scope := flags["scope"]
	if scope == "" {
		// Default to project if .slop-mcp exists, else user
		projectDir := getProjectMemoryDir()
		if _, err := os.Stat(filepath.Dir(projectDir)); err == nil {
			scope = "project"
		} else {
			scope = "user"
		}
	}

	path := getBankPath(bankName, scope)

	bank, err := loadBank(path)
	if err != nil {
		printError("LOAD_ERROR", err.Error(), bankName, key, scope)
		return err
	}

	operation := "update"
	if bank == nil {
		bank = newBank()
		operation = "create"
	}

	now := time.Now()
	entry := &Entry{
		Value:     value,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Preserve created_at if updating
	if existing, ok := bank.Entries[key]; ok {
		entry.CreatedAt = existing.CreatedAt
	} else {
		operation = "create"
	}

	// Handle TTL
	if ttlStr, ok := flags["ttl"]; ok {
		var ttl int64
		fmt.Sscanf(ttlStr, "%d", &ttl)
		entry.TTL = &ttl
	}

	bank.Entries[key] = entry

	if err := saveBank(path, bank); err != nil {
		printError("SAVE_ERROR", err.Error(), bankName, key, scope)
		return err
	}

	printJSON(WriteResult{
		Success:   true,
		Key:       key,
		Bank:      bankName,
		Operation: operation,
	})
	return nil
}

func cmdDelete(args []string) error {
	positional, flags := parseFlags(args)

	if len(positional) < 1 {
		printError("INVALID_ARGS", "Usage: memory-cli delete <bank> [key]", "", "", "")
		return fmt.Errorf("invalid args")
	}

	bankName := positional[0]
	if err := validateBankName(bankName); err != nil {
		printError("INVALID_BANK_NAME", err.Error(), bankName, "", "")
		return err
	}

	scope := flags["scope"]
	path := getBankPath(bankName, scope)

	// Delete entire bank
	if len(positional) < 2 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			printError("DELETE_ERROR", err.Error(), bankName, "", scope)
			return err
		}
		printJSON(map[string]any{
			"success": true,
			"bank":    bankName,
			"deleted": "bank",
		})
		return nil
	}

	// Delete specific key
	key := positional[1]

	bank, err := loadBank(path)
	if err != nil {
		printError("LOAD_ERROR", err.Error(), bankName, key, scope)
		return err
	}

	if bank == nil {
		printError("BANK_NOT_FOUND", fmt.Sprintf("Memory bank '%s' does not exist", bankName), bankName, key, scope)
		return fmt.Errorf("bank not found")
	}

	if _, ok := bank.Entries[key]; !ok {
		printError("KEY_NOT_FOUND", fmt.Sprintf("Key '%s' not found in bank '%s'", key, bankName), bankName, key, scope)
		return fmt.Errorf("key not found")
	}

	delete(bank.Entries, key)

	if err := saveBank(path, bank); err != nil {
		printError("SAVE_ERROR", err.Error(), bankName, key, scope)
		return err
	}

	printJSON(map[string]any{
		"success": true,
		"bank":    bankName,
		"key":     key,
		"deleted": "key",
	})
	return nil
}

func cmdList(args []string) error {
	positional, flags := parseFlags(args)

	scope := flags["scope"]

	// List keys in specific bank
	if len(positional) > 0 {
		bankName := positional[0]
		if err := validateBankName(bankName); err != nil {
			printError("INVALID_BANK_NAME", err.Error(), bankName, "", "")
			return err
		}

		path := getBankPath(bankName, scope)
		bank, err := loadBank(path)
		if err != nil {
			printError("LOAD_ERROR", err.Error(), bankName, "", scope)
			return err
		}

		if bank == nil {
			printError("BANK_NOT_FOUND", fmt.Sprintf("Memory bank '%s' does not exist", bankName), bankName, "", scope)
			return fmt.Errorf("bank not found")
		}

		keys := make([]KeyInfo, 0, len(bank.Entries))
		for k, e := range bank.Entries {
			if !e.IsExpired() {
				keys = append(keys, KeyInfo{
					Key:       k,
					UpdatedAt: e.UpdatedAt.Format(time.RFC3339),
					TTL:       e.TTL,
				})
			}
		}

		printJSON(ListResult{Keys: keys})
		return nil
	}

	// List all banks
	banks := make([]BankInfo, 0)

	// Scan user directory
	if scope == "" || scope == "user" || scope == "all" {
		userDir := getUserMemoryDir()
		entries, _ := os.ReadDir(userDir)
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".json")
			path := filepath.Join(userDir, entry.Name())
			info, _ := entry.Info()
			bank, _ := loadBank(path)

			keyCount := 0
			updatedAt := ""
			if bank != nil {
				for _, e := range bank.Entries {
					if !e.IsExpired() {
						keyCount++
					}
				}
				updatedAt = bank.Meta.UpdatedAt.Format(time.RFC3339)
			}

			var size int64
			if info != nil {
				size = info.Size()
			}

			banks = append(banks, BankInfo{
				Name:      name,
				Scope:     "user",
				KeyCount:  keyCount,
				UpdatedAt: updatedAt,
				SizeBytes: size,
			})
		}
	}

	// Scan project directory
	if scope == "" || scope == "project" || scope == "all" {
		projectDir := getProjectMemoryDir()
		entries, _ := os.ReadDir(projectDir)
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".json")
			path := filepath.Join(projectDir, entry.Name())
			info, _ := entry.Info()
			bank, _ := loadBank(path)

			keyCount := 0
			updatedAt := ""
			if bank != nil {
				for _, e := range bank.Entries {
					if !e.IsExpired() {
						keyCount++
					}
				}
				updatedAt = bank.Meta.UpdatedAt.Format(time.RFC3339)
			}

			var size int64
			if info != nil {
				size = info.Size()
			}

			banks = append(banks, BankInfo{
				Name:      name,
				Scope:     "project",
				KeyCount:  keyCount,
				UpdatedAt: updatedAt,
				SizeBytes: size,
			})
		}
	}

	printJSON(ListResult{Banks: banks})
	return nil
}

func cmdQuery(args []string) error {
	positional, flags := parseFlags(args)

	if len(positional) < 2 {
		printError("INVALID_ARGS", "Usage: memory-cli query <bank> [key] <filter>", "", "", "")
		return fmt.Errorf("invalid args")
	}

	bankName := positional[0]
	if err := validateBankName(bankName); err != nil {
		printError("INVALID_BANK_NAME", err.Error(), bankName, "", "")
		return err
	}

	scope := flags["scope"]
	path := getBankPath(bankName, scope)

	bank, err := loadBank(path)
	if err != nil {
		printError("LOAD_ERROR", err.Error(), bankName, "", scope)
		return err
	}

	if bank == nil {
		printError("BANK_NOT_FOUND", fmt.Sprintf("Memory bank '%s' does not exist", bankName), bankName, "", scope)
		return fmt.Errorf("bank not found")
	}

	// Simple query support (basic jq-like)
	var data any
	var filter string

	if len(positional) == 2 {
		// Query entire bank
		entries := make(map[string]any)
		for k, e := range bank.Entries {
			if !e.IsExpired() {
				entries[k] = e.Value
			}
		}
		data = entries
		filter = positional[1]
	} else {
		// Query specific key
		key := positional[1]
		entry, ok := bank.Entries[key]
		if !ok || entry.IsExpired() {
			printError("KEY_NOT_FOUND", fmt.Sprintf("Key '%s' not found in bank '%s'", key, bankName), bankName, key, scope)
			return fmt.Errorf("key not found")
		}
		data = entry.Value
		filter = positional[2]
	}

	// Apply simple filter (basic jq subset)
	result, err := applyFilter(data, filter)
	if err != nil {
		printError("FILTER_ERROR", err.Error(), bankName, "", scope)
		return err
	}

	if flags["raw"] == "true" {
		if s, ok := result.(string); ok {
			fmt.Println(s)
			return nil
		}
	}

	printJSON(result)
	return nil
}

// Very basic jq-like filter support
func applyFilter(data any, filter string) (any, error) {
	filter = strings.TrimSpace(filter)

	// Identity
	if filter == "." {
		return data, nil
	}

	// Simple property access: .foo or .foo.bar
	if strings.HasPrefix(filter, ".") {
		parts := strings.Split(strings.TrimPrefix(filter, "."), ".")
		current := data

		for _, part := range parts {
			if part == "" {
				continue
			}

			// Handle array index: [0]
			if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
				indexStr := strings.TrimPrefix(strings.TrimSuffix(part, "]"), "[")
				var index int
				fmt.Sscanf(indexStr, "%d", &index)

				arr, ok := current.([]any)
				if !ok {
					return nil, fmt.Errorf("cannot index non-array")
				}
				if index < 0 || index >= len(arr) {
					return nil, fmt.Errorf("index out of bounds")
				}
				current = arr[index]
				continue
			}

			// Handle object property
			obj, ok := current.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("cannot access property '%s' on non-object", part)
			}

			val, ok := obj[part]
			if !ok {
				return nil, nil // Return null for missing properties
			}
			current = val
		}

		return current, nil
	}

	// Keys
	if filter == "keys" {
		obj, ok := data.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("keys requires object")
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		return keys, nil
	}

	return nil, fmt.Errorf("unsupported filter: %s", filter)
}

func cmdSearch(args []string) error {
	positional, flags := parseFlags(args)

	if len(positional) < 1 {
		printError("INVALID_ARGS", "Usage: memory-cli search <pattern>", "", "", "")
		return fmt.Errorf("invalid args")
	}

	pattern := strings.ToLower(positional[0])
	scope := flags["scope"]
	searchValues := flags["value"] == "true"

	matches := make([]SearchMatch, 0)

	// Search function
	searchBank := func(bankPath, bankName, bankScope string) {
		bank, err := loadBank(bankPath)
		if err != nil || bank == nil {
			return
		}

		for key, entry := range bank.Entries {
			if entry.IsExpired() {
				continue
			}

			// Match key name
			if strings.Contains(strings.ToLower(key), pattern) {
				match := SearchMatch{
					Bank:  bankName,
					Key:   key,
					Scope: bankScope,
				}
				if searchValues {
					match.Value = entry.Value
				}
				matches = append(matches, match)
				continue
			}

			// Match value if requested
			if searchValues {
				valueJSON, _ := json.Marshal(entry.Value)
				if strings.Contains(strings.ToLower(string(valueJSON)), pattern) {
					matches = append(matches, SearchMatch{
						Bank:  bankName,
						Key:   key,
						Scope: bankScope,
						Value: entry.Value,
					})
				}
			}
		}
	}

	// Search user banks
	if scope == "" || scope == "user" || scope == "all" {
		userDir := getUserMemoryDir()
		entries, _ := os.ReadDir(userDir)
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".json")
			searchBank(filepath.Join(userDir, entry.Name()), name, "user")
		}
	}

	// Search project banks
	if scope == "" || scope == "project" || scope == "all" {
		projectDir := getProjectMemoryDir()
		entries, _ := os.ReadDir(projectDir)
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".json")
			searchBank(filepath.Join(projectDir, entry.Name()), name, "project")
		}
	}

	printJSON(SearchResult{
		Matches: matches,
		Total:   len(matches),
	})
	return nil
}
