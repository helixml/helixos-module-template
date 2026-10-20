package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

// A host deep link "/job?id=1" must land on "/job/?id=1", query intact.
func TestStaticUIKeepsQueryOnDirRedirect(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "job"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "job", "index.html"), []byte("job"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := staticUI(dir)
	req := httptest.NewRequest("GET", "/job?id=1&run=2", nil)
	req.SetPathValue("path", "job")
	rec := httptest.NewRecorder()
	e := &core.RequestEvent{Event: router.Event{Response: rec, Request: req}}
	if err := h(e); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/job/?id=1&run=2" {
		t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Location"))
	}
}
