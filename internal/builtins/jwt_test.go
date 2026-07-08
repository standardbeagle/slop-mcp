package builtins

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/standardbeagle/slop/pkg/slop"
)

func TestJWT_ES256_SignVerifyRoundTrip(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	privDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal private key: %v", err)
	}
	privPEM := string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER}))

	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("failed to marshal public key: %v", err)
	}
	pubPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))

	claims := slop.GoToValue(map[string]any{"sub": "alice", "role": "admin"})

	// Sign
	signed, err := jwtSign([]slop.Value{
		claims,
		slop.NewStringValue(privPEM),
		slop.NewStringValue("ES256"),
	}, nil)
	if err != nil {
		t.Fatalf("jwt_sign ES256 failed: %v", err)
	}
	token, ok := signed.(*slop.StringValue)
	if !ok {
		t.Fatalf("expected StringValue token, got %T", signed)
	}

	// Verify
	verified, err := jwtVerify([]slop.Value{
		token,
		slop.NewStringValue(pubPEM),
		slop.NewStringValue("ES256"),
	}, nil)
	if err != nil {
		t.Fatalf("jwt_verify ES256 failed: %v", err)
	}
	mv, ok := verified.(*slop.MapValue)
	if !ok {
		t.Fatalf("expected MapValue result, got %T", verified)
	}
	payloadV, ok := mv.Get("payload")
	if !ok {
		t.Fatal("verify result missing payload")
	}
	payload, ok := payloadV.(*slop.MapValue)
	if !ok {
		t.Fatalf("expected payload MapValue, got %T", payloadV)
	}
	subV, ok := payload.Get("sub")
	if !ok {
		t.Fatal("payload missing sub claim")
	}
	sub, ok := subV.(*slop.StringValue)
	if !ok || sub.Value != "alice" {
		t.Errorf("expected sub=alice, got %v", subV)
	}

	// Tampered token must fail verification
	tampered := token.Value[:len(token.Value)-4] + "AAAA"
	_, err = jwtVerify([]slop.Value{
		slop.NewStringValue(tampered),
		slop.NewStringValue(pubPEM),
		slop.NewStringValue("ES256"),
	}, nil)
	if err == nil {
		t.Error("expected verification failure for tampered token")
	}
}

func TestJWTSignPreservesFloatAndNullClaims(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	privDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal private key: %v", err)
	}
	privPEM := string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER}))

	claims := slop.NewMapValue()
	claims.Set("exp", slop.NewNumberValue(1.7e9))
	claims.Set("optional", slop.NewNullValue())

	signed, err := jwtSign([]slop.Value{
		claims,
		slop.NewStringValue(privPEM),
		slop.NewStringValue("ES256"),
	}, nil)
	if err != nil {
		t.Fatalf("jwt_sign ES256 failed: %v", err)
	}
	token := signed.(*slop.StringValue).Value
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("payload is not base64url: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if _, ok := payload["exp"].(float64); !ok {
		t.Fatalf("exp type = %T, want float64", payload["exp"])
	}
	if payload["optional"] != nil {
		t.Fatalf("optional = %#v, want nil", payload["optional"])
	}
}
