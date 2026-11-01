//go:build linux

package freedesktop

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math/big"
)

// The encrypted session of the Secret Service, algorithm
// "dh-ietf1024-sha256-aes128-cbc-pkcs7" (Secret Service API 0.2,
// "Transfer of Secrets"), with the details of libsecret and gnome-keyring
// (egg-dh.c, secret-session.c, gkd-secret-session.c), which the
// specification leaves out:
//
//   - Diffie-Hellman over the 1024-bit MODP group 2 of RFC 2409
//     ("ietf-ike-grp-modp-1024", generator 2). Public keys travel as
//     big-endian unsigned bytes without leading zeros.
//   - The shared secret is padded with leading zeros to the prime's
//     length (128 bytes), then HKDF-SHA256 with no salt and empty info
//     gives the 16-byte AES key.
//   - Each secret is AES-128-CBC with PKCS#7 padding under a fresh random
//     IV, sent as the Secret's parameters.
//
// Everything is in the standard library (math/big, crypto/hkdf, crypto/aes,
// crypto/cipher). math/big is not constant time; the exchange stays on the
// local session bus, as libsecret's own.

// dhPrime is the 1024-bit MODP group 2 prime of RFC 2409, section 6.2.
var dhPrime, _ = new(big.Int).SetString(
	"FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD1"+
		"29024E088A67CC74020BBEA63B139B22514A08798E3404DD"+
		"EF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245"+
		"E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7ED"+
		"EE386BFB5A899FA5AE9F24117C4B1FE649286651ECE65381"+
		"FFFFFFFFFFFFFFFF", 16)

var dhGenerator = big.NewInt(2)

// dhPrimeLen is the prime's length in bytes: the shared secret is padded
// to it.
const dhPrimeLen = 128

// dhKey is the client's key pair of one session.
type dhKey struct {
	priv *big.Int
	pub  []byte
}

// newDHKey makes a key pair from rand: a private exponent as long as the
// prime (as libsecret's egg_dh_gen_pair), in [2, p-2].
func newDHKey(rand io.Reader) (*dhKey, error) {
	b := make([]byte, dhPrimeLen)
	if _, err := io.ReadFull(rand, b); err != nil {
		return nil, err
	}
	pm3 := new(big.Int).Sub(dhPrime, big.NewInt(3))
	x := new(big.Int).SetBytes(b)
	x.Mod(x, pm3).Add(x, big.NewInt(2))
	return &dhKey{priv: x, pub: new(big.Int).Exp(dhGenerator, x, dhPrime).Bytes()}, nil
}

// aesKey derives the session's AES-128 key from the service's public key.
func (k *dhKey) aesKey(peer []byte) ([]byte, error) {
	y := new(big.Int).SetBytes(peer)
	pm1 := new(big.Int).Sub(dhPrime, big.NewInt(1))
	if y.Cmp(big.NewInt(1)) <= 0 || y.Cmp(pm1) >= 0 {
		return nil, errors.New("freedesktop: the Secret Service sent an invalid public key")
	}
	shared := new(big.Int).Exp(y, k.priv, dhPrime).FillBytes(make([]byte, dhPrimeLen))
	return hkdf.Key(sha256.New, shared, nil, "", 16)
}

// sealSecret encrypts plain with key under a fresh IV from rand.
func sealSecret(key, plain []byte, rand io.Reader) (iv, ciphertext []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	ciphertext = append(bytes.Clone(plain), bytes.Repeat([]byte{byte(pad)}, pad)...)
	iv = make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand, iv); err != nil {
		return nil, nil, err
	}
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, ciphertext)
	return iv, ciphertext, nil
}

// openSecret decrypts a secret sealed by sealSecret (or the service).
func openSecret(key, iv, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(iv) != aes.BlockSize || len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("freedesktop: the Secret Service sent a malformed secret (%d-byte IV, %d bytes)", len(iv), len(ciphertext))
	}
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ciphertext)
	pad := int(plain[len(plain)-1])
	if pad == 0 || pad > aes.BlockSize || !bytes.Equal(plain[len(plain)-pad:], bytes.Repeat([]byte{byte(pad)}, pad)) {
		return nil, errors.New("freedesktop: the Secret Service sent a secret with bad padding")
	}
	return plain[:len(plain)-pad], nil
}
