// Package cfg is the module's only reader of the environment. Everything else
// takes a *Config.
package cfg

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

// Config is the module's runtime configuration.
type Config struct {
	// Key is the module key (contract v1 §3). Defaults to the compiled key;
	// the "New module" flow sets MODULE_KEY so a fresh copy of the template
	// is installable before anyone edits it.
	Key string
	// PublicURL is this module's public origin, used to sign file URLs the
	// host downloads (MODULE_PUBLIC_URL). Optional: when empty, PublicBase
	// derives it from each request. Set it in production.
	PublicURL string
	// Issuers are the hosts whose tokens are accepted (HELIXOS_ISSUERS).
	Issuers []string
	// Credentials maps issuer → module credential for host API calls
	// (HELIXOS_MODULE_CREDENTIALS, "iss=credential" pairs). When that is
	// empty, HELIXOS_MODULE_DEV_CREDENTIALS is used instead: the dev-scoped
	// credential a HelixOS spec task gets, which only works for the module
	// owner's own installation.
	Credentials map[string]string
	// Endpoints maps issuer → base URL to reach the host at, when the public
	// issuer URL is not routable from here (HELIXOS_ISSUER_ENDPOINTS).
	Endpoints map[string]string
	// WebDir is the static UI build (Next.js export) to serve at /.
	WebDir string
	// WebDevProxy, when set (MODULE_WEB_DEV_PROXY, e.g. http://127.0.0.1:3000),
	// reverse-proxies UI paths to a `next dev` server instead of serving
	// WebDir, so the UI hot-reloads on the module's own origin. ./dev sets it.
	WebDevProxy string
	// Dev enables PocketBase's built-in APIs and dashboard. Never in prod.
	Dev bool
	// FileSigningKey signs short-lived file download URLs for the host
	// (MODULE_FILE_SIGNING_KEY). When unset, a random per-process key is
	// generated, so signed URLs stop working on restart.
	FileSigningKey string
}

// Load reads the environment.
func Load(defaultKey string) (*Config, error) {
	c := &Config{
		Key:            env("MODULE_KEY", defaultKey),
		PublicURL:      strings.TrimRight(os.Getenv("MODULE_PUBLIC_URL"), "/"),
		Issuers:        list(os.Getenv("HELIXOS_ISSUERS")),
		Credentials:    pairs(os.Getenv("HELIXOS_MODULE_CREDENTIALS")),
		Endpoints:      pairs(os.Getenv("HELIXOS_ISSUER_ENDPOINTS")),
		WebDir:         env("MODULE_WEB_DIR", "web/out"),
		WebDevProxy:    strings.TrimRight(strings.TrimSpace(os.Getenv("MODULE_WEB_DEV_PROXY")), "/"),
		Dev:            os.Getenv("MODULE_DEV") == "true",
		FileSigningKey: os.Getenv("MODULE_FILE_SIGNING_KEY"),
	}
	if len(c.Issuers) == 0 {
		return nil, fmt.Errorf("HELIXOS_ISSUERS is required (comma-separated host base URLs)")
	}
	credsVar := "HELIXOS_MODULE_CREDENTIALS"
	if len(c.Credentials) == 0 {
		if dev := pairs(os.Getenv("HELIXOS_MODULE_DEV_CREDENTIALS")); len(dev) > 0 {
			c.Credentials, credsVar = dev, "HELIXOS_MODULE_DEV_CREDENTIALS"
			log.Printf("cfg: using the dev-scoped HELIXOS_MODULE_DEV_CREDENTIALS (HELIXOS_MODULE_CREDENTIALS is empty)")
		}
	}
	for iss := range c.Credentials {
		if !c.Trusts(iss) {
			return nil, fmt.Errorf("%s names %q, which is not in HELIXOS_ISSUERS", credsVar, iss)
		}
	}
	if c.FileSigningKey == "" {
		k := make([]byte, 32)
		if _, err := rand.Read(k); err != nil {
			return nil, err
		}
		c.FileSigningKey = hex.EncodeToString(k)
		log.Printf("cfg: MODULE_FILE_SIGNING_KEY is not set; using a random per-process key (signed file URLs stop working on restart)")
	}
	return c, nil
}

// PublicBase is the module's public origin as seen by whoever made r:
// MODULE_PUBLIC_URL when set, otherwise derived from X-Forwarded-Proto /
// X-Forwarded-Host (set by the proxy in front of a dev or preview server) or
// Host. Used for URLs the module hands out, such as signed file links.
func (c *Config) PublicBase(r *http.Request) string {
	if c.PublicURL != "" {
		return c.PublicURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := firstValue(r.Header.Get("X-Forwarded-Proto")); p == "http" || p == "https" {
		scheme = p
	}
	host := r.Host
	if h := firstValue(r.Header.Get("X-Forwarded-Host")); h != "" {
		host = h
	}
	return scheme + "://" + host
}

// firstValue is the first entry of a comma-separated forwarding header (the
// one the outermost proxy saw).
func firstValue(v string) string {
	first, _, _ := strings.Cut(v, ",")
	return strings.TrimSpace(first)
}

// Trusts reports whether iss is a configured issuer.
func (c *Config) Trusts(iss string) bool {
	for _, i := range c.Issuers {
		if i == iss {
			return true
		}
	}
	return false
}

// HostBase is where to reach issuer iss server-to-server.
func (c *Config) HostBase(iss string) string {
	if e := c.Endpoints[iss]; e != "" {
		return e
	}
	return iss
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func list(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimRight(strings.TrimSpace(p), "/"); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// pairs parses "a=b,c=d". The key is the text before the FIRST "=", with any
// trailing "/" trimmed, so URLs work as keys.
func pairs(s string) map[string]string {
	out := map[string]string{}
	for _, p := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
		if !ok {
			continue
		}
		out[strings.TrimRight(strings.TrimSpace(k), "/")] = strings.TrimRight(strings.TrimSpace(v), "/")
	}
	return out
}
