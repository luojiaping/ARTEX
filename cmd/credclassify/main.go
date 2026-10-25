// Command credclassify recomputes the fine-grained provider/variant/pclass
// classification for every ledger entry. It does not change status or re-probe.
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
	dataDir := flag.String("data", "/root/ARTEX/data", "ARTEX data dir (holds credhunt.key)")
	flag.Parse()
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
	stats, err := svc.Store.Reclassify(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "reclassify:", err)
		os.Exit(1)
	}
	facets, _ := svc.Store.Facets(context.Background())
	out, _ := json.MarshalIndent(map[string]any{"updated_by_pclass": stats, "facets": facets}, "", "  ")
	fmt.Println(string(out))
}
