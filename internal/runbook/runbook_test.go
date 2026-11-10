package runbook

import (
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/helixml/helixos-module-template/runbooks"
)

func TestParse(t *testing.T) {
	r, err := Parse([]byte("---\nkey: a-1\ntitle: A: one\nsummary:  Does a thing.  \norder: 2\n---\n# Body\n\n---\nrest\n"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Key != "a-1" || r.Title != "A: one" || r.Summary != "Does a thing." || r.Order != 2 {
		t.Fatalf("frontmatter = %+v", r)
	}
	// The body is everything after the closing line, byte for byte (a later
	// --- is just markdown).
	if got := r.Text(nil); got != "# Body\n\n---\nrest\n" {
		t.Fatalf("body = %q", got)
	}
	// CRLF files read the same.
	if r2, err := Parse([]byte("---\r\nkey: a\r\ntitle: A\r\nsummary: S\r\n---\r\nx\r\n")); err != nil || r2.Text(nil) != "x\n" {
		t.Fatalf("crlf: %v %q", err, r2.Text(nil))
	}
}

func TestParseRejects(t *testing.T) {
	for name, src := range map[string]string{
		"no frontmatter":  "# Just markdown\n",
		"not closed":      "---\nkey: a\ntitle: A\nsummary: S\n",
		"missing title":   "---\nkey: a\nsummary: S\n---\nx",
		"missing summary": "---\nkey: a\ntitle: A\n---\nx",
		"bad key":         "---\nkey: Not OK\ntitle: A\nsummary: S\n---\nx",
		"unknown field":   "---\nkey: a\ntitle: A\nsummary: S\ntitel: B\n---\nx",
		"repeated field":  "---\nkey: a\nkey: b\ntitle: A\nsummary: S\n---\nx",
		"bad line":        "---\nkey: a\ntitle: A\nsummary: S\njust words\n---\nx",
		"bad order":       "---\nkey: a\ntitle: A\nsummary: S\norder: first\n---\nx",
	} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestLoadIncludesOrderAndVars(t *testing.T) {
	fsys := fstest.MapFS{
		"_shared.md": {Data: []byte("# Shared\nrules\n\n")},
		"b.md":       {Data: []byte("---\nkey: b\ntitle: B\nsummary: S\norder: 1\n---\n<!-- include: _shared.md -->\nUse {{prefix}}_tool.\n")},
		"a.md":       {Data: []byte("---\nkey: a\ntitle: A\nsummary: S\norder: 2\n---\nplain\n")},
		"c.md":       {Data: []byte("---\nkey: c\ntitle: C\nsummary: S\norder: 2\n---\nplain\n")},
	}
	l, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, r := range l.List() {
		keys = append(keys, r.Key)
	}
	if strings.Join(keys, ",") != "b,a,c" {
		t.Fatalf("order = %v (fragments must not be listed)", keys)
	}
	b, _ := l.Get("b")
	if got := b.Text(map[string]string{"prefix": "my_mod"}); got != "# Shared\nrules\n\nUse my_mod_tool.\n" {
		t.Fatalf("text = %q", got)
	}
	if got := b.Text(nil); !strings.Contains(got, "{{prefix}}") {
		t.Fatalf("no vars must leave placeholders alone: %q", got)
	}
	if _, ok := l.Get("_shared"); ok {
		t.Fatal("a fragment is not a runbook")
	}
}

func TestLoadRejects(t *testing.T) {
	ok := "---\nkey: a\ntitle: A\nsummary: S\n---\n"
	for name, fsys := range map[string]fstest.MapFS{
		"key differs from file": {"b.md": {Data: []byte(ok)}},
		"missing fragment":      {"a.md": {Data: []byte(ok + "<!-- include: _nope.md -->\n")}},
		"fragment not _name":    {"a.md": {Data: []byte(ok + "<!-- include: other.md -->\n")}, "other.md": {Data: []byte(ok)}},
		"nested fragment": {"a.md": {Data: []byte(ok + "<!-- include: _x.md -->\n")},
			"_x.md": {Data: []byte("<!-- include: _y.md -->\n")}, "_y.md": {Data: []byte("y")}},
	} {
		if _, err := Load(fsys); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

var placeholderRE = regexp.MustCompile(`\{\{[a-z_]+\}\}`)

// TestEmbedded checks this module's own runbooks load and read as complete
// documents: no include line is left, and every placeholder is a {{name}} the
// module fills (see runbookVars in internal/api).
func TestEmbedded(t *testing.T) {
	l, err := Load(runbooks.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.List()) == 0 {
		t.Fatal("no runbooks embedded")
	}
	for _, r := range All() {
		text := r.Text(nil)
		if strings.Contains(text, includePrefix) {
			t.Errorf("%s: unexpanded include", r.Key)
		}
		if rest := placeholderRE.ReplaceAllString(text, ""); strings.Contains(rest, "{{") || strings.Contains(rest, "}}") {
			t.Errorf("%s: a {{placeholder}} is not of the form {{lower_name}}", r.Key)
		}
		if strings.TrimSpace(text) == "" {
			t.Errorf("%s: empty", r.Key)
		}
	}
}
