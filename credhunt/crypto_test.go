package credhunt

import "testing"

func TestEncryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	plain := []byte(`{"access_token":"secret","refresh_token":"r"}`)
	sealed, err := encrypt(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	if string(sealed) == string(plain) {
		t.Fatal("ciphertext equals plaintext")
	}
	got, err := decrypt(key, sealed)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(plain) {
		t.Fatalf("round trip = %q", got)
	}
}

func TestFingerprintJSONCanonical(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	a := fingerprint(key, KindOAuth, `{"b":2,"a":1}`)
	b := fingerprint(key, KindOAuth, `{ "a": 1, "b": 2 }`)
	if a != b {
		t.Fatal("canonical JSON fingerprints differ")
	}
	c := fingerprint(key, KindOAuth, `{"a":1,"b":3}`)
	if a == c {
		t.Fatal("different values share a fingerprint")
	}
}

func TestMaskSecret(t *testing.T) {
	if got := maskSecret("sk-1234567890abcdef"); got != "sk-123…cdef" {
		t.Fatalf("mask = %q", got)
	}
	if got := maskSecret("short"); got != "sh…" {
		t.Fatalf("short mask = %q", got)
	}
}
