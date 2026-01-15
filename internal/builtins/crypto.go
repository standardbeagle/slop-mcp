// Package builtins provides additional built-in functions for SLOP scripts.
package builtins

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/standardbeagle/slop/pkg/slop"
)

// Character sets for password generation
const (
	lowercase = "abcdefghijklmnopqrstuvwxyz"
	uppercase = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digits    = "0123456789"
	symbols   = "!@#$%^&*()_+-=[]{}|;:,.<>?"
)

// RegisterCrypto registers all crypto built-in functions with the runtime.
func RegisterCrypto(rt *slop.Runtime) {
	// Password generation
	rt.RegisterBuiltin("crypto_password", builtinCryptoPassword)
	rt.RegisterBuiltin("crypto_passphrase", builtinCryptoPassphrase)

	// Key generation
	rt.RegisterBuiltin("crypto_ed25519_keygen", builtinEd25519Keygen)
	rt.RegisterBuiltin("crypto_rsa_keygen", builtinRSAKeygen)
	rt.RegisterBuiltin("crypto_x25519_keygen", builtinX25519Keygen)

	// Hashing
	rt.RegisterBuiltin("crypto_sha256", builtinSHA256)
	rt.RegisterBuiltin("crypto_sha512", builtinSHA512)
	rt.RegisterBuiltin("crypto_hash", builtinHash)

	// Random bytes
	rt.RegisterBuiltin("crypto_random_bytes", builtinRandomBytes)
	rt.RegisterBuiltin("crypto_random_base64", builtinRandomBase64)

	// Encoding
	rt.RegisterBuiltin("crypto_base64_encode", builtinBase64Encode)
	rt.RegisterBuiltin("crypto_base64_decode", builtinBase64Decode)
	rt.RegisterBuiltin("crypto_hex_encode", builtinHexEncode)
	rt.RegisterBuiltin("crypto_hex_decode", builtinHexDecode)
}

// Password generation

func builtinCryptoPassword(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	// Default options
	length := 32
	useUpper := true
	useLower := true
	useDigits := true
	useSymbols := true
	exclude := ""

	// Parse length from args
	if len(args) > 0 {
		if v, ok := args[0].(*slop.IntValue); ok {
			length = int(v.Value)
		}
	}

	// Parse kwargs
	if v, ok := kwargs["length"]; ok {
		if iv, ok := v.(*slop.IntValue); ok {
			length = int(iv.Value)
		}
	}
	if v, ok := kwargs["upper"]; ok {
		if bv, ok := v.(*slop.BoolValue); ok {
			useUpper = bv.Value
		}
	}
	if v, ok := kwargs["lower"]; ok {
		if bv, ok := v.(*slop.BoolValue); ok {
			useLower = bv.Value
		}
	}
	if v, ok := kwargs["digits"]; ok {
		if bv, ok := v.(*slop.BoolValue); ok {
			useDigits = bv.Value
		}
	}
	if v, ok := kwargs["symbols"]; ok {
		if bv, ok := v.(*slop.BoolValue); ok {
			useSymbols = bv.Value
		}
	}
	if v, ok := kwargs["exclude"]; ok {
		if sv, ok := v.(*slop.StringValue); ok {
			exclude = sv.Value
		}
	}

	if length < 1 {
		return nil, fmt.Errorf("crypto_password: length must be at least 1")
	}

	// Build character set
	var charset strings.Builder
	if useLower {
		charset.WriteString(lowercase)
	}
	if useUpper {
		charset.WriteString(uppercase)
	}
	if useDigits {
		charset.WriteString(digits)
	}
	if useSymbols {
		charset.WriteString(symbols)
	}

	chars := charset.String()
	if exclude != "" {
		for _, c := range exclude {
			chars = strings.ReplaceAll(chars, string(c), "")
		}
	}

	if len(chars) == 0 {
		return nil, fmt.Errorf("crypto_password: no characters available after exclusions")
	}

	// Generate password using crypto/rand
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		idx, err := randomInt(len(chars))
		if err != nil {
			return nil, fmt.Errorf("crypto_password: %w", err)
		}
		result[i] = chars[idx]
	}

	return slop.NewStringValue(string(result)), nil
}

