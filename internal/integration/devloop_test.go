package integration

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helixos-module-template/cfg"
)

// fakeNextDev stands in for `next dev`: it echoes the path and the proxy
// headers it saw, and accepts a WebSocket-style upgrade on the HMR path,
// echoing bytes back over the hijacked connection.
func fakeNextDev(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_next/webpack-hmr" && strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			conn, rw, err := http.NewResponseController(w).Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			defer conn.Close()
			rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
			rw.Flush()
			buf := make([]byte, 4)
			if _, err := io.ReadFull(rw, buf); err == nil {
				rw.Write(buf)
				rw.Flush()
			}
			return
		}
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<html>next:"+r.URL.RequestURI()+" origin="+r.Header.Get("Origin")+" xfh="+r.Header.Get("X-Forwarded-Host")+"</html>")
	}))
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, u string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Origin", "https://share-abc.example.dev")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestDevProxy(t *testing.T) {
	host := newFakeHost(t)
	next := fakeNextDev(t)
	srv := bootWith(t, host, func(c *cfg.Config) { c.WebDevProxy = next.URL })

	// UI paths go to the dev server, query kept, Origin dropped.
	code, body := get(t, srv.URL+"/job/?id=1")
	if code != 200 || !strings.Contains(body, "next:/job/?id=1 origin= ") {
		t.Fatalf("UI via proxy: %d %q", code, body)
	}
	if !strings.Contains(body, "xfh="+strings.TrimPrefix(srv.URL, "http://")) {
		t.Fatalf("X-Forwarded-Host not set: %q", body)
	}
	// Module routes are not proxied.
	if code, m := do(t, "GET", srv.URL+"/.well-known/helixos-module.json", "", nil); code != 200 || m["key"] != moduleKey {
		t.Fatalf("manifest through dev server: %d %v", code, m)
	}
	if code, _ := do(t, "GET", srv.URL+"/api/app/notes", "", nil); code != 401 {
		t.Fatalf("API without token: %d, want 401", code)
	}
	for _, p := range []string{"/api/unknown", "/_/", "/api/collections/notes/records"} {
		if code, body := get(t, srv.URL+p); code != 404 || strings.Contains(body, "next:") {
			t.Fatalf("%s: %d %q, want a module 404", p, code, body)
		}
	}

	// The HMR WebSocket upgrade passes through.
	u, _ := url.Parse(srv.URL)
	conn, err := net.DialTimeout("tcp", u.Host, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	io.WriteString(conn, "GET /_next/webpack-hmr HTTP/1.1\r\nHost: "+u.Host+"\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n")
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil || resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade: %v %v", resp, err)
	}
	io.WriteString(conn, "ping")
	echo := make([]byte, 4)
	if _, err := io.ReadFull(br, echo); err != nil || string(echo) != "ping" {
		t.Fatalf("websocket echo %q %v", echo, err)
	}
}

func TestDevProxyDownIs502(t *testing.T) {
	host := newFakeHost(t)
	srv := bootWith(t, host, func(c *cfg.Config) { c.WebDevProxy = "http://127.0.0.1:1" })
	if code, _ := get(t, srv.URL+"/"); code != http.StatusBadGateway {
		t.Fatalf("dev server down: %d, want 502", code)
	}
}

// The dev host's frame token works against the module end to end: the module
// verifies it against the dev host's JWKS and calls its whoami.
func TestDevHostFrameTokenEndToEnd(t *testing.T) {
	host := newFakeHost(t)
	srv := boot(t, host)
	req, _ := http.NewRequest("POST", host.srv.URL+"/devhost/token", nil)
	req.Header.Set("Origin", host.srv.URL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var tok struct {
		Token string `json:"token"`
		Org   string `json:"org"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil || tok.Token == "" {
		t.Fatalf("frame token: %v", err)
	}
	resp.Body.Close()
	code, me := do(t, "GET", srv.URL+"/api/app/me", tok.Token, nil)
	if code != 200 || me["org"] != "org_a" || me["org_name"] != "Org A" || me["host"] != host.srv.URL {
		t.Fatalf("/api/app/me with a dev host token: %d %v", code, me)
	}
}
