// Package integration boots the module's real router, routes and MCP endpoint
// in-process against a temporary PocketBase database and a fake host that
// publishes a JWKS and answers the host API. No network, no external host.
package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helixos-module-template/cfg"
	"github.com/helixml/helixos-module-template/internal/api"
	"github.com/helixml/helixos-module-template/internal/devhost"
	"github.com/helixml/helixos-module-template/internal/helixos"
	_ "github.com/helixml/helixos-module-template/migrations"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

const moduleKey = "module-template"

// fakeHost is internal/devhost (the same fake host ./dev runs on :8090),
// served by an httptest server, with POST /runs answered so agent runs can be
// tested.
type fakeHost struct {
	*devhost.Host
	srv *httptest.Server
}

func newFakeHost(t *testing.T) *fakeHost {
	t.Helper()
	dh, err := devhost.New(devhost.Options{ModuleKey: moduleKey, Credential: "hxm_test", Org: "org_a", OrgName: "Org A",
		Scopes: []string{"agents.run"}})
	if err != nil {
		t.Fatal(err)
	}
	h := &fakeHost{Host: dh, srv: httptest.NewServer(dh)}
	t.Cleanup(h.srv.Close)
	dh.SetIssuer(h.srv.URL)
	dh.Handle("POST /runs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{"run_id": "run_1", "status": "dispatched", "run_frame_url": h.srv.URL + "/runs/run_1/frame?t=x"})
	})
	return h
}

// token mints a contract token. Override fields via mut.
func (h *fakeHost) token(typ, org, inst string, mut func(c map[string]interface{})) string {
	return h.Mint(typ, org, inst, mut)
}

func (h *fakeHost) lastCall() devhost.Call { return h.LastCall() }

// boot starts the module against host.
func boot(t *testing.T, host *fakeHost) *httptest.Server {
	t.Helper()
	return bootWith(t, host, nil)
}