func builtinCryptoPassphrase(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	// Default options
	wordCount := 6
	separator := "-"

	// Parse args
	if len(args) > 0 {
		if v, ok := args[0].(*slop.IntValue); ok {
			wordCount = int(v.Value)
		}
	}

	// Parse kwargs
	if v, ok := kwargs["words"]; ok {
		if iv, ok := v.(*slop.IntValue); ok {
			wordCount = int(iv.Value)
		}
	}
	if v, ok := kwargs["separator"]; ok {
		if sv, ok := v.(*slop.StringValue); ok {
			separator = sv.Value
		}
	}

	if wordCount < 1 {
		return nil, fmt.Errorf("crypto_passphrase: word count must be at least 1")
	}

	// Generate passphrase from EFF wordlist (subset)
	words := make([]string, wordCount)
	for i := 0; i < wordCount; i++ {
		idx, err := randomInt(len(effWordlist))
		if err != nil {
			return nil, fmt.Errorf("crypto_passphrase: %w", err)
		}
		words[i] = effWordlist[idx]
	}

	return slop.NewStringValue(strings.Join(words, separator)), nil
}

// Key generation

func builtinEd25519Keygen(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("crypto_ed25519_keygen: %w", err)
	}

	// Encode as PEM
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: priv,
	})

	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pub,
	})

	return slop.GoToValue(map[string]any{
		"private_key": string(privPEM),
		"public_key":  string(pubPEM),
		"private_hex": hex.EncodeToString(priv.Seed()),
		"public_hex":  hex.EncodeToString(pub),
	}), nil
}

func builtinRSAKeygen(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	bits := 4096

	// Parse args
	if len(args) > 0 {
		if v, ok := args[0].(*slop.IntValue); ok {
			bits = int(v.Value)
		}
	}
	if v, ok := kwargs["bits"]; ok {
		if iv, ok := v.(*slop.IntValue); ok {
			bits = int(iv.Value)
		}
	}

	if bits < 2048 {
		return nil, fmt.Errorf("crypto_rsa_keygen: bits must be at least 2048")
	}

	priv, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, fmt.Errorf("crypto_rsa_keygen: %w", err)
	}

	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("crypto_rsa_keygen: %w", err)
	}

	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("crypto_rsa_keygen: %w", err)
	}

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privDER,
	})

	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDER,
	})

	return slop.GoToValue(map[string]any{
		"private_key": string(privPEM),
		"public_key":  string(pubPEM),
		"bits":        bits,
	}), nil
}

func builtinX25519Keygen(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	// X25519 uses 32-byte keys
	priv := make([]byte, 32)
	if _, err := rand.Read(priv); err != nil {
		return nil, fmt.Errorf("crypto_x25519_keygen: %w", err)
	}

	// Clamp private key per X25519 spec
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64

	// For X25519, we just return the raw keys (no standard PEM format)
	return slop.GoToValue(map[string]any{
		"private_key": base64.StdEncoding.EncodeToString(priv),
		"private_hex": hex.EncodeToString(priv),
	}), nil
}

// Hashing

func builtinSHA256(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("crypto_sha256: requires input argument")
	}

	data, err := getBytes(args[0])
	if err != nil {
		return nil, fmt.Errorf("crypto_sha256: %w", err)
	}

	hash := sha256.Sum256(data)
	return slop.NewStringValue(hex.EncodeToString(hash[:])), nil
}

func builtinSHA512(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("crypto_sha512: requires input argument")
	}

	data, err := getBytes(args[0])
	if err != nil {
		return nil, fmt.Errorf("crypto_sha512: %w", err)
	}

	hash := sha512.Sum512(data)
	return slop.NewStringValue(hex.EncodeToString(hash[:])), nil
}

func builtinHash(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("crypto_hash: requires input argument")
	}

	algorithm := "sha256"
	if v, ok := kwargs["algorithm"]; ok {
		if sv, ok := v.(*slop.StringValue); ok {
			algorithm = sv.Value
		}
	}

	data, err := getBytes(args[0])
	if err != nil {
		return nil, fmt.Errorf("crypto_hash: %w", err)
	}

	var result []byte
	switch algorithm {
	case "sha256":
		h := sha256.Sum256(data)
		result = h[:]
	case "sha512":
		h := sha512.Sum512(data)
		result = h[:]
	case "sha384":
		h := sha512.Sum384(data)
		result = h[:]
	default:
		return nil, fmt.Errorf("crypto_hash: unsupported algorithm %s", algorithm)
	}

	return slop.NewStringValue(hex.EncodeToString(result)), nil
}

