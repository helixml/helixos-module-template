// Package devhost is a minimal local HelixOS-compatible host for developing a
// module without HelixOS: it publishes a JWKS, mints contract tokens (§4) for
// a fixed dev user and org, frames the module and speaks the UI frame
// protocol (§7), and answers the host API (§6) with whoami only. Every other
// host API call is 501 not_implemented, which modules must handle anyway
// (§6.2).
//
// `./dev` runs it on :8090 (`module devhost`); the integration tests use it
// in-process as their fake host.
//
// It is a development tool. It signs tokens for anyone who can reach it, so
// it listens on loopback by default and must never be a trusted issuer of a
// production deployment.
package devhost

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Options configure a Host. Zero values get the dev defaults.
type Options struct {
	// Issuer is the iss claim: the base URL modules fetch the JWKS from.
	// Set it later with SetIssuer when it is only known after listening.
	Issuer string
	// ModuleKey is the module the tokens are for (aud = module:<key>).
	ModuleKey string
	// ModuleURL is the module origin (and optional start path) to frame.
	ModuleURL string
	// Credential is the module credential the host API expects as
	// Authorization: Bearer. Empty accepts any non-empty credential.
	Credential string
	// The fixed dev identity tokens are minted for.
	Org, OrgName, Inst, UserID, Email, Name string
	// Scopes granted to the installation (reported by whoami).
	Scopes []string
	// TokenTTL is the user-token lifetime (contract: 5 minutes).
	TokenTTL time.Duration
}

func (o *Options) defaults() {
	def := func(p *string, v string) {
		if *p == "" {
			*p = v
		}
	}
	def(&o.Issuer, "http://localhost:8090")
	def(&o.ModuleKey, "module-template")
	def(&o.ModuleURL, "http://localhost:8080")
	def(&o.Org, "org_dev")
	def(&o.OrgName, "Dev Org")
	def(&o.Inst, "inst_dev")
	def(&o.UserID, "user_dev")
	def(&o.Email, "dev@example.com")
	def(&o.Name, "Dev User")
	o.Issuer = strings.TrimRight(o.Issuer, "/")
	if o.Scopes == nil {
		o.Scopes = []string{"identity.read", "agents.run"}
	}
	if o.TokenTTL == 0 {
		o.TokenTTL = 5 * time.Minute
	}
}

// Call is one recorded host API request.
type Call struct {
	Method, Path, Credential, Inst, UserToken string
	Body                                      map[string]interface{}
}

// Host is a running dev host. It is an http.Handler.
type Host struct {
	opts Options
	key  *ecdsa.PrivateKey
	kid  string
	mux  *http.ServeMux

	mu       sync.Mutex
	calls    []Call
	handlers map[string]http.HandlerFunc // "METHOD /path" under /api/module-host/v1
}

// New creates a host with a fresh ES256 key.
func New(o Options) (*Host, error) {
	o.defaults()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	// A new kid per process: a module that cached a previous run's JWKS
	// re-fetches on the unknown kid instead of failing.
	kid := make([]byte, 6)
	_, _ = rand.Read(kid)
	h := &Host{opts: o, key: k, kid: "dev-" + b64(kid), mux: http.NewServeMux(), handlers: map[string]http.HandlerFunc{}}
	h.mux.HandleFunc("GET /.well-known/jwks.json", h.jwks)
	h.mux.HandleFunc("/api/module-host/v1/", h.hostAPI)
	h.mux.HandleFunc("POST /devhost/token", h.frameToken)
	h.mux.HandleFunc("GET /devhost/config", h.config)
	h.mux.HandleFunc("GET /{$}", h.page)
	return h, nil
}

// SetIssuer sets the iss claim (e.g. an httptest server URL).
func (h *Host) SetIssuer(iss string) {
	h.mu.Lock()
	h.opts.Issuer = strings.TrimRight(iss, "/")
	h.mu.Unlock()
}

// Issuer is the iss claim this host signs.
func (h *Host) Issuer() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.opts.Issuer
}

// Options returns the effective options.
func (h *Host) Options() Options {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.opts
}

// Handle overrides one host API route, e.g. Handle("POST /runs", fn). The
// path is relative to /api/module-host/v1. Calls are recorded either way.
func (h *Host) Handle(route string, fn http.HandlerFunc) {
	h.mu.Lock()
	h.handlers[route] = fn
	h.mu.Unlock()
}

// Calls returns the host API calls received so far.
func (h *Host) Calls() []Call {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Call(nil), h.calls...)
}

// LastCall returns the most recent host API call (zero Call if none).
func (h *Host) LastCall() Call {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.calls) == 0 {
		return Call{}
	}
	return h.calls[len(h.calls)-1]
}

