package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/Rake-Pro/go-proxy-manager/internal/importer"
	"github.com/Rake-Pro/go-proxy-manager/internal/logging"
	"github.com/Rake-Pro/go-proxy-manager/internal/store"
)

// runImport is the `gpm import` subcommand: a one-time, best-effort migration of
// an existing NPM/NPMplus /data directory into our git-backed config.
func runImport(args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	npmData := fs.String("npm-data", "", "path to the NPM/NPMplus /data directory to import (required)")
	configDir := fs.String("config-dir", envOr("GPM_CONFIG_DIR", "/data/config"), "target git-backed config directory")
	certDir := fs.String("cert-dir", envOr("GPM_CERT_DIR", "/data/certs"), "target certificate directory")
	dryRun := fs.Bool("dry-run", false, "map and report only; write nothing")
	authorName := fs.String("author", "npm-import", "git author name for the import commit")
	_ = fs.Parse(args)

	logging.Setup("info", true)

	if *npmData == "" {
		fmt.Fprintln(os.Stderr, "error: --npm-data is required")
		os.Exit(2)
	}

	ctx := context.Background()
	result, err := importer.Import(ctx, *npmData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "import failed: %v\n", err)
		os.Exit(1)
	}
	printImportReport(result)

	if *dryRun {
		fmt.Println("\nDry run - nothing written. Re-run without --dry-run to apply.")
		return
	}

	// Copy certificate material into the cert store, matching the relative paths
	// the imported Certificate objects reference (<name>/fullchain.pem etc).
	for _, cc := range result.Certs {
		dst := filepath.Join(*certDir, cc.Name)
		if err := os.MkdirAll(dst, 0o700); err != nil {
			fmt.Fprintf(os.Stderr, "warning: cert %q: %v\n", cc.Name, err)
			continue
		}
		if err := copyFile(cc.CertPEM, filepath.Join(dst, "fullchain.pem"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "warning: cert %q chain: %v\n", cc.Name, err)
		}
		if err := copyFile(cc.KeyPEM, filepath.Join(dst, "privkey.pem"), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "warning: cert %q key: %v\n", cc.Name, err)
		}
	}

	st := store.New(*configDir, store.NewExecGit(*configDir))
	if err := st.Init(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to open config store: %v\n", err)
		os.Exit(1)
	}
	sha, err := st.SaveBatch(ctx, result.Objects, "Import from NPM/NPMplus", store.Author{Name: *authorName})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write imported config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nImported %d objects into %s (commit %s).\n", len(result.Objects), *configDir, short(sha))
	if len(result.Warnings) > 0 {
		fmt.Printf("Review %d warning(s) above before relying on the result.\n", len(result.Warnings))
	}
}

func printImportReport(r *importer.Result) {
	fmt.Println("Import summary:")
	kinds := make([]string, 0, len(r.Summary))
	for k := range r.Summary {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Printf("  %-18s %d\n", k, r.Summary[k])
	}
	if len(r.Warnings) == 0 {
		fmt.Println("\nNo warnings - clean mapping.")
		return
	}
	fmt.Printf("\n%d warning(s) - the following were not fully imported:\n", len(r.Warnings))
	for _, w := range r.Warnings {
		fmt.Printf("  [%s] %s: %s\n", w.Object, w.Field, w.Reason)
	}
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