// Random bytes

func builtinRandomBytes(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	length := 32
	if len(args) > 0 {
		if v, ok := args[0].(*slop.IntValue); ok {
			length = int(v.Value)
		}
	}

	if length < 1 {
		return nil, fmt.Errorf("crypto_random_bytes: length must be at least 1")
	}

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return nil, fmt.Errorf("crypto_random_bytes: %w", err)
	}

	return slop.NewStringValue(hex.EncodeToString(bytes)), nil
}

func builtinRandomBase64(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	length := 32
	if len(args) > 0 {
		if v, ok := args[0].(*slop.IntValue); ok {
			length = int(v.Value)
		}
	}

	if length < 1 {
		return nil, fmt.Errorf("crypto_random_base64: length must be at least 1")
	}

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return nil, fmt.Errorf("crypto_random_base64: %w", err)
	}

	return slop.NewStringValue(base64.StdEncoding.EncodeToString(bytes)), nil
}

// Encoding

func builtinBase64Encode(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("crypto_base64_encode: requires input argument")
	}

	data, err := getBytes(args[0])
	if err != nil {
		return nil, fmt.Errorf("crypto_base64_encode: %w", err)
	}

	urlSafe := false
	if v, ok := kwargs["url_safe"]; ok {
		if bv, ok := v.(*slop.BoolValue); ok {
			urlSafe = bv.Value
		}
	}

	var encoded string
	if urlSafe {
		encoded = base64.URLEncoding.EncodeToString(data)
	} else {
		encoded = base64.StdEncoding.EncodeToString(data)
	}

	return slop.NewStringValue(encoded), nil
}

func builtinBase64Decode(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("crypto_base64_decode: requires input argument")
	}

	sv, ok := args[0].(*slop.StringValue)
	if !ok {
		return nil, fmt.Errorf("crypto_base64_decode: requires string input")
	}

	urlSafe := false
	if v, ok := kwargs["url_safe"]; ok {
		if bv, ok := v.(*slop.BoolValue); ok {
			urlSafe = bv.Value
		}
	}

	var decoded []byte
	var err error
	if urlSafe {
		decoded, err = base64.URLEncoding.DecodeString(sv.Value)
	} else {
		decoded, err = base64.StdEncoding.DecodeString(sv.Value)
	}

	if err != nil {
		return nil, fmt.Errorf("crypto_base64_decode: %w", err)
	}

	return slop.NewStringValue(string(decoded)), nil
}

func builtinHexEncode(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("crypto_hex_encode: requires input argument")
	}

	data, err := getBytes(args[0])
	if err != nil {
		return nil, fmt.Errorf("crypto_hex_encode: %w", err)
	}

	return slop.NewStringValue(hex.EncodeToString(data)), nil
}

func builtinHexDecode(args []slop.Value, kwargs map[string]slop.Value) (slop.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("crypto_hex_decode: requires input argument")
	}

	sv, ok := args[0].(*slop.StringValue)
	if !ok {
		return nil, fmt.Errorf("crypto_hex_decode: requires string input")
	}

	decoded, err := hex.DecodeString(sv.Value)
	if err != nil {
		return nil, fmt.Errorf("crypto_hex_decode: %w", err)
	}

	return slop.NewStringValue(string(decoded)), nil
}

// Helper functions

func randomInt(max int) (int, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return 0, err
	}
	// Simple modulo (not perfectly uniform but good enough for passwords)
	n := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if n < 0 {
		n = -n
	}
	return n % max, nil
}

func getBytes(v slop.Value) ([]byte, error) {
	switch val := v.(type) {
	case *slop.StringValue:
		return []byte(val.Value), nil
	default:
		return nil, fmt.Errorf("expected string, got %T", v)
	}
}

