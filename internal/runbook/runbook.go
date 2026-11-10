// Package runbook reads the agent runbooks in runbooks/*.md.
//
// A runbook is markdown with a short frontmatter:
//
//	---
//	key: notes-agent
//	title: Notes agent
//	summary: One line on what the agent does.
//	order: 1
//	---
//	The instructions, in markdown…
//
// key (= the file name without .md), title and summary are required; order is
// optional and sorts the list (then key). Each frontmatter line is `name: value`;
// the value is taken verbatim, no quoting.
//
// The body may pull in a shared fragment (a file whose name starts with "_",
// which is never listed itself) with a line of its own:
//
//	<!-- include: _linkedin-safety.md -->
//
// The line, newline included, is replaced by the fragment's bytes, so a runbook
// renders as one complete document. Placeholders {{name}} are filled from the
// vars passed to Text (the module key's tool prefix, say); a runbook that needs
// none is returned as is.
package runbook

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Runbook is one parsed runbook.
type Runbook struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Order   int    `json:"-"`
	body    string // includes expanded, placeholders not yet filled
}

// Text is the runbook's markdown with placeholders filled from vars: what an
// agent is given and what a person reads.
func (r Runbook) Text(vars map[string]string) string {
	if len(vars) == 0 {
		return r.body
	}
	pairs := make([]string, 0, 2*len(vars))
	for k, v := range vars {
		pairs = append(pairs, "{{"+k+"}}", v)
	}
	return strings.NewReplacer(pairs...).Replace(r.body)
}

// Library is a set of runbooks.
type Library struct {
	list  []Runbook
	byKey map[string]Runbook
}

// List returns the runbooks in order.
func (l *Library) List() []Runbook { return append([]Runbook(nil), l.list...) }

// Get returns the runbook with key.
func (l *Library) Get(key string) (Runbook, bool) {
	r, ok := l.byKey[key]
	return r, ok
}

var keyRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

const includePrefix, includeSuffix = "<!-- include: ", " -->"

// Load reads every *.md at the root of fsys.
func Load(fsys fs.FS) (*Library, error) {
	names, err := fs.Glob(fsys, "*.md")
	if err != nil {
		return nil, err
	}
	l := &Library{byKey: map[string]Runbook{}}
	for _, name := range names {
		if strings.HasPrefix(name, "_") {
			continue
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		r, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("runbook %s: %w", name, err)
		}
		if want := strings.TrimSuffix(name, ".md"); r.Key != want {
			return nil, fmt.Errorf("runbook %s: key %q must match the file name (%q)", name, r.Key, want)
		}
		if r.body, err = expand(fsys, r.body); err != nil {
			return nil, fmt.Errorf("runbook %s: %w", name, err)
		}
		l.list = append(l.list, r)
		l.byKey[r.Key] = r
	}
	sort.SliceStable(l.list, func(i, j int) bool {
		if l.list[i].Order != l.list[j].Order {
			return l.list[i].Order < l.list[j].Order
		}
		return l.list[i].Key < l.list[j].Key
	})
	return l, nil
}

// MustLoad is Load for embedded files, which a test has already checked.
func MustLoad(fsys fs.FS) *Library {
	l, err := Load(fsys)
	if err != nil {
		panic(err)
	}
	return l
}

// Parse splits a runbook file into its frontmatter and body. The body is
// everything after the closing "---" line, byte for byte.
func Parse(data []byte) (Runbook, error) {
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return Runbook{}, fmt.Errorf("must start with a --- frontmatter line")
	}
	head, body, ok := strings.Cut(s[len("---\n"):], "\n---\n")
	if !ok {
		return Runbook{}, fmt.Errorf("frontmatter is not closed by a --- line")
	}
	var r Runbook
	seen := map[string]bool{}
	for _, line := range strings.Split(head, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return Runbook{}, fmt.Errorf("frontmatter line %q is not name: value", line)
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if seen[k] {
			return Runbook{}, fmt.Errorf("frontmatter repeats %q", k)
		}
		seen[k] = true
		switch k {
		case "key":
			r.Key = v
		case "title":
			r.Title = v
		case "summary":
			r.Summary = v
		case "order":
			n, err := strconv.Atoi(v)
			if err != nil {
				return Runbook{}, fmt.Errorf("order %q is not a number", v)
			}
			r.Order = n
		default:
			return Runbook{}, fmt.Errorf("unknown frontmatter field %q", k)
		}
	}
	switch {
	case !keyRE.MatchString(r.Key):
		return Runbook{}, fmt.Errorf("key %q must be lowercase letters, digits and dashes", r.Key)
	case r.Title == "":
		return Runbook{}, fmt.Errorf("title is required")
	case r.Summary == "":
		return Runbook{}, fmt.Errorf("summary is required")
	}
	r.body = body
	return r, nil
}

// expand replaces each include line with its fragment. Fragments do not nest.
func expand(fsys fs.FS, body string) (string, error) {
	if !strings.Contains(body, includePrefix) {
		return body, nil
	}
	lines := strings.SplitAfter(body, "\n")
	var b strings.Builder
	for _, line := range lines {
		t := strings.TrimRight(line, "\n")
		if !strings.HasPrefix(t, includePrefix) || !strings.HasSuffix(t, includeSuffix) {
			b.WriteString(line)
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(t, includePrefix), includeSuffix)
		if !strings.HasPrefix(name, "_") || path.Base(name) != name || !strings.HasSuffix(name, ".md") {
			return "", fmt.Errorf("include %q: fragments are _name.md files next to the runbook", name)
		}
		frag, err := fs.ReadFile(fsys, name)
		if err != nil {
			return "", fmt.Errorf("include %q: %w", name, err)
		}
		if strings.Contains(string(frag), includePrefix) {
			return "", fmt.Errorf("include %q: fragments cannot include other fragments", name)
		}
		b.Write(frag)
	}
	return b.String(), nil
}
