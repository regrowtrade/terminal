// Package vault encrypts credentials with a per-record data key. KEKs belong
// only in the engine/admin secret manager, never the database or web service.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
)

var ErrVault = errors.New("credenciales cifradas no disponibles")
var keyID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)
var apiValue = regexp.MustCompile(`^[a-zA-Z0-9]{32,256}$`)

type Credentials struct{ APIKey, APISecret string }

func (Credentials) String() string               { return "[credenciales omitidas]" }
func (Credentials) GoString() string             { return "[credenciales omitidas]" }
func (Credentials) MarshalJSON() ([]byte, error) { return []byte(`"[credenciales omitidas]"`), nil }
func (c Credentials) Valid() bool {
	return apiValue.MatchString(c.APIKey) && apiValue.MatchString(c.APISecret)
}

type Binding struct{ OwnerID, AccountID, Exchange, Environment string }
type Envelope struct {
	Version    int    `json:"version"`
	KeyID      string `json:"keyId"`
	WrappedKey []byte `json:"wrappedKey"`
	WrapNonce  []byte `json:"wrapNonce"`
	Ciphertext []byte `json:"ciphertext"`
	Nonce      []byte `json:"nonce"`
}
type Keyring struct {
	active string
	keys   map[string][]byte
}

func (*Keyring) String() string               { return "[keyring omitido]" }
func (*Keyring) GoString() string             { return "[keyring omitido]" }
func (*Keyring) MarshalJSON() ([]byte, error) { return []byte(`"[keyring omitido]"`), nil }

func New(active, encoded string) (*Keyring, error) {
	var values map[string]string
	if !keyID.MatchString(active) || len(encoded) > 4096 || json.Unmarshal([]byte(encoded), &values) != nil || len(values) == 0 || len(values) > 8 {
		return nil, ErrVault
	}
	k := &Keyring{active: active, keys: make(map[string][]byte)}
	for id, value := range values {
		key, err := base64.StdEncoding.Strict().DecodeString(value)
		if err != nil || len(key) != 32 || !keyID.MatchString(id) {
			return nil, ErrVault
		}
		var nonzero byte
		for _, b := range key {
			nonzero |= b
		}
		if nonzero == 0 {
			return nil, ErrVault
		}
		k.keys[id] = key
	}
	if k.keys[active] == nil {
		return nil, ErrVault
	}
	return k, nil
}

func aad(b Binding, purpose string) []byte {
	data, _ := json.Marshal([]string{"regrow-credentials-v1", b.OwnerID, b.AccountID, b.Exchange, b.Environment, purpose})
	return data
}
func gcm(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrVault
	}
	a, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrVault
	}
	return a, nil
}
func seal(key, plain, context []byte) ([]byte, []byte, error) {
	a, err := gcm(key)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, ErrVault
	}
	return a.Seal(nil, nonce, plain, context), nonce, nil
}
func open(key, encrypted, nonce, context []byte) ([]byte, error) {
	a, err := gcm(key)
	if err != nil || len(nonce) != 12 || len(encrypted) < 16 || len(encrypted) > 4096 {
		return nil, ErrVault
	}
	plain, err := a.Open(nil, nonce, encrypted, context)
	if err != nil {
		return nil, ErrVault
	}
	return plain, nil
}
func (k *Keyring) Seal(b Binding, c Credentials) (Envelope, error) {
	var e Envelope
	if k == nil || !c.Valid() || b.OwnerID == "" || b.AccountID == "" || b.Exchange != "binance" || b.Environment != "production" {
		return e, ErrVault
	}
	plain, err := json.Marshal(struct {
		Key    string `json:"key"`
		Secret string `json:"secret"`
	}{c.APIKey, c.APISecret})
	if err != nil {
		return e, ErrVault
	}
	defer clear(plain)
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return e, ErrVault
	}
	defer clear(dek)
	e = Envelope{Version: 1, KeyID: k.active}
	e.Ciphertext, e.Nonce, err = seal(dek, plain, aad(b, "data"))
	if err != nil {
		return Envelope{}, err
	}
	e.WrappedKey, e.WrapNonce, err = seal(k.keys[k.active], dek, aad(b, "wrap:"+e.KeyID))
	if err != nil {
		return Envelope{}, err
	}
	return e, nil
}
func (k *Keyring) Open(b Binding, e Envelope) (Credentials, error) {
	var c Credentials
	if k == nil || e.Version != 1 || k.keys[e.KeyID] == nil {
		return c, ErrVault
	}
	dek, err := open(k.keys[e.KeyID], e.WrappedKey, e.WrapNonce, aad(b, "wrap:"+e.KeyID))
	if err != nil {
		return c, err
	}
	defer clear(dek)
	plain, err := open(dek, e.Ciphertext, e.Nonce, aad(b, "data"))
	if err != nil {
		return c, err
	}
	defer clear(plain)
	var data struct {
		Key    string `json:"key"`
		Secret string `json:"secret"`
	}
	if json.Unmarshal(plain, &data) != nil {
		return c, ErrVault
	}
	c = Credentials{APIKey: data.Key, APISecret: data.Secret}
	if !c.Valid() {
		return Credentials{}, ErrVault
	}
	return c, nil
}
