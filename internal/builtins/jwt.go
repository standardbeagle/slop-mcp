package builtins

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"hash"
	"math/big"
	"strings"
	"time"

	"github.com/standardbeagle/slop/pkg/slop"
)

// RegisterJWT registers JWT functions with the SLOP runtime.
func RegisterJWT(rt *slop.Runtime) {
	rt.RegisterBuiltin("jwt_decode", jwtDecode)
	rt.RegisterBuiltin("jwt_verify", jwtVerify)
	rt.RegisterBuiltin("jwt_sign", jwtSign)
	rt.RegisterBuiltin("jwt_expired", jwtExpired)
}

// jwtDecode decodes a JWT without verification (useful for inspecting claims).
// Returns {header: {...}, payload: {...}, signature: "..."}
func jwtDecode(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("jwt_decode requires token argument")
	}

	tokenStr, ok := args[0].(*slop.StringValue)
	if !ok {
		return nil, fmt.Errorf("jwt_decode: token must be a string")
	}

	parts := strings.Split(tokenStr.Value, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("jwt_decode: invalid token format (expected 3 parts)")
	}

	header, err := decodeJWTPart(parts[0])
	if err != nil {
		return nil, fmt.Errorf("jwt_decode: invalid header: %w", err)
	}

	payload, err := decodeJWTPart(parts[1])
	if err != nil {
		return nil, fmt.Errorf("jwt_decode: invalid payload: %w", err)
	}

	return slop.GoToValue(map[string]any{
		"header":    header,
		"payload":   payload,
		"signature": parts[2],
	}), nil
}

// jwtVerify verifies a JWT signature and returns the decoded token.
// jwt_verify(token, key, algorithm)
// Supported algorithms: HS256, HS384, HS512, RS256, RS384, RS512, ES256, ES384, ES512, EdDSA
func jwtVerify(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("jwt_verify requires token, key, and algorithm arguments")
	}

	tokenStr, ok := args[0].(*slop.StringValue)
	if !ok {
		return nil, fmt.Errorf("jwt_verify: token must be a string")
	}

	keyStr, ok := args[1].(*slop.StringValue)
	if !ok {
		return nil, fmt.Errorf("jwt_verify: key must be a string")
	}

	algStr, ok := args[2].(*slop.StringValue)
	if !ok {
		return nil, fmt.Errorf("jwt_verify: algorithm must be a string")
	}

	parts := strings.Split(tokenStr.Value, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("jwt_verify: invalid token format")
	}

	// Verify signature
	signingInput := parts[0] + "." + parts[1]
	signature, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("jwt_verify: invalid signature encoding: %w", err)
	}

	valid, err := verifySignature(signingInput, signature, keyStr.Value, algStr.Value)
	if err != nil {
		return nil, fmt.Errorf("jwt_verify: %w", err)
	}

	if !valid {
		return nil, fmt.Errorf("jwt_verify: signature verification failed")
	}

	// Decode and return
	header, _ := decodeJWTPart(parts[0])
	payload, _ := decodeJWTPart(parts[1])

	return slop.GoToValue(map[string]any{
		"valid":   true,
		"header":  header,
		"payload": payload,
	}), nil
}

// jwtSign creates a signed JWT token.
// jwt_sign(claims, key, algorithm)
func jwtSign(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	if len(args) < 3 {
		return nil, fmt.Errorf("jwt_sign requires claims, key, and algorithm arguments")
	}

	claimsVal, ok := args[0].(*slop.MapValue)
	if !ok {
		return nil, fmt.Errorf("jwt_sign: claims must be a map")
	}

	keyStr, ok := args[1].(*slop.StringValue)
	if !ok {
		return nil, fmt.Errorf("jwt_sign: key must be a string")
	}

	algStr, ok := args[2].(*slop.StringValue)
	if !ok {
		return nil, fmt.Errorf("jwt_sign: algorithm must be a string")
	}

	// Build header
	header := map[string]any{
		"alg": algStr.Value,
		"typ": "JWT",
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("jwt_sign: failed to encode header: %w", err)
	}

	// Convert claims to Go map
	claims := slopMapToGoMap(claimsVal)
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return nil, fmt.Errorf("jwt_sign: failed to encode claims: %w", err)
	}

	// Base64URL encode
	headerB64 := base64URLEncode(headerJSON)
	claimsB64 := base64URLEncode(claimsJSON)
	signingInput := headerB64 + "." + claimsB64

	// Sign
	signature, err := createSignature(signingInput, keyStr.Value, algStr.Value)
	if err != nil {
		return nil, fmt.Errorf("jwt_sign: %w", err)
	}

	signatureB64 := base64URLEncode(signature)
	token := signingInput + "." + signatureB64

	return slop.NewStringValue(token), nil
}

