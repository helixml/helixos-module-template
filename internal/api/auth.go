package api

import (
	"net/http"
	"strings"

	"github.com/helixml/helixos-module-template/internal/helixos"
	"github.com/pocketbase/pocketbase/core"
)



// requireUser verifies the user token the module UI sends (contract §4.3).
func (d *Deps) requireUser(e *core.RequestEvent) error {
	tok := bearer(e.Request)
	c, err := d.Verifier.VerifyUser(e.Request.Context(), tok)
	if tok == "" || err != nil {
		return apiErr(e, http.StatusUnauthorized, "unauthenticated", "open this page from your HelixOS workspace")
	}
	e.Set(helixos.CallerKey, c)
	return e.Next()
}

// requireHost verifies a host-call token (MCP, events). Tokens minted for the
// data migration (purpose=migrate) are only accepted by the import route.
func (d *Deps) requireHost(purpose string) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		c, err := d.Verifier.VerifyHost(e.Request.Context(), bearer(e.Request))
		if err != nil || c.Purpose != purpose {
			return apiErr(e, http.StatusUnauthorized, "unauthenticated", "invalid host token")
		}
		e.Set(helixos.CallerKey, c)
		return e.Next()
	}
}

func caller(e *core.RequestEvent) *helixos.Caller {
	c, _ := e.Get(helixos.CallerKey).(*helixos.Caller)
	return c
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
}

// apiErr writes the contract's error shape.
func apiErr(e *core.RequestEvent, status int, code, msg string) error {
	return e.JSON(status, map[string]string{"error": msg, "code": code})
}
