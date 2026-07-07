package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/standardbeagle/slop-mcp/internal/atomicfile"
	"github.com/standardbeagle/slop-mcp/internal/builtins"
	"github.com/standardbeagle/slop-mcp/internal/overrides"
)

var Version = "0.1.0"

var getwd = os.Getwd

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
	Value       any       `json:"value"`
	Description string    `json:"description,omitempty"`
	Schema      any       `json:"schema,omitempty"`
	Size        int       `json:"size,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	TTL         *int64    `json:"ttl,omitempty"` // seconds, nil = no expiry
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
	Value    any          `json:"value,omitempty"`
	Metadata *EntryMeta   `json:"metadata,omitempty"`
	Bank     *BankSummary `json:"bank,omitempty"`
	Error    *ErrorResult `json:"error,omitempty"`
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
	Key         string `json:"key"`
	Description string `json:"description,omitempty"`
	Size        int    `json:"size,omitempty"`
	UpdatedAt   string `json:"updated_at"`
	TTL         *int64 `json:"ttl,omitempty"`
}

type SearchResult struct {
	Matches []SearchMatch `json:"matches"`
	Total   int           `json:"total"`
}

type SearchMatch struct {
	Bank        string `json:"bank"`
	Key         string `json:"key"`
	Description string `json:"description,omitempty"`
	Scope       string `json:"scope"`
	Value       any    `json:"value,omitempty"`
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
  User:    $XDG_CONFIG_HOME/slop-mcp/memory/<bank>.json or ~/.config/slop-mcp/memory/<bank>.json
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

func getProjectMemoryDir() (string, error) {
	cwd, err := getwd()
	if err != nil {
		return "", fmt.Errorf("getting current directory: %w", err)
	}
	return filepath.Join(cwd, ".slop-mcp", "memory"), nil
}

func getBankPath(bankName, scope string) (string, error) {
	var dir string
	switch scope {
	case "user":
		dir = getUserMemoryDir()
	case "project":
		projectDir, err := getProjectMemoryDir()
		if err != nil {
			return "", err
		}
		dir = projectDir
	default:
		// Auto-detect: prefer project if exists
		projectDir, err := getProjectMemoryDir()
		if err != nil {
			return "", err
		}
		projectPath := filepath.Join(projectDir, bankName+".json")
		if _, err := os.Stat(projectPath); err == nil {
			return projectPath, nil
		}
		// Check user
		userDir := getUserMemoryDir()
		userPath := filepath.Join(userDir, bankName+".json")
		if _, err := os.Stat(userPath); err == nil {
			return userPath, nil
		}
		// Default to project for new banks
		return filepath.Join(projectDir, bankName+".json"), nil
	}
	return filepath.Join(dir, bankName+".json"), nil
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

	// Atomic write (unique temp file + rename)
	return atomicfile.WriteFile(path, data, 0644)
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

// validateBankName delegates to the shared bank name validation used by the
// SLOP mem_* builtins so both entry points enforce identical rules.
func validateBankName(name string) error {
	return builtins.ValidateBankName(name)
}

func validateMemoryScope(scope string, allowAll bool) error {
	switch scope {
	case "", "user", "project":
		return nil
	case "all":
		if allowAll {
			return nil
		}
	}
	if allowAll {
		return fmt.Errorf("invalid scope %q: expected user, project, or all", scope)
	}
	return fmt.Errorf("invalid scope %q: expected user or project", scope)
}

// Parse flags
func parseFlags(args []string, valueFlags, boolFlags []string) (positional []string, flags map[string]string, err error) {
	flags = make(map[string]string)
	valueFlagSet := stringSet(valueFlags)
	boolFlagSet := stringSet(boolFlags)

	for i := 0; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "--") {
			positional = append(positional, args[i])
			continue
		}

		raw := strings.TrimPrefix(args[i], "--")
		key, value, hasValue := strings.Cut(raw, "=")
		if key == "" {
			return nil, nil, fmt.Errorf("invalid flag %q", args[i])
		}

		if _, ok := boolFlagSet[key]; ok {
			if !hasValue {
				flags[key] = "true"
				continue
			}
			if value != "true" && value != "false" {
				return nil, nil, fmt.Errorf("--%s expects true or false", key)
			}
			flags[key] = value
			continue
		}

		if _, ok := valueFlagSet[key]; ok {
			if hasValue {
				flags[key] = value
				continue
			}
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return nil, nil, fmt.Errorf("--%s requires a value", key)
			}
			flags[key] = args[i+1]
			i++
			continue
		}

		return nil, nil, fmt.Errorf("unknown flag --%s", key)
	}
	return positional, flags, nil
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func parseReadFlags(args []string) ([]string, map[string]string, error) {
	return parseFlags(args, []string{"scope", "default"}, []string{"verbose"})
}

func parseWriteFlags(args []string) ([]string, map[string]string, error) {
	return parseFlags(args, []string{"scope", "ttl"}, []string{"stdin"})
}

func parseScopeOnlyFlags(args []string) ([]string, map[string]string, error) {
	return parseFlags(args, []string{"scope"}, nil)
}

func parseQueryFlags(args []string) ([]string, map[string]string, error) {
	return parseFlags(args, []string{"scope"}, []string{"raw"})
}

func parseSearchFlags(args []string) ([]string, map[string]string, error) {
	return parseFlags(args, []string{"scope"}, []string{"value"})
}

func parseOrPrintFlagError(args []string, parser func([]string) ([]string, map[string]string, error)) ([]string, map[string]string, error) {
	positional, flags, err := parser(args)
	if err != nil {
		printError("INVALID_ARGS", err.Error(), "", "", "")
		return nil, nil, err
	}
	return positional, flags, nil
}

// Commands
func cmdRead(args []string) error {
	positional, flags, err := parseOrPrintFlagError(args, parseReadFlags)
	if err != nil {
		return err
	}

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
	if err := validateMemoryScope(scope, false); err != nil {
		printError("INVALID_SCOPE", err.Error(), bankName, "", scope)
		return err
	}
	path, err := getBankPath(bankName, scope)
	if err != nil {
		printError("PATH_ERROR", err.Error(), bankName, "", scope)
		return err
	}

	bank, err := loadBank(path)
	if err != nil {
		printError("LOAD_ERROR", err.Error(), bankName, "", scope)
		return err
	}

	if bank == nil {
		// Check for default
		if defaultVal, ok := flags["default"]; ok {
			v, err := parseDefaultValue(defaultVal)
			if err != nil {
				printError("INVALID_DEFAULT", err.Error(), bankName, "", scope)
				return err
			}
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
			v, err := parseDefaultValue(defaultVal)
			if err != nil {
				printError("INVALID_DEFAULT", err.Error(), bankName, key, scope)
				return err
			}
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

func parseDefaultValue(raw string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, fmt.Errorf("invalid default JSON: %w", err)
	}
	return v, nil
}

func cmdWrite(args []string) error {
	positional, flags, err := parseOrPrintFlagError(args, parseWriteFlags)
	if err != nil {
		return err
	}

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

	if overrides.IsReservedBank(bankName) {
		fmt.Fprintf(os.Stderr, "bank %q is reserved; use customize_tools via slop-mcp\n", bankName)
		os.Exit(2)
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
	if err := validateMemoryScope(scope, false); err != nil {
		printError("INVALID_SCOPE", err.Error(), bankName, key, scope)
		return err
	}
	if scope == "" {
		// Default to project if .slop-mcp exists, else user
		projectDir, err := getProjectMemoryDir()
		if err != nil {
			printError("PATH_ERROR", err.Error(), bankName, key, scope)
			return err
		}
		if _, err := os.Stat(filepath.Dir(projectDir)); err == nil {
			scope = "project"
		} else {
			scope = "user"
		}
	}

	path, err := getBankPath(bankName, scope)
	if err != nil {
		printError("PATH_ERROR", err.Error(), bankName, key, scope)
		return err
	}

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

	// Auto-compute size
	if valBytes, err := json.Marshal(value); err == nil {
		entry.Size = len(valBytes)
	}

	// Preserve created_at and metadata if updating
	if existing, ok := bank.Entries[key]; ok {
		entry.CreatedAt = existing.CreatedAt
		entry.Description = existing.Description
		entry.Schema = existing.Schema
	} else {
		operation = "create"
	}

	// Handle TTL
	if ttlStr, ok := flags["ttl"]; ok {
		ttl, err := parseTTL(ttlStr)
		if err != nil {
			printError("INVALID_TTL", err.Error(), bankName, key, scope)
			return err
		}
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
	positional, flags, err := parseOrPrintFlagError(args, parseScopeOnlyFlags)
	if err != nil {
		return err
	}

	if len(positional) < 1 {
		printError("INVALID_ARGS", "Usage: memory-cli delete <bank> [key]", "", "", "")
		return fmt.Errorf("invalid args")
	}

	bankName := positional[0]
	if err := validateBankName(bankName); err != nil {
		printError("INVALID_BANK_NAME", err.Error(), bankName, "", "")
		return err
	}

	if overrides.IsReservedBank(bankName) {
		fmt.Fprintf(os.Stderr, "bank %q is reserved; use customize_tools via slop-mcp\n", bankName)
		os.Exit(2)
	}

	scope := flags["scope"]
	if err := validateMemoryScope(scope, false); err != nil {
		printError("INVALID_SCOPE", err.Error(), bankName, "", scope)
		return err
	}
	path, err := getBankPath(bankName, scope)
	if err != nil {
		printError("PATH_ERROR", err.Error(), bankName, "", scope)
		return err
	}

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
	positional, flags, err := parseOrPrintFlagError(args, parseScopeOnlyFlags)
	if err != nil {
		return err
	}

	scope := flags["scope"]
	if err := validateMemoryScope(scope, len(positional) == 0); err != nil {
		printError("INVALID_SCOPE", err.Error(), "", "", scope)
		return err
	}

	// List keys in specific bank
	if len(positional) > 0 {
		bankName := positional[0]
		if err := validateBankName(bankName); err != nil {
			printError("INVALID_BANK_NAME", err.Error(), bankName, "", "")
			return err
		}

		path, err := getBankPath(bankName, scope)
		if err != nil {
			printError("PATH_ERROR", err.Error(), bankName, "", scope)
			return err
		}
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
					Key:         k,
					Description: e.Description,
					Size:        e.Size,
					UpdatedAt:   e.UpdatedAt.Format(time.RFC3339),
					TTL:         e.TTL,
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
		infos, err := listBankInfos(userDir, "user")
		if err != nil {
			printError("LIST_ERROR", err.Error(), "", "", scope)
			return err
		}
		banks = append(banks, infos...)
	}

	// Scan project directory
	if scope == "" || scope == "project" || scope == "all" {
		projectDir, err := getProjectMemoryDir()
		if err != nil {
			printError("PATH_ERROR", err.Error(), "", "", scope)
			return err
		}
		infos, err := listBankInfos(projectDir, "project")
		if err != nil {
			printError("LIST_ERROR", err.Error(), "", "", scope)
			return err
		}
		banks = append(banks, infos...)
	}

	printJSON(ListResult{Banks: banks})
	return nil
}

func listBankInfos(dir, scope string) ([]BankInfo, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	banks := make([]BankInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		bank, err := loadBank(path)
		if err != nil {
			return nil, fmt.Errorf("loading bank %q: %w", name, err)
		}

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
			Scope:     scope,
			KeyCount:  keyCount,
			UpdatedAt: updatedAt,
			SizeBytes: size,
		})
	}
	return banks, nil
}

func cmdQuery(args []string) error {
	positional, flags, err := parseOrPrintFlagError(args, parseQueryFlags)
	if err != nil {
		return err
	}

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
	if err := validateMemoryScope(scope, false); err != nil {
		printError("INVALID_SCOPE", err.Error(), bankName, "", scope)
		return err
	}
	path, err := getBankPath(bankName, scope)
	if err != nil {
		printError("PATH_ERROR", err.Error(), bankName, "", scope)
		return err
	}

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
				index, err := parseArrayIndex(indexStr)
				if err != nil {
					return nil, err
				}

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

func parseTTL(raw string) (int64, error) {
	ttl, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || ttl <= 0 {
		return 0, fmt.Errorf("invalid ttl %q: expected a positive integer number of seconds", raw)
	}
	return ttl, nil
}

func parseArrayIndex(raw string) (int, error) {
	index, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid array index %q", raw)
	}
	return index, nil
}

func cmdSearch(args []string) error {
	positional, flags, err := parseOrPrintFlagError(args, parseSearchFlags)
	if err != nil {
		return err
	}

	if len(positional) < 1 {
		printError("INVALID_ARGS", "Usage: memory-cli search <pattern>", "", "", "")
		return fmt.Errorf("invalid args")
	}

	pattern := strings.ToLower(positional[0])
	scope := flags["scope"]
	if err := validateMemoryScope(scope, true); err != nil {
		printError("INVALID_SCOPE", err.Error(), "", "", scope)
		return err
	}
	searchValues := flags["value"] == "true"

	matches := make([]SearchMatch, 0)

	// Search function
	searchBank := func(bankPath, bankName, bankScope string) error {
		bank, err := loadBank(bankPath)
		if err != nil {
			return fmt.Errorf("loading bank %q: %w", bankName, err)
		}
		if bank == nil {
			return nil
		}

		for key, entry := range bank.Entries {
			if entry.IsExpired() {
				continue
			}

			matched := false

			// Match key name
			if strings.Contains(strings.ToLower(key), pattern) {
				matched = true
			}

			// Match description
			if !matched && strings.Contains(strings.ToLower(entry.Description), pattern) {
				matched = true
			}

			// Match value if requested
			if !matched && searchValues {
				valueJSON, _ := json.Marshal(entry.Value)
				if strings.Contains(strings.ToLower(string(valueJSON)), pattern) {
					matched = true
				}
			}

			if matched {
				match := SearchMatch{
					Bank:        bankName,
					Key:         key,
					Description: entry.Description,
					Scope:       bankScope,
				}
				if searchValues {
					match.Value = entry.Value
				}
				matches = append(matches, match)
			}
		}
		return nil
	}

	// Search user banks
	if scope == "" || scope == "user" || scope == "all" {
		userDir := getUserMemoryDir()
		if err := searchBankFiles(userDir, "user", searchBank); err != nil {
			printError("SEARCH_ERROR", err.Error(), "", "", scope)
			return err
		}
	}

	// Search project banks
	if scope == "" || scope == "project" || scope == "all" {
		projectDir, err := getProjectMemoryDir()
		if err != nil {
			printError("PATH_ERROR", err.Error(), "", "", scope)
			return err
		}
		if err := searchBankFiles(projectDir, "project", searchBank); err != nil {
			printError("SEARCH_ERROR", err.Error(), "", "", scope)
			return err
		}
	}

	printJSON(SearchResult{
		Matches: matches,
		Total:   len(matches),
	})
	return nil
}

func searchBankFiles(dir, scope string, searchBank func(path, name, scope string) error) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		if err := searchBank(filepath.Join(dir, entry.Name()), name, scope); err != nil {
			return err
		}
	}
	return nil
}