// jwtExpired checks if a JWT's exp claim has passed.
func jwtExpired(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("jwt_expired requires token argument")
	}

	tokenStr, ok := args[0].(*slop.StringValue)
	if !ok {
		return nil, fmt.Errorf("jwt_expired: token must be a string")
	}

	parts := strings.Split(tokenStr.Value, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("jwt_expired: invalid token format")
	}

	payload, err := decodeJWTPart(parts[1])
	if err != nil {
		return nil, fmt.Errorf("jwt_expired: invalid payload: %w", err)
	}

	exp, ok := payload["exp"]
	if !ok {
		// No exp claim means not expired (or never expires)
		return slop.GoToValue(false), nil
	}

	var expTime int64
	switch v := exp.(type) {
	case float64:
		expTime = int64(v)
	case int64:
		expTime = v
	case int:
		expTime = int64(v)
	default:
		return nil, fmt.Errorf("jwt_expired: exp claim is not a number")
	}

	expired := time.Now().Unix() > expTime
	return slop.GoToValue(expired), nil
}

// Helper functions

func decodeJWTPart(part string) (map[string]any, error) {
	data, err := base64URLDecode(part)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func base64URLDecode(s string) ([]byte, error) {
	// Add padding if needed
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

func base64URLEncode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

func verifySignature(signingInput string, signature []byte, key string, alg string) (bool, error) {
	switch alg {
	case "HS256":
		return verifyHMAC(signingInput, signature, key, sha256.New)
	case "HS384":
		return verifyHMAC(signingInput, signature, key, sha512.New384)
	case "HS512":
		return verifyHMAC(signingInput, signature, key, sha512.New)
	case "RS256":
		return verifyRSA(signingInput, signature, key, crypto.SHA256)
	case "RS384":
		return verifyRSA(signingInput, signature, key, crypto.SHA384)
	case "RS512":
		return verifyRSA(signingInput, signature, key, crypto.SHA512)
	case "ES256":
		return verifyECDSA(signingInput, signature, key, crypto.SHA256)
	case "ES384":
		return verifyECDSA(signingInput, signature, key, crypto.SHA384)
	case "ES512":
		return verifyECDSA(signingInput, signature, key, crypto.SHA512)
	case "EdDSA":
		return verifyEdDSA(signingInput, signature, key)
	default:
		return false, fmt.Errorf("unsupported algorithm: %s", alg)
	}
}

func verifyHMAC(signingInput string, signature []byte, key string, hashFunc func() hash.Hash) (bool, error) {
	mac := hmac.New(hashFunc, []byte(key))
	mac.Write([]byte(signingInput))
	expected := mac.Sum(nil)
	return hmac.Equal(signature, expected), nil
}

func verifyRSA(signingInput string, signature []byte, keyPEM string, hashType crypto.Hash) (bool, error) {
	pubKey, err := parseRSAPublicKey(keyPEM)
	if err != nil {
		return false, err
	}

	hasher := hashType.New()
	hasher.Write([]byte(signingInput))
	hashed := hasher.Sum(nil)

	err = rsa.VerifyPKCS1v15(pubKey, hashType, hashed, signature)
	return err == nil, nil
}

func verifyECDSA(signingInput string, signature []byte, keyPEM string, hashType crypto.Hash) (bool, error) {
	pubKey, err := parseECDSAPublicKey(keyPEM)
	if err != nil {
		return false, err
	}

	hasher := hashType.New()
	hasher.Write([]byte(signingInput))
	hashed := hasher.Sum(nil)

	// ECDSA signature is r || s
	keySize := (pubKey.Curve.Params().BitSize + 7) / 8
	if len(signature) != 2*keySize {
		return false, fmt.Errorf("invalid ECDSA signature length")
	}

	r := new(big.Int).SetBytes(signature[:keySize])
	s := new(big.Int).SetBytes(signature[keySize:])

	return ecdsa.Verify(pubKey, hashed, r, s), nil
}

func verifyEdDSA(signingInput string, signature []byte, keyPEM string) (bool, error) {
	pubKey, err := parseEdDSAPublicKey(keyPEM)
	if err != nil {
		return false, err
	}

	return ed25519.Verify(pubKey, []byte(signingInput), signature), nil
}

func createSignature(signingInput string, key string, alg string) ([]byte, error) {
	switch alg {
	case "HS256":
		return signHMAC(signingInput, key, sha256.New)
	case "HS384":
		return signHMAC(signingInput, key, sha512.New384)
	case "HS512":
		return signHMAC(signingInput, key, sha512.New)
	case "RS256":
		return signRSA(signingInput, key, crypto.SHA256)
	case "RS384":
		return signRSA(signingInput, key, crypto.SHA384)
	case "RS512":
		return signRSA(signingInput, key, crypto.SHA512)
	case "ES256":
		return signECDSA(signingInput, key, crypto.SHA256)
	case "ES384":
		return signECDSA(signingInput, key, crypto.SHA384)
	case "ES512":
		return signECDSA(signingInput, key, crypto.SHA512)
	case "EdDSA":
		return signEdDSA(signingInput, key)
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", alg)
	}
}

func signHMAC(signingInput string, key string, hashFunc func() hash.Hash) ([]byte, error) {
	mac := hmac.New(hashFunc, []byte(key))
	mac.Write([]byte(signingInput))
	return mac.Sum(nil), nil
}

func signRSA(signingInput string, keyPEM string, hashType crypto.Hash) ([]byte, error) {
	privKey, err := parseRSAPrivateKey(keyPEM)
	if err != nil {
		return nil, err
	}

	hasher := hashType.New()
	hasher.Write([]byte(signingInput))
	hashed := hasher.Sum(nil)

	return rsa.SignPKCS1v15(nil, privKey, hashType, hashed)
}

func signECDSA(signingInput string, keyPEM string, hashType crypto.Hash) ([]byte, error) {
	privKey, err := parseECDSAPrivateKey(keyPEM)
	if err != nil {
		return nil, err
	}

	hasher := hashType.New()
	hasher.Write([]byte(signingInput))
	hashed := hasher.Sum(nil)

	r, s, err := ecdsa.Sign(nil, privKey, hashed)
	if err != nil {
		return nil, err
	}

	// Encode as r || s with fixed size
	keySize := (privKey.Curve.Params().BitSize + 7) / 8
	signature := make([]byte, 2*keySize)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(signature[keySize-len(rBytes):keySize], rBytes)
	copy(signature[2*keySize-len(sBytes):], sBytes)

	return signature, nil
}

func signEdDSA(signingInput string, keyPEM string) ([]byte, error) {
	privKey, err := parseEdDSAPrivateKey(keyPEM)
	if err != nil {
		return nil, err
	}

	return ed25519.Sign(privKey, []byte(signingInput)), nil
}

// Key parsing helpers

func parseRSAPublicKey(keyPEM string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		// Try parsing as PKCS1
		return x509.ParsePKCS1PublicKey(block.Bytes)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}

	return rsaPub, nil
}

func parseRSAPrivateKey(keyPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}

	// Try PKCS8 first
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if ok {
			return rsaKey, nil
		}
	}

	// Fall back to PKCS1
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func parseECDSAPublicKey(keyPEM string) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	ecdsaPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an ECDSA public key")
	}

	return ecdsaPub, nil
}

