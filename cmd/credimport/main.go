// Command credimport migrates the legacy reverselab-console SQLite credential
// ledger into ARTEX's encrypted cred_entries table.
//
//	go run ./cmd/credimport -sqlite console.db -dry-run
//	ARTEX_PG_DSN=... go run ./cmd/credimport -sqlite console.db -data /root/ARTEX/data
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Autumn-27/artex/credhunt"
	pgdb "github.com/Autumn-27/artex/db"
	_ "modernc.org/sqlite"
)

type legacyRow struct {
	Number       int64
	UID          string
	EntryID      string
	TargetNumber int64
	TargetAddr   string
	Type         string
	Service      string
	Risk         string
	Verified     string
	VerifiedDesc string
	VerifiedAt   string
	Source       string
	Credential   string
	Title        string
	Context      string
	SyncedAt     string
}

func main() {
	sqlitePath := flag.String("sqlite", "", "legacy console.db path")
	dryRun := flag.Bool("dry-run", false, "only summarize, do not write PostgreSQL")
	dataDir := flag.String("data", "/root/ARTEX/data", "ARTEX data dir (holds credhunt.key)")
	limit := flag.Int("limit", 0, "max rows to migrate (0 = all)")
	flag.Parse()
	if strings.TrimSpace(*sqlitePath) == "" {
		fmt.Fprintln(os.Stderr, "-sqlite is required")
		os.Exit(2)
	}

	rows, err := readLegacy(*sqlitePath, *limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read legacy:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "legacy rows: %d\n", len(rows))

	if *dryRun {
		printDryRun(rows)
		return
	}

	dsn, source, err := pgdb.DSN()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dsn:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "pg dsn source: %s\n", source)
	pg, err := pgdb.Open(dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pg open:", err)
		os.Exit(1)
	}
	defer pg.Close()
	svc, err := credhunt.NewService(pg, *dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "credhunt:", err)
		os.Exit(1)
	}
	stats := svc.Store.Import(context.Background(), mapRows(rows))
	out, _ := json.MarshalIndent(stats, "", "  ")
	fmt.Println(string(out))
}

func readLegacy(path string, limit int) ([]legacyRow, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	q := `SELECT number,uid,entry_id,target_number,target_addr,type,service,risk,verified,verified_desc,
	             verified_at,source,credential,title,context,synced_at
	      FROM credentials WHERE archived=0 ORDER BY number`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rs, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []legacyRow
	for rs.Next() {
		var r legacyRow
		var entryID, targetAddr, service, risk, verified, desc, verifiedAt, source, credential, title, context, synced sql.NullString
		var targetNumber sql.NullInt64
		if err := rs.Scan(&r.Number, &r.UID, &entryID, &targetNumber, &targetAddr, &r.Type, &service, &risk,
			&verified, &desc, &verifiedAt, &source, &credential, &title, &context, &synced); err != nil {
			return nil, err
		}
		r.EntryID, r.TargetAddr, r.Service, r.Risk = entryID.String, targetAddr.String, service.String, risk.String
		r.Verified, r.VerifiedDesc, r.VerifiedAt = verified.String, desc.String, verifiedAt.String
		r.Source, r.Credential, r.Title, r.Context, r.SyncedAt = source.String, credential.String, title.String, context.String, synced.String
		r.TargetNumber = targetNumber.Int64
		out = append(out, r)
	}
	return out, rs.Err()
}