// EFF Short Wordlist (subset for passphrase generation)
// Full list: https://www.eff.org/dice
var effWordlist = []string{
	"acid", "acorn", "acre", "acts", "afar", "affix", "aged", "agent", "agile", "aging",
	"agony", "ahead", "aide", "aids", "aim", "ajar", "alarm", "alias", "alibi", "alien",
	"alike", "alive", "alley", "allot", "allow", "alloy", "ally", "aloft", "alone", "along",
	"alpha", "alps", "altar", "alter", "amaze", "amber", "amend", "amino", "ample", "amuse",
	"angel", "anger", "angle", "angry", "ankle", "apple", "april", "apron", "arena", "argue",
	"arise", "armor", "army", "aroma", "array", "arrow", "arson", "artsy", "ascot", "ashen",
	"aside", "asked", "asset", "atom", "attic", "audio", "avert", "avoid", "awake", "award",
	"aware", "awful", "awoke", "axes", "axiom", "axis", "azure", "bacon", "badge", "badly",
	"bagel", "baggy", "baked", "baker", "balmy", "banal", "banjo", "barge", "baron", "basin",
	"batch", "bath", "baton", "bazaar", "beach", "beady", "beam", "beard", "beast", "beat",
	"begin", "begun", "being", "bell", "belly", "below", "belt", "bench", "berry", "beta",
	"bevel", "bevy", "bike", "bird", "birth", "black", "blade", "blame", "bland", "blank",
	"blast", "blaze", "bleak", "bleat", "bleed", "blend", "bless", "blimp", "blind", "blink",
	"bliss", "blitz", "bloat", "block", "blond", "blood", "bloom", "blown", "blue", "bluff",
	"blunt", "blur", "blurt", "blush", "board", "boast", "body", "bogus", "boil", "bold",
	"bolt", "bonus", "bony", "book", "booth", "boots", "booze", "bore", "born", "boss",
	"botch", "both", "bough", "bound", "bowed", "bowl", "boxer", "brace", "brain", "brake",
	"brand", "brass", "brave", "bravo", "brawl", "bread", "break", "breed", "brew", "brick",
	"bride", "brief", "brim", "bring", "brink", "brisk", "broad", "broil", "broke", "brook",
	"broom", "broth", "brown", "brunt", "brush", "brute", "buck", "buddy", "budge", "buggy",
	"build", "built", "bulge", "bulk", "bully", "bunch", "bunny", "burden", "burn", "burst",
	"bury", "bush", "bust", "busy", "buyer", "buzz", "cable", "cache", "cadet", "cage",
	"cake", "calm", "camel", "camp", "canal", "candy", "cane", "canon", "cape", "card",
	"cargo", "carol", "carry", "carve", "case", "cash", "cast", "catch", "cater", "cause",
	"cave", "cease", "cedar", "chain", "chair", "chalk", "champ", "chant", "chaos", "charm",
	"chart", "chase", "cheap", "cheat", "check", "cheek", "cheer", "chess", "chest", "chew",
	"chick", "chief", "child", "chill", "chimp", "china", "chip", "choke", "chomp", "chord",
	"chore", "chose", "chunk", "churn", "cider", "cigar", "cinch", "city", "civic", "civil",
	"clad", "claim", "clamp", "clang", "clank", "clash", "clasp", "class", "claw", "clay",
	"clean", "clear", "clerk", "click", "cliff", "climb", "cling", "clip", "cloak", "clock",
	"clone", "close", "cloth", "cloud", "clout", "clown", "club", "cluck", "clue", "clump",
	"clung", "coach", "coast", "coat", "cobra", "cocoa", "code", "coil", "coin", "coke",
	"cold", "colon", "color", "comet", "comic", "comma", "conch", "condo", "cone", "coral",
	"cord", "core", "cork", "corn", "cost", "cosy", "couch", "cough", "could", "count",
	"coup", "court", "cover", "covet", "crack", "craft", "cramp", "crane", "crank", "crash",
	"crate", "crave", "crawl", "craze", "crazy", "creak", "cream", "cred", "creek", "creep",
	"creme", "crepe", "crept", "crest", "crick", "cried", "crime", "crimp", "crisp", "croak",
	"crock", "crook", "crop", "cross", "crowd", "crown", "crude", "cruel", "crush", "crust",
	"crypt", "cube", "cult", "cupid", "curb", "cure", "curl", "curry", "curse", "curve",
	"cycle", "cynic", "daddy", "daily", "dairy", "daisy", "dance", "dandy", "dared", "dark",
	"dash", "data", "date", "dawn", "days", "daze", "dead", "deaf", "deal", "dealt",
	"dean", "dear", "death", "debit", "debt", "debug", "debut", "decal", "decay", "deck",
	"decor", "decoy", "decry", "deed", "deep", "deer", "delay", "delta", "delve", "demon",
	"denim", "dense", "dent", "depot", "depth", "derby", "desk", "deter", "devil", "diary",
	"dice", "diced", "dig", "digit", "dill", "dime", "dimly", "diner", "dingy", "disco",
	"dish", "disk", "ditch", "ditto", "ditty", "diver", "dizzy", "dock", "dodge", "doing",
	"doll", "dolly", "dome", "donor", "donut", "doom", "door", "dope", "dose", "dot",
	"doubt", "dough", "dove", "down", "dowry", "doze", "draft", "drain", "drake", "drama",
	"drank", "drape", "draw", "dread", "dream", "dress", "dried", "drift", "drill", "drink",
	"drip", "drive", "drone", "drool", "droop", "drop", "drown", "drum", "drunk", "dryer",
	"dual", "duck", "duct", "dude", "duel", "duet", "duke", "dull", "dummy", "dump",
	"dune", "dunk", "duo", "dusk", "dust", "dusty", "duty", "dwarf", "dwell", "eagle",
	"early", "earth", "ease", "east", "eaten", "eater", "ebony", "echo", "edge", "edgy",
	"edit", "eel", "eerie", "ego", "eight", "elbow", "elder", "elect", "elite", "elope",
	"elude", "elves", "email", "ember", "emit", "empty", "emu", "ended", "enemy", "enjoy",
	"enter", "entry", "envoy", "equal", "equip", "erase", "error", "erupt", "essay", "etch",
	"evade", "even", "event", "every", "evict", "evil", "evoke", "exact", "exam", "exert",
	"exile", "exist", "exit", "exotic", "expel", "extol", "extra", "exult", "fable", "faced",
	"facet", "fact", "fade", "fail", "faint", "fair", "fairy", "faith", "fake", "fall",
	"false", "fame", "fancy", "fang", "far", "farce", "farm", "fast", "fatal", "fate",
	"fatty", "fault", "fauna", "favor", "feast", "feat", "fed", "feeble", "feed", "feel",
	"feet", "feign", "feline", "fell", "felon", "felt", "femur", "fence", "fend", "fern",
	"ferry", "fest", "fetch", "fetid", "fetus", "feud", "fever", "fewer", "fiber", "fickle",
	"field", "fiend", "fiery", "fifth", "fifty", "fig", "fight", "filch", "file", "filet",
	"fill", "film", "filth", "final", "finch", "find", "fine", "finer", "fire", "firm",
	"first", "fish", "fist", "fit", "five", "fixer", "fizz", "flag", "flair", "flak",
	"flame", "flank", "flap", "flare", "flash", "flask", "flat", "flaw", "flax", "flea",
	"fled", "flee", "fleet", "flesh", "flex", "flick", "flier", "flight", "fling", "flint",
	"flip", "flirt", "float", "flock", "flood", "floor", "flop", "floss", "flour", "flow",
	"flown", "flue", "fluff", "fluid", "fluke", "flung", "flunk", "flush", "flute", "flux",
	"foal", "foam", "foe", "fog", "foggy", "foil", "fold", "folk", "folly", "fond",
	"font", "food", "fool", "foot", "for", "force", "forge", "fork", "form", "fort",
	"forth", "forty", "forum", "fossil", "foster", "foul", "found", "four", "fox", "foyer",
	"frail", "frame", "frank", "fraud", "fray", "freak", "free", "fresh", "friar", "fried",
	"friend", "fright", "frisk", "frizz", "frog", "from", "front", "frost", "froth", "frown",
	"froze", "fruit", "fudge", "fuel", "fully", "fume", "fund", "fungi", "funk", "funny",
	"fury", "fuse", "fuss", "fussy", "fuzzy", "gag", "gain", "gal", "gala", "gale",
	"gall", "game", "gamma", "gap", "gape", "garb", "gas", "gash", "gasp", "gate",
	"gauge", "gave", "gawk", "gaze", "gear", "gecko", "geek", "gem", "gene", "gent",
	"genus", "germ", "get", "ghost", "giant", "gift", "gig", "gild", "gill", "gilt",
}
