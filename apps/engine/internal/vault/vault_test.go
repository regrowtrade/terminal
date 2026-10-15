package vault

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func randomValue() string {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		panic("random test data unavailable")
	}
	return hex.EncodeToString(data)
}
func testRing(t *testing.T) *Keyring {
	t.Helper()
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		t.Fatal("random test data unavailable")
	}
	raw, _ := json.Marshal(map[string]string{"test-key": base64.StdEncoding.EncodeToString(data)})
	k, err := New("test-key", string(raw))
	if err != nil {
		t.Fatal("could not create test keyring")
	}
	return k
}
func TestEnvelopeBindingAndTampering(t *testing.T) {
	k := testRing(t)
	c := Credentials{randomValue(), randomValue()}
	b := Binding{"owner-a", "account-a", "binance", "production"}
	e, err := k.Seal(b, c)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := k.Open(b, e)
	if err != nil || opened != c {
		t.Fatal("round trip failed")
	}
	other, _ := k.Seal(b, c)
	if string(other.Ciphertext) == string(e.Ciphertext) || string(other.WrappedKey) == string(e.WrappedKey) {
		t.Fatal("encryption is deterministic")
	}
	for _, altered := range []Binding{{"owner-b", b.AccountID, b.Exchange, b.Environment}, {b.OwnerID, "account-b", b.Exchange, b.Environment}, {b.OwnerID, b.AccountID, "bybit", b.Environment}, {b.OwnerID, b.AccountID, b.Exchange, "testnet"}} {
		if _, err := k.Open(altered, e); err == nil {
			t.Fatal("ciphertext can be moved across ownership/environment")
		}
	}
	for _, field := range []string{"ciphertext", "wrapped", "nonce", "wrap-nonce", "version", "key"} {
		copy := e
		copy.Ciphertext = append([]byte(nil), e.Ciphertext...)
		copy.WrappedKey = append([]byte(nil), e.WrappedKey...)
		switch field {
		case "ciphertext":
			copy.Ciphertext[0] ^= 1
		case "wrapped":
			copy.WrappedKey[0] ^= 1
		case "nonce":
			copy.Nonce = []byte{1}
		case "wrap-nonce":
			copy.WrapNonce = []byte{1}
		case "version":
			copy.Version = 2
		case "key":
			copy.KeyID = "missing"
		}
		if _, err := k.Open(b, copy); err == nil {
			t.Fatal("tampered envelope accepted")
		}
	}
	if _, err := testRing(t).Open(b, e); err == nil {
		t.Fatal("wrong KEK accepted")
	}
	encoded, _ := json.Marshal(e)
	if strings.Contains(string(encoded), c.APIKey) || strings.Contains(string(encoded), c.APISecret) {
		t.Fatal("plaintext stored in envelope")
	}
	for _, value := range []any{c, k} {
		for _, text := range []string{fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value), func() string { raw, _ := json.Marshal(value); return string(raw) }()} {
			if strings.Contains(text, c.APIKey) || strings.Contains(text, c.APISecret) || strings.Contains(text, base64.StdEncoding.EncodeToString(k.keys[k.active])) {
				t.Fatal("secret exposed by formatting")
			}
		}
	}
}
func TestRejectUnsafeKeyConfiguration(t *testing.T) {
	for _, raw := range []string{"", `{}`, `{"id":"replace-with-secret"}`, `{"id":"` + base64.StdEncoding.EncodeToString(make([]byte, 32)) + `"}`} {
		if _, err := New("id", raw); err == nil {
			t.Fatal("invalid key configuration accepted")
		}
	}
}