func mapRows(rows []legacyRow) []credhunt.ImportRow {
	out := make([]credhunt.ImportRow, 0, len(rows))
	for _, r := range rows {
		provider, _ := credhunt.Classify("", r.Credential, r.Service, r.Context, r.TargetAddr)
		kind := mapKind(r.Type, provider)
		if kind == credhunt.KindProxyNode || kind == credhunt.KindProxySubscription {
			provider = "proxy"
		}
		status, detail := mapStatus(r.Verified, r.VerifiedDesc)
		sourceURL := strings.TrimSpace(r.TargetAddr)
		if !strings.HasPrefix(sourceURL, "http://") && !strings.HasPrefix(sourceURL, "https://") {
			sourceURL = fmt.Sprintf("legacy://console/%d", r.TargetNumber)
		}
		evidence := strings.TrimSpace(r.Source)
		if evidence == "" {
			evidence = "console.db#" + r.EntryID
		}
		context := strings.TrimSpace(r.Context)
		if r.Title != "" {
			context = strings.TrimSpace(r.Title + "\n" + context)
		}
		extra := map[string]any{
			"legacy_number": r.Number, "legacy_uid": r.UID, "legacy_entry_id": r.EntryID,
			"legacy_target": r.TargetNumber, "legacy_addr": r.TargetAddr,
			"legacy_type": r.Type, "legacy_verified": r.Verified, "legacy_desc": r.VerifiedDesc,
		}
		quota := map[string]any{}
		if r.Verified != "" || r.VerifiedDesc != "" {
			quota["legacy_verified"] = r.Verified
			quota["legacy_desc"] = r.VerifiedDesc
		}
		value := r.Credential
		var proxy *credhunt.ProxyMeta
		if kind == credhunt.KindProxyNode {
			if parsed := credhunt.ParseProxyContent(r.Credential); len(parsed) > 0 {
				one := parsed[0]
				if b, err := json.Marshal(one.Config); err == nil {
					value = string(b)
					proxy = &credhunt.ProxyMeta{Protocol: one.Protocol, Server: one.Server,
						Port: one.Port, Status: status, Message: detail}
				}
			}
		}
		out = append(out, credhunt.ImportRow{
			Kind: kind, Provider: provider, Service: r.Service, Value: value,
			SourceURL: sourceURL, EvidencePath: evidence, Status: status, StatusDetail: detail,
			Risk: r.Risk, Context: context, Quota: quota, Extra: extra,
			FoundAt: parseTime(r.SyncedAt), CheckedAt: parseTimePtr(r.VerifiedAt),
			Proxy: proxy,
		})
	}
	return out
}

func mapKind(oldType, provider string) string {
	switch strings.ToLower(strings.TrimSpace(oldType)) {
	case "api-key":
		if provider != "" && provider != "unknown" {
			return credhunt.KindLLMKey
		}
		return credhunt.KindAPIKey
	case "token":
		return credhunt.KindToken
	case "credential":
		return credhunt.KindPassword
	case "private-key", "ssh-key", "ssl-key":
		return credhunt.KindPrivateKey
	case "proxy-node":
		return credhunt.KindProxyNode
	case "proxy-subscription":
		return credhunt.KindProxySubscription
	default:
		return credhunt.KindOther
	}
}

func mapStatus(verified, desc string) (string, string) {
	switch strings.TrimSpace(verified) {
	case "✅":
		return credhunt.StatusValid, desc
	case "❌":
		return credhunt.StatusInvalid, desc
	case "⚠️":
		return credhunt.StatusUnknown, desc
	case "❓", "":
		return credhunt.StatusUnverified, desc
	default:
		return credhunt.StatusUnknown, desc
	}
}

func parseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t
		}
	}
	return time.Time{}
}

func parseTimePtr(s string) *time.Time {
	t := parseTime(s)
	if t.IsZero() {
		return nil
	}
	return &t
}

func printDryRun(rows []legacyRow) {
	byKind := map[string]int{}
	byStatus := map[string]int{}
	seen := map[string]bool{}
	dup := 0
	for _, r := range rows {
		provider, _ := credhunt.Classify("", r.Credential, r.Service, r.Context, r.TargetAddr)
		kind := mapKind(r.Type, provider)
		status, _ := mapStatus(r.Verified, r.VerifiedDesc)
		byKind[kind]++
		byStatus[status]++
		if r.Credential != "" {
			if seen[r.Credential] {
				dup++
			}
			seen[r.Credential] = true
		}
	}
	type kv struct {
		K string
		N int
	}
	var kinds, statuses []kv
	for k, n := range byKind {
		kinds = append(kinds, kv{k, n})
	}
	for k, n := range byStatus {
		statuses = append(statuses, kv{k, n})
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i].N > kinds[j].N })
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].N > statuses[j].N })
	out := map[string]any{"rows": len(rows), "duplicates": dup, "by_kind": kinds, "by_status": statuses}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
}
