package devhost

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// Main runs `module devhost [flags]`: the dev host on --addr until killed.
func Main(args []string, defaultKey string) error {
	fs := flag.NewFlagSet("devhost", flag.ContinueOnError)
	envOr := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return def
	}
	var o Options
	addr := fs.String("addr", envOr("DEVHOST_ADDR", "127.0.0.1:8090"), "listen address (loopback by default: this host signs tokens for anyone who reaches it)")
	fs.StringVar(&o.Issuer, "issuer", envOr("DEVHOST_ISSUER", "http://localhost:8090"), "iss claim; the module must list it in HELIXOS_ISSUERS and reach its JWKS there")
	fs.StringVar(&o.ModuleURL, "module", envOr("DEVHOST_MODULE_URL", "http://localhost:8080"), "module URL to frame")
	fs.StringVar(&o.ModuleKey, "key", envOr("MODULE_KEY", defaultKey), "module key (token audience module:<key>)")
	fs.StringVar(&o.Credential, "credential", os.Getenv("DEVHOST_CREDENTIAL"), "module credential the host API accepts (empty: any)")
	fs.StringVar(&o.Org, "org", "org_dev", "dev org id")
	fs.StringVar(&o.Inst, "inst", "inst_dev", "dev installation id")
	fs.StringVar(&o.Email, "email", "dev@example.com", "dev user email")
	fs.StringVar(&o.Name, "name", "Dev User", "dev user name")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	h, err := New(o)
	if err != nil {
		return err
	}
	o = h.Options()
	log.Printf("devhost: issuer %s, framing %s (module %s), org %s, listening on %s", o.Issuer, o.ModuleURL, o.ModuleKey, o.Org, *addr)
	srv := &http.Server{Addr: *addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		return fmt.Errorf("devhost: %w", err)
	}
	return nil
}
