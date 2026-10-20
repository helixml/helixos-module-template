// Package module describes this module to its hosts: the manifest
// (contract v1 §3) and its identity. Edit DefaultKey, Name and the sections
// when you turn the template into your own module.
package module

import "strings"

// DefaultKey is the module key used when MODULE_KEY is not set.
const DefaultKey = "module-template"

// Name is the human-readable module name.
const Name = "Module Template"

// BaseVersion is the module's own version; bump it when you release.
const BaseVersion = "0.1.0"

// GitSHA is stamped at build time (-ldflags "-X …/internal/module.GitSHA=…").
// Every deploy changes it, so hosts re-read the manifest and tool list.
var GitSHA = "dev"

// Version is BaseVersion+GitSHA, the manifest version.
var Version = BaseVersion + "+" + GitSHA

// Manifest is the document served at /.well-known/helixos-module.json.
type Manifest struct {
	SchemaVersion        int        `json:"schema_version"`
	Key                  string     `json:"key"`
	Name                 string     `json:"name"`
	Version              string     `json:"version"`
	Description          string     `json:"description"`
	Icon                 string     `json:"icon"`
	Sections             []Section  `json:"sections"`
	MCP                  *Endpoint  `json:"mcp,omitempty"`
	Events               *Events    `json:"events,omitempty"`
	HostScopes           []string   `json:"host_scopes"`
	AgentToolGroups      []string   `json:"agent_tool_groups"`
	RequiredIntegrations []string   `json:"required_integrations"`
	Health               string     `json:"health"`
}

type Section struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Icon  string `json:"icon,omitempty"`
	Path  string `json:"path"`
}

type Endpoint struct {
	Path string `json:"path"`
}

type Events struct {
	Path      string   `json:"path"`
	Subscribe []string `json:"subscribe"`
}

// Build returns the manifest for module key.
func Build(key string) Manifest {
	return Manifest{
		SchemaVersion: 1,
		Key:           key,
		Name:          Name,
		Version:       Version,
		Description:   "Starter module: org-scoped notes, an agent tool, and agent runs shown inline.",
		Icon:          "Puzzle",
		Sections:      []Section{{Key: "main", Label: Name, Icon: "Puzzle", Path: "/"}},
		MCP:           &Endpoint{Path: "/mcp"},
		Events:        &Events{Path: "/helixos/events", Subscribe: []string{"run.completed", "installation.changed"}},
		HostScopes:    []string{"identity.read", "agents.run"},
		AgentToolGroups:      []string{"memory"},
		RequiredIntegrations: []string{},
		Health:               "/healthz",
	}
}

// ToolPrefix namespaces this module's MCP tool names (they must be unique
// across a host's catalogue): "module-template" → "module_template".
func ToolPrefix(key string) string { return strings.ReplaceAll(key, "-", "_") }
