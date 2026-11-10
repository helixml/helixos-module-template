package devhost

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/helixml/helixos-module-template/internal/helixos"
)

func start(t *testing.T) (*Host, *httptest.Server) {
	t.Helper()
	h, err := New(Options{ModuleKey: "demo", Credential: "hxm_dev"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	h.SetIssuer(srv.URL)
	return h, srv
}

func call(t *testing.T, method, url string, hdr map[string]string) (int, map[string]interface{}) {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestTokensVerifyAgainstJWKS(t *testing.T) {
	h, srv := start(t)
	v := helixos.NewVerifier(helixos.Settings{ModuleKey: "demo",
		Trusts: func(iss string) bool { return iss == srv.URL }, HostBase: func(iss string) string { return iss }})
	c, err := v.VerifyUser(context.Background(), h.Mint("user", "org_dev", "inst_dev", nil))
	if err != nil || c.Org != "org_dev" || c.Inst != "inst_dev" || c.UserID != "user_dev" || c.Email != "dev@example.com" {
		t.Fatalf("user token: %+v %v", c, err)
	}
	if _, err := v.VerifyHost(context.Background(), h.Mint("host", "org_dev", "inst_dev", nil)); err != nil {
		t.Fatalf("host token: %v", err)
	}
	if _, err := v.VerifyUser(context.Background(), h.Mint("host", "org_dev", "inst_dev", nil)); err == nil {
		t.Fatal("host token accepted as user token")
	}
}

func TestFrameTokenCarriesPageOrigin(t *testing.T) {
	_, srv := start(t)
	code, out := call(t, "POST", srv.URL+"/devhost/token", map[string]string{"Origin": "http://localhost:8090"})
	if code != 200 || out["org"] != "org_dev" || out["inst"] != "inst_dev" || out["expires_at"] == "" {
		t.Fatalf("token: %d %v", code, out)
	}
	claims := decode(t, out["token"].(string))
	if claims["origin"] != "http://localhost:8090" || claims["typ"] != "user" || claims["aud"] != "module:demo" || claims["iss"] != srv.URL {
		t.Fatalf("claims %v", claims)
	}
}

func TestHostAPI(t *testing.T) {
	h, srv := start(t)
	auth := map[string]string{"Authorization": "Bearer hxm_dev", "X-HelixOS-Installation": "inst_dev"}
	code, who := call(t, "GET", srv.URL+"/api/module-host/v1/whoami", auth)
	if code != 200 || who["org"] != "org_dev" || who["module"] != "demo" || who["installation"] != "inst_dev" {
		t.Fatalf("whoami %d %v", code, who)
	}
	if code, e := call(t, "POST", srv.URL+"/api/module-host/v1/runs", auth); code != 501 || e["code"] != "not_implemented" {
		t.Fatalf("runs %d %v, want 501 not_implemented", code, e)
	}
	if code, e := call(t, "GET", srv.URL+"/api/module-host/v1/whoami", map[string]string{"Authorization": "Bearer wrong", "X-HelixOS-Installation": "inst_dev"}); code != 401 || e["code"] != "unauthenticated" {
		t.Fatalf("wrong credential %d %v", code, e)
	}
	if c := h.LastCall(); c.Path != "/whoami" || c.Credential != "Bearer wrong" {
		t.Fatalf("last call %+v", c)
	}
}

func TestPageFramesModule(t *testing.T) {
	_, srv := start(t)
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	for _, want := range []string{"<iframe", "allow-modals", `"helixos-module/1"`, "#helixos=1&host="} {
		if !strings.Contains(string(b), want) {
			t.Errorf("page lacks %q", want)
		}
	}
	code, c := call(t, "GET", srv.URL+"/devhost/config", nil)
	if code != 200 || c["module_url"] != "http://localhost:8080" || c["module_key"] != "demo" {
		t.Fatalf("config %d %v", code, c)
	}
}

func decode(t *testing.T, jwt string) map[string]interface{} {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %q", jwt)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var c map[string]interface{}
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}
