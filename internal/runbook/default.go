package runbook

import "github.com/helixml/helixos-module-template/runbooks"

// lib is this module's runbooks (runbooks/*.md, embedded). TestEmbedded
// checks they load, so the panic in MustLoad cannot happen in a built binary.
var lib = MustLoad(runbooks.FS)

// All lists this module's runbooks in order.
func All() []Runbook { return lib.List() }

// Lookup returns this module's runbook with key.
func Lookup(key string) (Runbook, bool) { return lib.Get(key) }
