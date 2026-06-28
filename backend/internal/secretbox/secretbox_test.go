package secretbox_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"rekenraam/backend/internal/secretbox"
)

func key32(b byte) []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = b
	}
	return k
}

func TestRoundTrip(t *testing.T) {
	box, err := secretbox.New(key32(0x42))
	if err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("super-secret-api-key-12345")
	ciphertext, err := box.Seal(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	got, err := box.Open(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(plaintext) {
		t.Fatalf("got %q, want %q", got, plaintext)
	}
}

func TestEmptyPlaintext(t *testing.T) {
	box, _ := secretbox.New(key32(0x01))
	ct, err := box.Seal([]byte{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := box.Open(ct)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestFreshNoncePerSeal(t *testing.T) {
	box, _ := secretbox.New(key32(0x07))
	plaintext := []byte("same-plaintext")

	ct1, _ := box.Seal(plaintext)
	ct2, _ := box.Seal(plaintext)

	// Ciphertexts must differ (different nonces).
	if ct1 == ct2 {
		t.Fatal("two Seal calls produced identical ciphertext — nonce is not fresh")
	}
}

func TestTamperedCiphertextFails(t *testing.T) {
	box, _ := secretbox.New(key32(0x99))
	ct, _ := box.Seal([]byte("secret"))

	raw, _ := base64.StdEncoding.DecodeString(ct)
	// Flip one bit in the ciphertext body (after the 12-byte nonce).
	raw[12] ^= 0xFF
	tampered := base64.StdEncoding.EncodeToString(raw)

	_, err := box.Open(tampered)
	if err != secretbox.ErrTampered {
		t.Fatalf("expected ErrTampered, got %v", err)
	}
}

func TestTamperedNonceFails(t *testing.T) {
	box, _ := secretbox.New(key32(0xAA))
	ct, _ := box.Seal([]byte("secret"))

	raw, _ := base64.StdEncoding.DecodeString(ct)
	// Flip a nonce byte.
	raw[0] ^= 0x01
	tampered := base64.StdEncoding.EncodeToString(raw)

	_, err := box.Open(tampered)
	if err != secretbox.ErrTampered {
		t.Fatalf("expected ErrTampered, got %v", err)
	}
}

func TestWrongKeyFails(t *testing.T) {
	box1, _ := secretbox.New(key32(0x11))
	box2, _ := secretbox.New(key32(0x22))

	ct, _ := box1.Seal([]byte("secret"))
	_, err := box2.Open(ct)
	if err != secretbox.ErrTampered {
		t.Fatalf("expected ErrTampered with wrong key, got %v", err)
	}
}

func TestInvalidBase64Fails(t *testing.T) {
	box, _ := secretbox.New(key32(0x33))
	_, err := box.Open("not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestTooShortCiphertextFails(t *testing.T) {
	box, _ := secretbox.New(key32(0x44))
	// A valid base64 string that decodes to fewer than 12 bytes.
	short := base64.StdEncoding.EncodeToString([]byte("short"))
	_, err := box.Open(short)
	if err != secretbox.ErrTampered {
		t.Fatalf("expected ErrTampered for too-short ciphertext, got %v", err)
	}
}

func TestInvalidKeyLength(t *testing.T) {
	_, err := secretbox.New([]byte("too-short"))
	if err != secretbox.ErrInvalidKey {
		t.Fatalf("expected ErrInvalidKey, got %v", err)
	}

	_, err = secretbox.New(make([]byte, 16)) // AES-128 key (wrong size)
	if err != secretbox.ErrInvalidKey {
		t.Fatalf("expected ErrInvalidKey for 16-byte key, got %v", err)
	}

	_, err = secretbox.New(make([]byte, 33)) // too long
	if err != secretbox.ErrInvalidKey {
		t.Fatalf("expected ErrInvalidKey for 33-byte key, got %v", err)
	}
}

func TestCiphertextIsBase64(t *testing.T) {
	box, _ := secretbox.New(key32(0x55))
	ct, _ := box.Seal([]byte("test"))
	// Should decode without error.
	raw, err := base64.StdEncoding.DecodeString(ct)
	if err != nil {
		t.Fatalf("ciphertext is not valid base64: %v", err)
	}
	// Should be nonce (12) + at minimum GCM tag (16) bytes for empty plaintext.
	if len(raw) < 12+16 {
		t.Fatalf("ciphertext too short: %d bytes", len(raw))
	}
}

func TestSealDoesNotLeakPlaintext(t *testing.T) {
	box, _ := secretbox.New(key32(0x66))
	secret := "this-must-not-appear"
	ct, _ := box.Seal([]byte(secret))
	if strings.Contains(ct, secret) {
		t.Fatal("plaintext appears in ciphertext — no encryption")
	}
}
