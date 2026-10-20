package helixos

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Caller is a verified request, from a user (UI → API) or from a host (MCP,
// events). Every read and write a module does is scoped by Tenant().
type Caller struct {
	Issuer  string // the host
	Org     string
	Inst    string
	UserID  string
	Email   string
	Name    string
	Bot     string
	Run     string
	Purpose string
	Token   string // the raw user token, forwarded on user-attributed host calls
}

// Tenant identifies the data partition: one org on one host.
func (c *Caller) Tenant() Tenant { return Tenant{Host: c.Issuer, Org: c.Org} }

// Tenant is the (host, org) pair every record is stored under.
type Tenant struct {
	Host string
	Org  string
}

// Settings configure a Verifier.
type Settings struct {
	ModuleKey string
	// Trusts reports whether an issuer is configured.
	Trusts func(iss string) bool
	// HostBase maps an issuer to the base URL to fetch its JWKS from.
	HostBase func(iss string) string
	HTTP     *http.Client
}

// Verifier checks host tokens against each trusted issuer's JWKS.
type Verifier struct {
	s   Settings
	mu  sync.Mutex
	set map[string]*keySet // by issuer
	// seen holds host-token jtis until they expire (replay protection).
	seen map[string]time.Time
	// instOrg remembers each installation's org, so a token cannot move an
	// installation to another org.
	instOrg map[string]string
}

type keySet struct {
	keys      map[string]*ecdsa.PublicKey
	fetchedAt time.Time
}

const (
	jwksTTL        = 10 * time.Minute
	jwksMinRefetch = 30 * time.Second
	maxJWKSBytes   = 64 << 10
)

// NewVerifier builds a verifier.
func NewVerifier(s Settings) *Verifier {
	if s.HTTP == nil {
		s.HTTP = &http.Client{Timeout: 10 * time.Second}
	}
	return &Verifier{s: s, set: map[string]*keySet{}, seen: map[string]time.Time{}, instOrg: map[string]string{}}
}

// Audience is this module's token audience.
func (v *Verifier) Audience() string { return "module:" + v.s.ModuleKey }

// VerifyUser checks a user token (Authorization: Bearer from the module UI).
func (v *Verifier) VerifyUser(ctx context.Context, token string) (*Caller, error) {
	c, err := v.verify(ctx, token, TokenUser)
	if err != nil {
		return nil, err
	}
	return &Caller{Issuer: c.Issuer, Org: c.Org, Inst: c.Inst, UserID: c.Subject, Email: c.Email, Name: c.Name, Purpose: c.Purpose, Token: token}, nil
}

// VerifyHost checks a host-call token (MCP, events, migration) and rejects a
// replayed jti.
func (v *Verifier) VerifyHost(ctx context.Context, token string) (*Caller, error) {
	c, err := v.verify(ctx, token, TokenHost)
	if err != nil {
		return nil, err
	}
	if c.ID == "" {
		return nil, fmt.Errorf("%w: host token without jti", ErrInvalidToken)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	now := time.Now()
	for k, exp := range v.seen {
		if now.After(exp) {
			delete(v.seen, k)
		}
	}
	key := c.Issuer + "|" + c.ID
	if _, dup := v.seen[key]; dup {
		return nil, fmt.Errorf("%w: replayed", ErrInvalidToken)
	}
	v.seen[key] = time.Unix(c.ExpiresAt, 0).Add(leeway)
	return &Caller{Issuer: c.Issuer, Org: c.Org, Inst: c.Inst, UserID: c.Subject, Bot: c.Bot, Run: c.Run, Purpose: c.Purpose}, nil
}

func (v *Verifier) verify(ctx context.Context, token, typ string) (*Claims, error) {
	iss, kid, err := peek(token)
	if err != nil {
		return nil, err
	}
	if !v.s.Trusts(iss) {
		return nil, fmt.Errorf("%w: untrusted issuer %q", ErrInvalidToken, iss)
	}
	pub, err := v.key(ctx, iss, kid)
	if err != nil {
		return nil, err
	}
	c, err := verifyWith(token, pub, v.Audience(), typ, time.Now())
	if err != nil {
		return nil, err
	}
	if c.Issuer != iss {
		return nil, fmt.Errorf("%w: issuer", ErrInvalidToken)
	}
	if c.Inst != "" {
		v.mu.Lock()
		k := iss + "|" + c.Inst
		if prev, ok := v.instOrg[k]; ok && prev != c.Org {
			v.mu.Unlock()
			return nil, fmt.Errorf("%w: installation %s moved org", ErrInvalidToken, c.Inst)
		}
		v.instOrg[k] = c.Org
		v.mu.Unlock()
	}
	return c, nil
}

func (v *Verifier) key(ctx context.Context, iss, kid string) (*ecdsa.PublicKey, error) {
	v.mu.Lock()
	ks := v.set[iss]
	v.mu.Unlock()
	if ks != nil {
		if k := ks.keys[kid]; k != nil && time.Since(ks.fetchedAt) < jwksTTL {
			return k, nil
		}
		if ks.keys[kid] == nil && time.Since(ks.fetchedAt) < jwksMinRefetch {
			return nil, fmt.Errorf("%w: unknown kid %q", ErrInvalidToken, kid)
		}
	}
	fresh, err := v.fetch(ctx, iss)
	if err != nil {
		if ks != nil && ks.keys[kid] != nil {
			return ks.keys[kid], nil // host briefly unreachable: keep using a known key
		}
		return nil, fmt.Errorf("%w: jwks: %v", ErrInvalidToken, err)
	}
	v.mu.Lock()
	v.set[iss] = fresh
	v.mu.Unlock()
	if k := fresh.keys[kid]; k != nil {
		return k, nil
	}
	return nil, fmt.Errorf("%w: unknown kid %q", ErrInvalidToken, kid)
}

func (v *Verifier) fetch(ctx context.Context, iss string) (*keySet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(v.s.HostBase(iss), "/")+"/.well-known/jwks.json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := v.s.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var doc struct {
		Keys []JWK `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxJWKSBytes)).Decode(&doc); err != nil {
		return nil, err
	}
	ks := &keySet{keys: map[string]*ecdsa.PublicKey{}, fetchedAt: time.Now()}
	for _, k := range doc.Keys {
		if pub, err := parseJWK(k); err == nil {
			ks.keys[k.Kid] = pub
		}
	}
	return ks, nil
}

// CallerKey is the request-store key under which middleware puts the *Caller.
const CallerKey = "helixos.caller"
