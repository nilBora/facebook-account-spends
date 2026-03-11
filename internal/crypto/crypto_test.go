package crypto

import (
	"strings"
	"testing"
)

var testKey = []byte("12345678901234567890123456789012") // 32 bytes

func TestEncryptDecryptRoundtrip(t *testing.T) {
	cases := []string{
		"EAAtest_access_token_value",
		"",
		strings.Repeat("x", 1024),
		"unicode: привіт 🌍",
	}
	for _, plain := range cases {
		enc, err := Encrypt(plain, testKey)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", plain, err)
		}
		got, err := Decrypt(enc, testKey)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if got != plain {
			t.Errorf("roundtrip failed: want %q, got %q", plain, got)
		}
	}
}

func TestEncryptProducesUniqueNonces(t *testing.T) {
	a, _ := Encrypt("same plaintext", testKey)
	b, _ := Encrypt("same plaintext", testKey)
	if a == b {
		t.Error("two encryptions of the same plaintext should differ (random nonce)")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	wrongKey := []byte("00000000000000000000000000000000")
	enc, _ := Encrypt("secret", testKey)
	if _, err := Decrypt(enc, wrongKey); err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestDecryptInvalidBase64(t *testing.T) {
	if _, err := Decrypt("not-valid-base64!!!", testKey); err == nil {
		t.Error("expected error for invalid base64 input")
	}
}

func TestDecryptTooShort(t *testing.T) {
	// Valid base64 but too short to contain nonce + ciphertext.
	if _, err := Decrypt("YQ==", testKey); err == nil {
		t.Error("expected error for ciphertext shorter than nonce size")
	}
}

func TestEncryptWrongKeySize(t *testing.T) {
	shortKey := []byte("short")
	if _, err := Encrypt("text", shortKey); err == nil {
		t.Error("expected error for invalid key size")
	}
}
