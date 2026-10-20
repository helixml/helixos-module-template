// Package cfg is the module's only reader of the environment. Everything else
// takes a *Config.
package cfg

import (
	"fmt"
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
	// host downloads. Optional: derived from the request when empty.
	PublicURL string
	// Issuers are the hosts whose tokens are accepted (HELIXOS_ISSUERS).
	Issuers []string
	// Credentials maps issuer → module credential for host API calls
	// (HELIXOS_MODULE_CREDENTIALS, "iss=credential" pairs).
	Credentials map[string]string
	// Endpoints maps issuer → base URL to reach the host at, when the public
	// issuer URL is not routable from here (HELIXOS_ISSUER_ENDPOINTS).
	Endpoints map[string]string
	// WebDir is the static UI build (Next.js export) to serve at /.
	WebDir string
	// Dev enables PocketBase's built-in APIs and dashboard. Never in prod.
	Dev bool
	// FileSigningKey signs short-lived file download URLs for the host.
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
		Dev:            os.Getenv("MODULE_DEV") == "true",
		FileSigningKey: os.Getenv("MODULE_FILE_SIGNING_KEY"),
	}
	if len(c.Issuers) == 0 {
		return nil, fmt.Errorf("HELIXOS_ISSUERS is required (comma-separated host base URLs)")
	}
	for iss := range c.Credentials {
		if !c.Trusts(iss) {
			return nil, fmt.Errorf("HELIXOS_MODULE_CREDENTIALS names %q, which is not in HELIXOS_ISSUERS", iss)
		}
	}
	return c, nil
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