func parseECDSAPrivateKey(keyPEM string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}

	// Try PKCS8 first
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err == nil {
		ecdsaKey, ok := key.(*ecdsa.PrivateKey)
		if ok {
			return ecdsaKey, nil
		}
	}

	// Fall back to EC private key
	return x509.ParseECPrivateKey(block.Bytes)
}

func parseEdDSAPublicKey(keyPEM string) (ed25519.PublicKey, error) {
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	edPub, ok := pub.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an Ed25519 public key")
	}

	return edPub, nil
}

func parseEdDSAPrivateKey(keyPEM string) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	edKey, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an Ed25519 private key")
	}

	return edKey, nil
}

// Value conversion helpers

func slopMapToGoMap(m *slop.MapValue) map[string]any {
	result := make(map[string]any)
	for k, v := range m.Pairs {
		result[k] = slopValueToAny(v)
	}
	return result
}

func slopValueToAny(v slop.Value) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case *slop.BoolValue:
		return val.Value
	case *slop.IntValue:
		return val.Value
	case *slop.StringValue:
		return val.Value
	case *slop.ListValue:
		result := make([]any, len(val.Elements))
		for i, elem := range val.Elements {
			result[i] = slopValueToAny(elem)
		}
		return result
	case *slop.MapValue:
		return slopMapToGoMap(val)
	default:
		return fmt.Sprintf("%v", v)
	}
}
