// Command credprobe runs the new tiny-probe engine against one credential
// without writing a verdict or probe history. It is a dry-run aid for
// comparing the new engine with the old one before enabling it for a batch.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/Autumn-27/artex/credhunt"
	pgdb "github.com/Autumn-27/artex/db"
)

func main() {
	uid := flag.String("uid", "", "credential uid")
	dataDir := flag.String("data", "/root/ARTEX/data", "ARTEX data dir (holds credhunt.key)")
	flag.Parse()
	if *uid == "" {
		fmt.Fprintln(os.Stderr, "-uid is required")
		os.Exit(2)
	}
	dsn, source, err := pgdb.DSN()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dsn:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "pg dsn source: %s\n", source)
	pg, err := pgdb.Open(dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pg:", err)
		os.Exit(1)
	}
	defer pg.Close()
	svc, err := credhunt.NewService(pg, *dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "credhunt:", err)
		os.Exit(1)
	}
	ctx := context.Background()
	entry, _, err := svc.Store.Get(ctx, *uid)
	if err != nil {
		fmt.Fprintln(os.Stderr, "get:", err)
		os.Exit(1)
	}
	secret, err := svc.Store.Reveal(ctx, *uid)
	if err != nil {
		fmt.Fprintln(os.Stderr, "reveal:", err)
		os.Exit(1)
	}
	res := credhunt.NewVerifier(svc.Store).CheckOne(ctx, *entry, secret)
	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))
}
