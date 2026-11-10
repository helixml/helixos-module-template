// Package runbooks embeds the agent runbooks: the instructions this module's
// agents run with, one markdown file per runbook. internal/runbook parses
// them; agent runs are built from them and the Runbooks page shows them, so
// what a person reads is exactly what an agent gets.
//
// A file named _something.md is a fragment, not a runbook: runbooks pull it in
// with a line `<!-- include: _something.md -->`.
package runbooks

import "embed"

//go:embed *.md
var FS embed.FS