// bootWith starts the module against host after mut adjusts its config.
func bootWith(t *testing.T, host *fakeHost, mut func(c *cfg.Config)) *httptest.Server {
	t.Helper()
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	if err := app.RunAllMigrations(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.ResetBootstrapState() })
	c := &cfg.Config{Key: moduleKey, Issuers: []string{host.srv.URL}, Credentials: map[string]string{host.srv.URL: "hxm_test"},
		Endpoints: map[string]string{}, WebDir: "/nonexistent"}
	if mut != nil {
		mut(c)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	deps := &api.Deps{App: app, Cfg: c,
		Verifier: helixos.NewVerifier(helixos.Settings{ModuleKey: c.Key, Trusts: c.Trusts, HostBase: c.HostBase, HTTP: client}),
		Host:     &helixos.HostAPI{Credential: func(iss string) string { return c.Credentials[iss] }, HostBase: c.HostBase, HTTP: client},
		Files:    api.NewFileSigner("test-key"),
	}
	r, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	api.Register(&core.ServeEvent{App: app, Router: r}, deps)
	mux, err := r.BuildMux()
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func do(t *testing.T, method, url, token string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rdr)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestManifestAndHealth(t *testing.T) {
	host := newFakeHost(t)
	srv := boot(t, host)
	code, m := do(t, "GET", srv.URL+"/.well-known/helixos-module.json", "", nil)
	if code != 200 || m["key"] != moduleKey || m["schema_version"].(float64) != 1 {
		t.Fatalf("manifest %d %v", code, m)
	}
	if code, _ := do(t, "GET", srv.URL+"/healthz", "", nil); code != 200 {
		t.Fatalf("healthz %d", code)
	}
}

func TestTenantIsolation(t *testing.T) {
	host := newFakeHost(t)
	srv := boot(t, host)
	a := host.token("user", "org_a", "inst_a", nil)
	b := host.token("user", "org_b", "inst_b", nil)

	code, note := do(t, "POST", srv.URL+"/api/app/notes", a, map[string]string{"title": "secret of A"})
	if code != 201 {
		t.Fatalf("create %d %v", code, note)
	}
	_, list := do(t, "GET", srv.URL+"/api/app/notes", b, nil)
	if n := len(list["notes"].([]interface{})); n != 0 {
		t.Fatalf("org B sees %d of org A's notes", n)
	}
	if code, _ := do(t, "DELETE", srv.URL+"/api/app/notes/"+note["id"].(string), b, nil); code != 404 {
		t.Fatalf("org B deleting A's note: %d, want 404", code)
	}
	_, list = do(t, "GET", srv.URL+"/api/app/notes", a, nil)
	if n := len(list["notes"].([]interface{})); n != 1 {
		t.Fatalf("org A sees %d notes, want 1", n)
	}
}

func TestTokenRejections(t *testing.T) {
	host := newFakeHost(t)
	other := newFakeHost(t) // an untrusted issuer
	srv := boot(t, host)
	cases := map[string]string{
		"no token":           "",
		"untrusted issuer":   other.token("user", "org_a", "inst_a", nil),
		"host token as user": host.token("host", "org_a", "inst_a", nil),
		"wrong audience":     host.token("user", "org_a", "inst_a", func(c map[string]interface{}) { c["aud"] = "module:other" }),
		"expired":            host.token("user", "org_a", "inst_a", func(c map[string]interface{}) { c["exp"] = time.Now().Add(-time.Hour).Unix() }),
		"no installation":    host.token("user", "org_a", "", nil),
	}
	for name, tok := range cases {
		if code, _ := do(t, "GET", srv.URL+"/api/app/notes", tok, nil); code != 401 {
			t.Errorf("%s: %d, want 401", name, code)
		}
	}
	// An installation cannot move to another org.
	do(t, "GET", srv.URL+"/api/app/notes", host.token("user", "org_a", "inst_x", nil), nil)
	if code, _ := do(t, "GET", srv.URL+"/api/app/notes", host.token("user", "org_b", "inst_x", nil), nil); code != 401 {
		t.Errorf("installation moved org: %d, want 401", code)
	}
}

func TestMCP(t *testing.T) {
	host := newFakeHost(t)
	srv := boot(t, host)
	tok := host.token("host", "org_a", "inst_a", nil)
	code, res := do(t, "POST", srv.URL+"/mcp", tok, map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	tools := res["result"].(map[string]interface{})["tools"].([]interface{})
	if code != 200 || len(tools) != 2 {
		t.Fatalf("tools/list %d %v", code, res)
	}
	call := map[string]interface{}{"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]interface{}{"name": "module_template_note_add", "arguments": map[string]string{"title": "from agent"}}}
	tok2 := host.token("host", "org_a", "inst_a", nil)
	if _, res := do(t, "POST", srv.URL+"/mcp", tok2, call); res["result"].(map[string]interface{})["isError"] != false {
		t.Fatalf("tools/call %v", res)
	}
	// Replay of the same host token is refused.
	if code, _ := do(t, "POST", srv.URL+"/mcp", tok2, call); code != 401 {
		t.Fatalf("replayed host token: %d, want 401", code)
	}
	// A user token cannot call MCP.
	if code, _ := do(t, "POST", srv.URL+"/mcp", host.token("user", "org_a", "inst_a", nil), call); code != 401 {
		t.Fatalf("user token on MCP: %d, want 401", code)
	}
	_, list := do(t, "GET", srv.URL+"/api/app/notes", host.token("user", "org_a", "inst_a", nil), nil)
	if n := len(list["notes"].([]interface{})); n != 1 {
		t.Fatalf("agent note not stored for org_a: %d", n)
	}
}

func TestAskAgentUsesHostAPI(t *testing.T) {
	host := newFakeHost(t)
	srv := boot(t, host)
	user := host.token("user", "org_a", "inst_a", nil)
	_, note := do(t, "POST", srv.URL+"/api/app/notes", user, map[string]string{"title": "plan", "body": "do things"})
	code, run := do(t, "POST", srv.URL+"/api/app/notes/"+note["id"].(string)+"/agent", user, nil)
	if code != 201 || run["run_id"] != "run_1" {
		t.Fatalf("ask agent %d %v", code, run)
	}
	c := host.lastCall()
	if c.Path != "/runs" || c.Credential != "Bearer hxm_test" || c.Inst != "inst_a" || c.UserToken != user {
		t.Fatalf("host call = %+v", c)
	}
	if !strings.Contains(c.Body["prompt"].(string), "do things") {
		t.Fatalf("prompt = %v", c.Body["prompt"])
	}
}

func TestEventsDedupeAndRunCompletion(t *testing.T) {
	host := newFakeHost(t)
	srv := boot(t, host)
	user := host.token("user", "org_a", "inst_a", nil)
	_, note := do(t, "POST", srv.URL+"/api/app/notes", user, map[string]string{"title": "t"})
	do(t, "POST", srv.URL+"/api/app/notes/"+note["id"].(string)+"/agent", user, nil)

	ev := map[string]interface{}{"id": "evt_1", "type": "run.completed", "org": "org_a", "inst": "inst_a",
		"data": map[string]interface{}{"run_id": "run_1", "status": "completed"}}
	if code, _ := do(t, "POST", srv.URL+"/helixos/events", host.token("host", "org_a", "inst_a", nil), ev); code != 200 {
		t.Fatalf("event %d", code)
	}
	_, dup := do(t, "POST", srv.URL+"/helixos/events", host.token("host", "org_a", "inst_a", nil), ev)
	if dup["duplicate"] != true {
		t.Fatalf("second delivery not deduped: %v", dup)
	}
	_, list := do(t, "GET", srv.URL+"/api/app/notes", user, nil)
	if s := list["notes"].([]interface{})[0].(map[string]interface{})["run_status"]; s != "completed" {
		t.Fatalf("run_status = %v", s)
	}
	// An event whose envelope names another org than its token is refused.
	ev["id"], ev["org"] = "evt_2", "org_b"
	if code, _ := do(t, "POST", srv.URL+"/helixos/events", host.token("host", "org_a", "inst_a", nil), ev); code != 400 {
		t.Fatalf("mismatched event: %d, want 400", code)
	}
}

func TestPocketBaseBuiltinsHidden(t *testing.T) {
	host := newFakeHost(t)
	srv := boot(t, host)
	for _, p := range []string{"/_/", "/api/collections/notes/records", "/api/collections/_superusers/auth-with-password", "/api/realtime"} {
		if code, _ := do(t, "GET", srv.URL+p, "", nil); code != 404 {
			t.Errorf("%s: %d, want 404", p, code)
		}
	}
}
