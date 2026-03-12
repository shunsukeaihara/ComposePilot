package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptString(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	cipher, err := New(key)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	cipherText, err := cipher.EncryptString("secret-value")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}
	plain, err := cipher.DecryptString(cipherText)
	if err != nil {
		t.Fatalf("DecryptString() error = %v", err)
	}
	if plain != "secret-value" {
		t.Fatalf("plain = %q, want %q", plain, "secret-value")
	}
}
