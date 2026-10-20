package helixos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HostAPI calls a host's /api/module-host/v1 (contract §6).
type HostAPI struct {
	// Credential returns this module's credential for an issuer.
	Credential func(iss string) string
	// HostBase maps an issuer to the base URL to reach it at.
	HostBase func(iss string) string
	HTTP     *http.Client
}

// HostError is a non-2xx host API answer.
type HostError struct {
	Status  int
	Code    string `json:"code"`
	Message string `json:"error"`
}

func (e *HostError) Error() string {
	return fmt.Sprintf("host API %d %s: %s", e.Status, e.Code, e.Message)
}

// NotImplemented reports a host that lacks the feature (contract §6.2): the
// module should degrade, not fail.
func NotImplemented(err error) bool {
	var he *HostError
	return errors.As(err, &he) && (he.Status == http.StatusNotImplemented || he.Code == "not_implemented")
}

// Do calls method path on the caller's host, as the caller's installation.
// asUser forwards the caller's user token for user-attributed calls.
func (h *HostAPI) Do(ctx context.Context, c *Caller, asUser bool, method, path string, body, out interface{}) error {
	cred := h.Credential(c.Issuer)
	if cred == "" {
		return fmt.Errorf("no module credential configured for host %s", c.Issuer)
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(h.HostBase(c.Issuer), "/")+"/api/module-host/v1"+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cred)
	req.Header.Set("X-HelixOS-Installation", c.Inst)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if asUser {
		if c.Token == "" {
			return errors.New("this host call needs a user, but the request has no user token")
		}
		req.Header.Set("X-HelixOS-User-Token", c.Token)
	}
	client := h.HTTP
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode/100 != 2 {
		he := &HostError{Status: resp.StatusCode}
		if json.Unmarshal(data, he) != nil || he.Message == "" {
			he.Message = strings.TrimSpace(string(data))
		}
		return he
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// --- typed helpers for the common endpoints ---

// WhoAmI is GET /whoami.
type WhoAmI struct {
	Module       string   `json:"module"`
	Installation string   `json:"installation"`
	Org          string   `json:"org"`
	OrgName      string   `json:"org_name"`
	Brand        string   `json:"brand"`
	Scopes       []string `json:"scopes"`
	Host         string   `json:"host"`
}

func (h *HostAPI) WhoAmI(ctx context.Context, c *Caller) (*WhoAmI, error) {
	var out WhoAmI
	return &out, h.Do(ctx, c, false, http.MethodGet, "/whoami", nil, &out)
}

// Person is GET /people/{id}.
type Person struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	Handle      string `json:"handle"`
	SlackUserID string `json:"slack_user_id"`
}

func (h *HostAPI) Person(ctx context.Context, c *Caller, userID string) (*Person, error) {
	var out Person
	return &out, h.Do(ctx, c, false, http.MethodGet, "/people/"+url.PathEscape(userID), nil, &out)
}

// RunRequest is POST /runs.
type RunRequest struct {
	BotKey       string                 `json:"bot_key"`
	Title        string                 `json:"title,omitempty"`
	SystemPrompt string                 `json:"system_prompt,omitempty"`
	Prompt       string                 `json:"prompt"`
	StartURL     string                 `json:"start_url,omitempty"`
	ToolGroups   []string               `json:"tool_groups,omitempty"`
	Files        []RunFile              `json:"files,omitempty"`
	Meta         map[string]interface{} `json:"meta,omitempty"`
}

// RunFile is a signed URL the host fetches into the run's workspace.
type RunFile struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Run is the contract's Run object.
type Run struct {
	RunID       string                 `json:"run_id"`
	TaskID      string                 `json:"task_id,omitempty"`
	BotKey      string                 `json:"bot_key"`
	Title       string                 `json:"title"`
	OwnerUserID string                 `json:"owner_user_id"`
	Status      string                 `json:"status"`
	CreatedAt   time.Time              `json:"created_at"`
	CompletedAt *time.Time             `json:"completed_at"`
	Meta        map[string]interface{} `json:"meta"`
	RunFrameURL string                 `json:"run_frame_url"`
	ViewURL     string                 `json:"view_url,omitempty"`
}

func (h *HostAPI) StartRun(ctx context.Context, c *Caller, r RunRequest) (*Run, error) {
	var out Run
	return &out, h.Do(ctx, c, true, http.MethodPost, "/runs", r, &out)
}

func (h *HostAPI) GetRun(ctx context.Context, c *Caller, id string) (*Run, error) {
	var out Run
	return &out, h.Do(ctx, c, false, http.MethodGet, "/runs/"+url.PathEscape(id), nil, &out)
}

func (h *HostAPI) ListRuns(ctx context.Context, c *Caller, botKeyPrefix string, limit int) ([]Run, error) {
	var out struct {
		Runs []Run `json:"runs"`
	}
	q := url.Values{}
	if botKeyPrefix != "" {
		q.Set("bot_key_prefix", botKeyPrefix)
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprint(limit))
	}
	return out.Runs, h.Do(ctx, c, false, http.MethodGet, "/runs?"+q.Encode(), nil, &out)
}

func (h *HostAPI) StopRun(ctx context.Context, c *Caller, id string) error {
	return h.Do(ctx, c, true, http.MethodDelete, "/runs/"+url.PathEscape(id), nil, nil)
}

func (h *HostAPI) MessageRun(ctx context.Context, c *Caller, id, text string) error {
	return h.Do(ctx, c, true, http.MethodPost, "/runs/"+url.PathEscape(id)+"/messages", map[string]interface{}{"text": text}, nil)
}

// Bot is an entry of GET /bots.
type Bot struct {
	BotKey      string     `json:"bot_key"`
	Title       string     `json:"title"`
	OwnerUserID string     `json:"owner_user_id"`
	Owner       string     `json:"owner"`
	LastRunAt   *time.Time `json:"last_run_at"`
	LiveRunID   string     `json:"live_run_id"`
}

func (h *HostAPI) Bots(ctx context.Context, c *Caller, keyPrefix string) ([]Bot, error) {
	var out struct {
		Bots []Bot `json:"bots"`
	}
	return out.Bots, h.Do(ctx, c, false, http.MethodGet, "/bots?key_prefix="+url.QueryEscape(keyPrefix), nil, &out)
}

// NotifySlack is POST /notify/slack. Hosts without Slack answer
// not_implemented; check with NotImplemented.
func (h *HostAPI) NotifySlack(ctx context.Context, c *Caller, userID, channel, text, path string, query map[string]string) error {
	body := map[string]interface{}{"text": text}
	if userID != "" {
		body["user_id"] = userID
	}
	if channel != "" {
		body["channel"] = channel
	}
	if path != "" {
		body["link"] = map[string]interface{}{"path": path, "query": query}
	}
	return h.Do(ctx, c, false, http.MethodPost, "/notify/slack", body, nil)
}
