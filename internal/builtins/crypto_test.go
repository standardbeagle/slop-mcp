package builtins

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/standardbeagle/slop/pkg/slop"
)

func TestRegisterCrypto(t *testing.T) {
	rt := slop.NewRuntime()
	defer rt.Close()

	RegisterCrypto(rt)

	// Test crypto_password is registered and works
	result, err := rt.Execute("crypto_password(16)")
	if err != nil {
		t.Fatalf("crypto_password failed: %v", err)
	}

	sv, ok := result.(*slop.StringValue)
	if !ok {
		t.Fatalf("expected StringValue, got %T", result)
	}

	if len(sv.Value) != 16 {
		t.Errorf("expected password length 16, got %d", len(sv.Value))
	}
}

func TestCryptoPassword(t *testing.T) {
	tests := []struct {
		name     string
		script   string
		validate func(t *testing.T, result slop.Value)
	}{
		{
			name:   "default length",
			script: "crypto_password()",
			validate: func(t *testing.T, result slop.Value) {
				sv := result.(*slop.StringValue)
				if len(sv.Value) != 32 {
					t.Errorf("expected length 32, got %d", len(sv.Value))
				}
			},
		},
		{
			name:   "custom length",
			script: "crypto_password(64)",
			validate: func(t *testing.T, result slop.Value) {
				sv := result.(*slop.StringValue)
				if len(sv.Value) != 64 {
					t.Errorf("expected length 64, got %d", len(sv.Value))
				}
			},
		},
		// Note: kwargs not supported in SLOP function call syntax
		// The kwargs are available for service calls
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := slop.NewRuntime()
			defer rt.Close()
			RegisterCrypto(rt)

			result, err := rt.Execute(tt.script)
			if err != nil {
				t.Fatalf("execution failed: %v", err)
			}
			tt.validate(t, result)
		})
	}
}

func TestCryptoPassphrase(t *testing.T) {
	rt := slop.NewRuntime()
	defer rt.Close()
	RegisterCrypto(rt)

	result, err := rt.Execute("crypto_passphrase(4)")
	if err != nil {
		t.Fatalf("crypto_passphrase failed: %v", err)
	}

	sv := result.(*slop.StringValue)
	words := strings.Split(sv.Value, "-")
	if len(words) != 4 {
		t.Errorf("expected 4 words, got %d", len(words))
	}
}

func TestCryptoEd25519Keygen(t *testing.T) {
	rt := slop.NewRuntime()
	defer rt.Close()
	RegisterCrypto(rt)

	result, err := rt.Execute("crypto_ed25519_keygen()")
	if err != nil {
		t.Fatalf("crypto_ed25519_keygen failed: %v", err)
	}

	mv, ok := result.(*slop.MapValue)
	if !ok {
		t.Fatalf("expected MapValue, got %T", result)
	}

	// Check that keys are present
	keys := []string{"private_key", "public_key", "private_hex", "public_hex"}
	for _, key := range keys {
		if _, ok := mv.Pairs[key]; !ok {
			t.Errorf("missing key: %s", key)
		}
	}

	// Verify PEM format
	privKey := mv.Pairs["private_key"].(*slop.StringValue).Value
	if !strings.Contains(privKey, "-----BEGIN PRIVATE KEY-----") {
		t.Error("private_key not in PEM format")
	}
}

func TestCryptoRSAKeygen(t *testing.T) {
	rt := slop.NewRuntime()
	defer rt.Close()
	RegisterCrypto(rt)

	// Use smaller key size for faster tests
	result, err := rt.Execute("crypto_rsa_keygen(2048)")
	if err != nil {
		t.Fatalf("crypto_rsa_keygen failed: %v", err)
	}

	mv, ok := result.(*slop.MapValue)
	if !ok {
		t.Fatalf("expected MapValue, got %T", result)
	}

	privKey := mv.Pairs["private_key"].(*slop.StringValue).Value
	if !strings.Contains(privKey, "-----BEGIN PRIVATE KEY-----") {
		t.Error("private_key not in PEM format")
	}
}

func TestCryptoSHA256(t *testing.T) {
	rt := slop.NewRuntime()
	defer rt.Close()
	RegisterCrypto(rt)

	result, err := rt.Execute(`crypto_sha256("hello")`)
	if err != nil {
		t.Fatalf("crypto_sha256 failed: %v", err)
	}

	sv := result.(*slop.StringValue)
	// Known SHA256 hash of "hello"
	expected := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if sv.Value != expected {
		t.Errorf("expected %s, got %s", expected, sv.Value)
	}
}

func TestCryptoRandomBytes(t *testing.T) {
	rt := slop.NewRuntime()
	defer rt.Close()
	RegisterCrypto(rt)

	result, err := rt.Execute("crypto_random_bytes(16)")
	if err != nil {
		t.Fatalf("crypto_random_bytes failed: %v", err)
	}

	sv := result.(*slop.StringValue)
	// Result should be hex-encoded, so 32 chars for 16 bytes
	if len(sv.Value) != 32 {
		t.Errorf("expected 32 hex chars, got %d", len(sv.Value))
	}

	// Should be valid hex
	_, err = hex.DecodeString(sv.Value)
	if err != nil {
		t.Errorf("invalid hex: %v", err)
	}
}

func TestCryptoBase64(t *testing.T) {
	rt := slop.NewRuntime()
	defer rt.Close()
	RegisterCrypto(rt)

	// Encode
	result, err := rt.Execute(`crypto_base64_encode("hello world")`)
	if err != nil {
		t.Fatalf("crypto_base64_encode failed: %v", err)
	}

	sv := result.(*slop.StringValue)
	if sv.Value != "aGVsbG8gd29ybGQ=" {
		t.Errorf("unexpected base64: %s", sv.Value)
	}

	// Decode
	result, err = rt.Execute(`crypto_base64_decode("aGVsbG8gd29ybGQ=")`)
	if err != nil {
		t.Fatalf("crypto_base64_decode failed: %v", err)
	}

	sv = result.(*slop.StringValue)
	if sv.Value != "hello world" {
		t.Errorf("unexpected decoded: %s", sv.Value)
	}
}

func TestCryptoHex(t *testing.T) {
	rt := slop.NewRuntime()
	defer rt.Close()
	RegisterCrypto(rt)

	// Encode
	result, err := rt.Execute(`crypto_hex_encode("hello")`)
	if err != nil {
		t.Fatalf("crypto_hex_encode failed: %v", err)
	}

	sv := result.(*slop.StringValue)
	if sv.Value != "68656c6c6f" {
		t.Errorf("unexpected hex: %s", sv.Value)
	}

	// Decode
	result, err = rt.Execute(`crypto_hex_decode("68656c6c6f")`)
	if err != nil {
		t.Fatalf("crypto_hex_decode failed: %v", err)
	}

	sv = result.(*slop.StringValue)
	if sv.Value != "hello" {
		t.Errorf("unexpected decoded: %s", sv.Value)
	}
}

func TestCryptoHash(t *testing.T) {
	rt := slop.NewRuntime()
	defer rt.Close()
	RegisterCrypto(rt)

	// Test default algorithm (sha256) via crypto_hash
	result, err := rt.Execute(`crypto_hash("hello")`)
	if err != nil {
		t.Fatalf("crypto_hash failed: %v", err)
	}

	sv := result.(*slop.StringValue)
	expected := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if sv.Value != expected {
		t.Errorf("expected %s, got %s", expected, sv.Value)
	}
}