func (h *Host) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.mux.ServeHTTP(w, r) }

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (h *Host) jwks(w http.ResponseWriter, r *http.Request) {
	x, y := make([]byte, 32), make([]byte, 32)
	h.key.PublicKey.X.FillBytes(x)
	h.key.PublicKey.Y.FillBytes(y)
	writeJSON(w, http.StatusOK, map[string]interface{}{"keys": []map[string]string{{
		"kty": "EC", "crv": "P-256", "kid": h.kid, "x": b64(x), "y": b64(y), "use": "sig", "alg": "ES256"}}})
}

// Mint signs a contract token of type typ ("user" or "host") for org/inst.
// User tokens carry the dev identity; host tokens get a jti and a bot slug.
// mut may override any claim.
func (h *Host) Mint(typ, org, inst string, mut func(c map[string]interface{})) string {
	o := h.Options()
	now := time.Now().Unix()
	ttl := int64(60)
	if typ == "user" {
		ttl = int64(o.TokenTTL / time.Second)
	}
	c := map[string]interface{}{"iss": o.Issuer, "aud": "module:" + o.ModuleKey, "typ": typ, "org": org, "inst": inst,
		"sub": o.UserID, "iat": now, "nbf": now, "exp": now + ttl}
	switch typ {
	case "user":
		c["email"], c["name"] = o.Email, o.Name
	case "host":
		jti := make([]byte, 12)
		_, _ = rand.Read(jti)
		c["jti"] = b64(jti)
		c["bot"] = "m-" + o.ModuleKey + "-dev"
	}
	if mut != nil {
		mut(c)
	}
	return h.sign(c)
}

func (h *Host) sign(claims map[string]interface{}) string {
	hb, _ := json.Marshal(map[string]string{"alg": "ES256", "typ": "JWT", "kid": h.kid})
	pb, _ := json.Marshal(claims)
	in := b64(hb) + "." + b64(pb)
	d := sha256.Sum256([]byte(in))
	r, s, err := ecdsa.Sign(rand.Reader, h.key, d[:])
	if err != nil {
		panic(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return in + "." + b64(sig)
}

// frameToken answers the dev page's token request with a §7.2 `token`
// message body. The origin claim is the page's origin (§7.1).
func (h *Host) frameToken(w http.ResponseWriter, r *http.Request) {
	o := h.Options()
	origin := r.Header.Get("Origin")
	if origin == "" || origin == "null" {
		origin = requestOrigin(r)
	}
	exp := time.Now().Add(o.TokenTTL)
	tok := h.Mint("user", o.Org, o.Inst, func(c map[string]interface{}) {
		c["origin"] = origin
		c["exp"] = exp.Unix()
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token": tok, "expires_at": exp.UTC().Format(time.RFC3339),
		"user": map[string]string{"id": o.UserID, "email": o.Email, "name": o.Name},
		"org":  o.Org, "inst": o.Inst,
	})
}

func (h *Host) config(w http.ResponseWriter, r *http.Request) {
	o := h.Options()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"module_url": o.ModuleURL, "module_key": o.ModuleKey, "issuer": o.Issuer,
		"org": o.Org, "org_name": o.OrgName, "inst": o.Inst,
		"user": map[string]string{"id": o.UserID, "email": o.Email, "name": o.Name},
	})
}

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// hostAPI is /api/module-host/v1 (contract §6).
func (h *Host) hostAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/module-host/v1")
	var body map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&body)
	cred := r.Header.Get("Authorization")
	inst := r.Header.Get("X-HelixOS-Installation")
	h.mu.Lock()
	h.calls = append(h.calls, Call{Method: r.Method, Path: path, Credential: cred, Inst: inst,
		UserToken: r.Header.Get("X-HelixOS-User-Token"), Body: body})
	fn := h.handlers[r.Method+" "+path]
	o := h.opts
	h.mu.Unlock()

	given := strings.TrimSpace(strings.TrimPrefix(cred, "Bearer "))
	if !strings.HasPrefix(cred, "Bearer ") || given == "" || (o.Credential != "" && given != o.Credential) {
		apiErr(w, http.StatusUnauthorized, "unauthenticated", "missing or wrong module credential (HELIXOS_MODULE_CREDENTIALS for "+o.Issuer+")")
		return
	}
	if inst == "" {
		apiErr(w, http.StatusBadRequest, "invalid_request", "X-HelixOS-Installation is required")
		return
	}
	if fn != nil {
		fn(w, r)
		return
	}
	if r.Method == http.MethodGet && path == "/whoami" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"module": o.ModuleKey, "installation": inst, "org": o.Org, "org_name": o.OrgName,
			"scopes": o.Scopes, "host": "devhost", "host_version": "dev",
		})
		return
	}
	apiErr(w, http.StatusNotImplemented, "not_implemented", "the local dev host does not implement "+r.Method+" "+path+"; it works in HelixOS (Dev preview)")
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func apiErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}
