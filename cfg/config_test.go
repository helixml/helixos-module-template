package cfg

import (
	"net/http/httptest"
	"testing"
)

func clearEnv(t *testing.T) {
	for _, k := range []string{"MODULE_KEY", "MODULE_PUBLIC_URL", "HELIXOS_ISSUERS", "HELIXOS_MODULE_CREDENTIALS",
		"HELIXOS_MODULE_DEV_CREDENTIALS", "HELIXOS_ISSUER_ENDPOINTS", "MODULE_WEB_DIR", "MODULE_WEB_DEV_PROXY", "MODULE_DEV",
		"MODULE_FILE_SIGNING_KEY"} {
		t.Setenv(k, "")
	}
	t.Setenv("HELIXOS_ISSUERS", "https://os.example.com,http://localhost:8090")
}

func TestCredentialsPreferProd(t *testing.T) {
	clearEnv(t)
	t.Setenv("HELIXOS_MODULE_CREDENTIALS", "https://os.example.com=prod")
	t.Setenv("HELIXOS_MODULE_DEV_CREDENTIALS", "https://os.example.com=dev")
	c, err := Load("k")
	if err != nil || c.Credentials["https://os.example.com"] != "prod" {
		t.Fatalf("%v %v", c, err)
	}
}

func TestCredentialsFallBackToDev(t *testing.T) {
	clearEnv(t)
	t.Setenv("HELIXOS_MODULE_DEV_CREDENTIALS", "https://os.example.com=dev,http://localhost:8090=local")
	c, err := Load("k")
	if err != nil || c.Credentials["https://os.example.com"] != "dev" || c.Credentials["http://localhost:8090"] != "local" {
		t.Fatalf("%v %v", c, err)
	}
	t.Setenv("HELIXOS_MODULE_DEV_CREDENTIALS", "https://other.example.com=dev")
	if _, err := Load("k"); err == nil {
		t.Fatal("dev credential for an untrusted issuer accepted")
	}
}

func TestFileSigningKey(t *testing.T) {
	clearEnv(t)
	t.Setenv("MODULE_FILE_SIGNING_KEY", "fixed")
	if c, _ := Load("k"); c.FileSigningKey != "fixed" {
		t.Fatalf("key %q", c.FileSigningKey)
	}
	t.Setenv("MODULE_FILE_SIGNING_KEY", "")
	a, _ := Load("k")
	b, _ := Load("k")
	if len(a.FileSigningKey) != 64 || a.FileSigningKey == b.FileSigningKey {
		t.Fatalf("random keys %q %q", a.FileSigningKey, b.FileSigningKey)
	}
}

func TestPublicBase(t *testing.T) {
	clearEnv(t)
	c, _ := Load("k")
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/x", nil)
	if got := c.PublicBase(r); got != "http://127.0.0.1:8080" {
		t.Fatalf("from Host: %q", got)
	}
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "share-abc.dev.example.com, 10.0.0.1")
	if got := c.PublicBase(r); got != "https://share-abc.dev.example.com" {
		t.Fatalf("forwarded: %q", got)
	}
	t.Setenv("MODULE_PUBLIC_URL", "https://mod.example.com/")
	c, _ = Load("k")
	if got := c.PublicBase(r); got != "https://mod.example.com" {
		t.Fatalf("configured: %q", got)
	}
}

func TestWebDevProxy(t *testing.T) {
	clearEnv(t)
	t.Setenv("MODULE_WEB_DEV_PROXY", "http://127.0.0.1:3000/")
	if c, _ := Load("k"); c.WebDevProxy != "http://127.0.0.1:3000" || c.Dev {
		t.Fatalf("%+v", c)
	}
}
