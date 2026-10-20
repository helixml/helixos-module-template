package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// FileSigner issues short-lived URLs for PocketBase file fields, so the host
// can fetch a file into an agent's workspace (contract §6.4 files[].url)
// without any other credential.
type FileSigner struct {
	key []byte
}

// NewFileSigner uses key, or a random per-process key when empty (URLs then
// stop working on restart, which only matters for files mid-delivery).
func NewFileSigner(key string) *FileSigner {
	k := []byte(key)
	if len(k) == 0 {
		k = make([]byte, 32)
		_, _ = rand.Read(k)
	}
	return &FileSigner{key: k}
}

type fileGrant struct {
	Collection string `json:"c"`
	Record     string `json:"r"`
	Field      string `json:"f"`
	Expires    int64  `json:"e"`
}

// URL returns base/files/<token> granting the file in rec.field for ttl.
func (s *FileSigner) URL(base string, rec *core.Record, field string, ttl time.Duration) string {
	g, _ := json.Marshal(fileGrant{Collection: rec.Collection().Name, Record: rec.Id, Field: field, Expires: time.Now().Add(ttl).Unix()})
	p := base64.RawURLEncoding.EncodeToString(g)
	return strings.TrimRight(base, "/") + "/files/" + p + "." + s.mac(p)
}

func (s *FileSigner) mac(p string) string {
	h := hmac.New(sha256.New, s.key)
	h.Write([]byte(p))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// Serve answers GET /files/{token}.
func (s *FileSigner) Serve(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		p, sig, ok := strings.Cut(e.Request.PathValue("token"), ".")
		if !ok || !hmac.Equal([]byte(sig), []byte(s.mac(p))) {
			return apiErr(e, http.StatusForbidden, "forbidden", "invalid file link")
		}
		raw, err := base64.RawURLEncoding.DecodeString(p)
		var g fileGrant
		if err != nil || json.Unmarshal(raw, &g) != nil || time.Now().Unix() > g.Expires {
			return apiErr(e, http.StatusForbidden, "forbidden", "expired file link")
		}
		rec, err := app.FindRecordById(g.Collection, g.Record)
		if err != nil {
			return apiErr(e, http.StatusNotFound, "not_found", "file not found")
		}
		name := rec.GetString(g.Field)
		if name == "" {
			return apiErr(e, http.StatusNotFound, "not_found", "file not found")
		}
		fsys, err := app.NewFilesystem()
		if err != nil {
			return err
		}
		defer fsys.Close()
		return fsys.Serve(e.Response, e.Request, rec.BaseFilesPath()+"/"+name, name)
	}
}
