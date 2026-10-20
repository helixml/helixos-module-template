// Package mcp is the module's MCP endpoint (contract v1 §5): JSON-RPC 2.0 over a
// single POST, protocol 2024-11-05. The host lists these tools once per version
// and forwards calls with a host token naming the org, bot and run. Tools call
// the same ops functions as the REST API.
package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/helixml/helixos-module-template/internal/helixos"
	"github.com/helixml/helixos-module-template/internal/module"
	"github.com/helixml/helixos-module-template/internal/ops"
	"github.com/pocketbase/pocketbase/core"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	run         func(app core.App, c *helixos.Caller, args map[string]interface{}) (interface{}, error)
}

// tools is the catalogue. Names carry the module's prefix so they never
// collide with host tools or other modules.
func tools(key string) []tool {
	p := module.ToolPrefix(key)
	return []tool{
		{
			Name:        p + "_note_add",
			Description: "Save a note in the " + module.Name + " module for this organization.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{"type": "string", "description": "Short title"},
					"body":  map[string]interface{}{"type": "string", "description": "Note text (markdown)"},
				},
				"required": []string{"title"},
			},
			run: func(app core.App, c *helixos.Caller, a map[string]interface{}) (interface{}, error) {
				title, _ := a["title"].(string)
				body, _ := a["body"].(string)
				return ops.CreateNote(app, c.Tenant(), ops.CreateNoteInput{Title: title, Body: body, Source: "agent", By: c.UserID, ByName: "Agent (" + c.Bot + ")"})
			},
		},
		{
			Name:        p + "_notes_list",
			Description: "List this organization's notes in the " + module.Name + " module, newest first.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
			run: func(app core.App, c *helixos.Caller, _ map[string]interface{}) (interface{}, error) {
				return ops.ListNotes(app, c.Tenant(), 50)
			},
		},
	}
}

// Handler serves POST /mcp. The host token was verified by middleware.
func Handler(app core.App, key string) func(e *core.RequestEvent) error {
	catalogue := tools(key)
	return func(e *core.RequestEvent) error {
		var req request
		if err := json.NewDecoder(io.LimitReader(e.Request.Body, 1<<20)).Decode(&req); err != nil {
			return rpcErr(e, nil, -32700, "parse error")
		}
		c, _ := e.Get(helixos.CallerKey).(*helixos.Caller)
		switch req.Method {
		case "initialize":
			return result(e, req.ID, map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
				"serverInfo":      map[string]string{"name": key, "version": module.Version},
			})
		case "notifications/initialized":
			return e.NoContent(http.StatusAccepted)
		case "ping":
			return result(e, req.ID, map[string]interface{}{})
		case "tools/list":
			list := make([]tool, len(catalogue))
			copy(list, catalogue)
			return result(e, req.ID, map[string]interface{}{"tools": list})
		case "tools/call":
			var p struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &p); err != nil {
				return rpcErr(e, req.ID, -32602, "invalid params")
			}
			for _, t := range catalogue {
				if t.Name != p.Name {
					continue
				}
				out, err := t.run(app, c, p.Arguments)
				if err != nil {
					return result(e, req.ID, textResult("Error: "+ops.AsErr(err).Msg, true))
				}
				b, _ := json.Marshal(out)
				return result(e, req.ID, textResult(string(b), false))
			}
			return rpcErr(e, req.ID, -32601, fmt.Sprintf("unknown tool %q", p.Name))
		}
		return rpcErr(e, req.ID, -32601, "method not found: "+req.Method)
	}
}

func textResult(text string, isErr bool) map[string]interface{} {
	return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": text}}, "isError": isErr}
}

func result(e *core.RequestEvent, id json.RawMessage, v interface{}) error {
	return e.JSON(http.StatusOK, map[string]interface{}{"jsonrpc": "2.0", "id": id, "result": v})
}

func rpcErr(e *core.RequestEvent, id json.RawMessage, code int, msg string) error {
	return e.JSON(http.StatusOK, map[string]interface{}{"jsonrpc": "2.0", "id": id, "error": map[string]interface{}{"code": code, "message": msg}})
}
