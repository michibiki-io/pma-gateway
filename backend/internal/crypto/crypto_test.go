package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptCredentialPassword(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	nonce, encrypted, err := cipher.Encrypt([]byte("secret-password"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte("secret-password")) {
		t.Fatal("ciphertext contains plaintext")
	}
	plain, err := cipher.Decrypt(nonce, encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "secret-password" {
		t.Fatalf("decrypted password = %q", plain)
	}
}
