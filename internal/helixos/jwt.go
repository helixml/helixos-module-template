// Package helixos is the module side of the HelixOS module contract v1
// (docs/modules/contract-v1.md in helixml/helixos): token verification against
// each trusted host's JWKS, request middleware, and the host API client.
//
// It works with any conforming host (HelixOS, the cyber portal). Copy it into
// a module unchanged; it has no dependencies beyond the standard library.
package helixos

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Token types (contract §4.3).
const (
	TokenUser = "user"
	TokenHost = "host"
)

// Claims are the contract §4.2 claims.
type Claims struct {
	Issuer    string `json:"iss"`
	Audience  string `json:"aud"`
	Type      string `json:"typ"`
	Subject   string `json:"sub,omitempty"`
	Email     string `json:"email,omitempty"`
	Name      string `json:"name,omitempty"`
	Org       string `json:"org"`
	Inst      string `json:"inst,omitempty"`
	Brand     string `json:"brand,omitempty"`
	Origin    string `json:"origin,omitempty"`
	Bot       string `json:"bot,omitempty"`
	Run       string `json:"run,omitempty"`
	Purpose   string `json:"purpose,omitempty"`
	ID        string `json:"jti,omitempty"`
	IssuedAt  int64  `json:"iat"`
	NotBefore int64  `json:"nbf"`
	ExpiresAt int64  `json:"exp"`
}

// ErrInvalidToken wraps every verification failure.
var ErrInvalidToken = errors.New("invalid token")

var b64 = base64.RawURLEncoding

const leeway = 30 * time.Second

type header struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

// peek decodes a token's issuer and kid without verifying it, so the right
// JWKS can be chosen. Nothing from it is trusted until Verify succeeds.
func peek(token string) (iss, kid string, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", "", fmt.Errorf("%w: malformed", ErrInvalidToken)
	}
	hb, err := b64.DecodeString(parts[0])
	if err != nil {
		return "", "", fmt.Errorf("%w: header", ErrInvalidToken)
	}
	var h header
	if json.Unmarshal(hb, &h) != nil || h.Alg != "ES256" {
		return "", "", fmt.Errorf("%w: alg", ErrInvalidToken)
	}
	pb, err := b64.DecodeString(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("%w: payload", ErrInvalidToken)
	}
	var c struct {
		Iss string `json:"iss"`
	}
	if json.Unmarshal(pb, &c) != nil {
		return "", "", fmt.Errorf("%w: payload", ErrInvalidToken)
	}
	return c.Iss, h.Kid, nil
}

// verifyWith checks the signature with pub and the claims against want.
func verifyWith(token string, pub *ecdsa.PublicKey, aud, typ string, now time.Time) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: malformed", ErrInvalidToken)
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return nil, fmt.Errorf("%w: signature", ErrInvalidToken)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(pub, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		return nil, fmt.Errorf("%w: bad signature", ErrInvalidToken)
	}
	pb, _ := b64.DecodeString(parts[1])
	var c Claims
	if err := json.Unmarshal(pb, &c); err != nil {
		return nil, fmt.Errorf("%w: payload", ErrInvalidToken)
	}
	switch {
	case c.Audience != aud:
		return nil, fmt.Errorf("%w: audience", ErrInvalidToken)
	case c.Type != typ:
		return nil, fmt.Errorf("%w: type %q", ErrInvalidToken, c.Type)
	case now.Add(-leeway).Unix() >= c.ExpiresAt:
		return nil, fmt.Errorf("%w: expired", ErrInvalidToken)
	case now.Add(leeway).Unix() < c.NotBefore:
		return nil, fmt.Errorf("%w: not yet valid", ErrInvalidToken)
	case c.Org == "":
		return nil, fmt.Errorf("%w: no org", ErrInvalidToken)
	case typ == TokenUser && c.Inst == "":
		return nil, fmt.Errorf("%w: no installation", ErrInvalidToken)
	}
	return &c, nil
}

// JWK is one published key.
type JWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	Kid string `json:"kid"`
}

func parseJWK(k JWK) (*ecdsa.PublicKey, error) {
	if k.Kty != "EC" || k.Crv != "P-256" {
		return nil, fmt.Errorf("unsupported key %s/%s", k.Kty, k.Crv)
	}
	x, err := b64.DecodeString(k.X)
	if err != nil {
		return nil, err
	}
	y, err := b64.DecodeString(k.Y)
	if err != nil {
		return nil, err
	}
	pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
	if _, err := pub.ECDH(); err != nil {
		return nil, err
	}
	return pub, nil
}
