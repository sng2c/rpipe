package secure

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/sng2c/rpipe/msgspec"
)

// --- AES-256-GCM round-trip ---

func TestEncryptDecryptMessage_RoundTrip(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	symKey := &SymKey{Key: key}

	plain := []byte("hello rpipe")
	cipher, err := EncryptMessage(symKey, plain)
	if err != nil {
		t.Fatalf("EncryptMessage: %v", err)
	}
	got, err := DecryptMessage(symKey, cipher)
	if err != nil {
		t.Fatalf("DecryptMessage: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("want %q, got %q", plain, got)
	}
}

func TestEncryptDecryptMessage_NonDeterministic(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	symKey := &SymKey{Key: key}

	plain := []byte("same message")
	c1, _ := EncryptMessage(symKey, plain)
	c2, _ := EncryptMessage(symKey, plain)
	if bytes.Equal(c1, c2) {
		t.Fatal("expected different ciphertexts due to random nonce")
	}
}

func TestDecryptMessage_TamperDetection(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	symKey := &SymKey{Key: key}

	cipher, _ := EncryptMessage(symKey, []byte("tamper me"))
	cipher[len(cipher)-1] ^= 0xff // flip last byte

	_, err := DecryptMessage(symKey, cipher)
	if err == nil {
		t.Fatal("expected decryption error on tampered ciphertext")
	}
}

func TestDecryptMessage_WrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	rand.Read(key1)
	rand.Read(key2)

	cipher, _ := EncryptMessage(&SymKey{Key: key1}, []byte("secret"))
	_, err := DecryptMessage(&SymKey{Key: key2}, cipher)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestDecryptMessage_TooShort(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	_, err := DecryptMessage(&SymKey{Key: key}, []byte("short"))
	if err == nil {
		t.Fatal("expected error on too-short ciphertext")
	}
}

// --- RSA OAEP round-trip ---

func TestEncryptDecryptPKI_RoundTrip(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)

	plain := make([]byte, 32)
	rand.Read(plain)

	enc, err := EncryptPKI(&priv.PublicKey, plain)
	if err != nil {
		t.Fatalf("EncryptPKI: %v", err)
	}
	dec, err := DecryptPKI(priv, enc)
	if err != nil {
		t.Fatalf("DecryptPKI: %v", err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatal("decrypted value does not match original")
	}
}

func TestDecryptPKI_WrongKey(t *testing.T) {
	priv1, _ := rsa.GenerateKey(rand.Reader, 2048)
	priv2, _ := rsa.GenerateKey(rand.Reader, 2048)

	enc, _ := EncryptPKI(&priv1.PublicKey, []byte("secret key"))
	_, err := DecryptPKI(priv2, enc)
	if err == nil {
		t.Fatal("expected error decrypting with wrong private key")
	}
}

// --- PEM encode/decode round-trip ---

func TestEncodeDecode_Pubkey(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	encoded := EncodePubkey(&priv.PublicKey)
	decoded := DecodePubkey(encoded)
	if decoded.N.Cmp(priv.PublicKey.N) != 0 {
		t.Fatal("pubkey mismatch after encode/decode")
	}
}

func TestEncodeDecode_Privkey(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	encoded := EncodePrivkey(priv)
	decoded := DecodePrivkey(encoded)
	if decoded.D.Cmp(priv.D) != 0 {
		t.Fatal("privkey mismatch after encode/decode")
	}
}

// --- InvalidateSymkey ---

func TestInvalidateSymkey_ClearsCache(t *testing.T) {
	c := &Cryptor{cache: make(map[string]*SymKey)}
	c.cache["alice:bob"] = &SymKey{Key: make([]byte, 32)}

	msg := &msgspec.RpipeMsg{From: "alice", To: "bob"}
	c.InvalidateSymkey(msg)

	if _, ok := c.cache["alice:bob"]; ok {
		t.Fatal("expected cache entry to be removed after InvalidateSymkey")
	}
}
