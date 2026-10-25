// Command credprobes imports the legacy console's probe snapshots, probe_log
// history, manual statuses and variant profiles into ARTEX, matching rows by
// extra->>'legacy_uid'. It never re-probes anything.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Autumn-27/artex/credhunt"
	pgdb "github.com/Autumn-27/artex/db"
	_ "modernc.org/sqlite"
)

func main() {
	sqlitePath := flag.String("sqlite", "", "legacy console.db path")
	flag.Parse()
	if strings.TrimSpace(*sqlitePath) == "" {
		fmt.Fprintln(os.Stderr, "-sqlite is required")
		os.Exit(2)
	}
	db, err := sql.Open("sqlite", *sqlitePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sqlite:", err)
		os.Exit(1)
	}
	defer db.Close()
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
	ctx := context.Background()

	if _, err := pg.ExecContext(ctx, `DELETE FROM cred_probes WHERE source='legacy'`); err != nil {
		fmt.Fprintln(os.Stderr, "clear legacy probes:", err)
		os.Exit(1)
	}

	updated, histories, skipped := 0, 0, 0
	rows, err := db.Query(`
SELECT c.uid, COALESCE(c.manual_status,''), COALESCE(c.manual_status_at,''),
       COALESCE(p.pclass,''), COALESCE(p.provider,''), COALESCE(p.provider_name,''),
       COALESCE(p.variant,''), COALESCE(p.avail,''), COALESCE(p.avail_http,0), COALESCE(p.avail_ms,0),
       COALESCE(p.avail_desc,''), COALESCE(p.avail_at,''), COALESCE(p.quota_kind,''),
       COALESCE(p.quota_json,''), COALESCE(p.fail_count,0), COALESCE(p.next_probe_at,''),
       COALESCE(f.variant,''), COALESCE(f.base_url,''), COALESCE(f.model,'')
FROM credentials c
LEFT JOIN credential_probes p ON p.uid=c.uid
LEFT JOIN credential_profiles f ON f.uid=c.uid
WHERE c.archived=0`)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read probes:", err)
		os.Exit(1)
	}
	type snap struct {
		legacyUID, manual, manualAt, pclass, provider, providerName, variant, avail, availDesc, availAt,
		quotaKind, quotaJSON, nextProbe, profileVariant, baseURL, model string
		http, failCount int
		ms              float64
	}
	var snaps []snap
	for rows.Next() {
		var s snap
		if err := rows.Scan(&s.legacyUID, &s.manual, &s.manualAt, &s.pclass, &s.provider, &s.providerName,
			&s.variant, &s.avail, &s.http, &s.ms, &s.availDesc, &s.availAt, &s.quotaKind,
			&s.quotaJSON, &s.failCount, &s.nextProbe, &s.profileVariant, &s.baseURL, &s.model); err != nil {
			rows.Close()
			fmt.Fprintln(os.Stderr, "scan probe:", err)
			os.Exit(1)
		}
		snaps = append(snaps, s)
	}
	rows.Close()

	for _, s := range snaps {
		var uid string
		err := pg.QueryRowContext(ctx, `SELECT uid FROM cred_entries WHERE extra->>'legacy_uid'=$1 AND archived=false LIMIT 1`, s.legacyUID).Scan(&uid)
		if err == sql.ErrNoRows {
			skipped++
			continue
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "match uid:", err)
			continue
		}
		status, detail := legacyStatus(s.avail, s.availDesc)
		variant := s.variant
		if variant == "" {
			variant = s.profileVariant
		}
		quota := map[string]any{}
		if json.Valid([]byte(s.quotaJSON)) && strings.TrimSpace(s.quotaJSON) != "" {
			_ = json.Unmarshal([]byte(s.quotaJSON), &quota)
		}
		quotaJSON, _ := json.Marshal(quota)
		checked := parseTimePtr(s.availAt)
		next := parseTimePtr(s.nextProbe)
		_, err = pg.ExecContext(ctx, `
UPDATE cred_entries SET
  pclass=CASE WHEN $2<>'' THEN $2 ELSE pclass END,
  provider=CASE WHEN $3<>'' THEN $3 ELSE provider END,
  provider_name=CASE WHEN $4<>'' THEN $4 ELSE provider_name END,
  variant=CASE WHEN $5<>'' THEN $5 ELSE variant END,
  status=CASE WHEN $6<>'' THEN $6 ELSE status END,
  status_detail=CASE WHEN $7<>'' THEN $7 ELSE status_detail END,
  quota=CASE WHEN $8::text<>'{}' THEN $8::jsonb ELSE quota END,
  quota_kind=CASE WHEN $9<>'' THEN $9 ELSE quota_kind END,
  manual_status=CASE WHEN $10<>'' THEN $10 ELSE manual_status END,
  manual_status_at=CASE WHEN $10<>'' THEN $11 ELSE manual_status_at END,
  fail_count=$12,
  checked_at=CASE WHEN $13::timestamptz IS NULL THEN checked_at ELSE $13 END,
  next_probe_at=$14,
  updated_at=now()
WHERE uid=$1`, uid, s.pclass, s.provider, s.providerName, variant, status, detail,
			string(quotaJSON), s.quotaKind, s.manual, parseTimePtr(s.manualAt), s.failCount, checked, next)
		if err != nil {
			fmt.Fprintln(os.Stderr, "update entry:", err)
			continue
		}
		if s.baseURL != "" || s.model != "" {
			_, _ = pg.ExecContext(ctx, `
UPDATE cred_entries SET extra=jsonb_set(extra,'{base_url}',to_jsonb($2::text),true), updated_at=now()
WHERE uid=$1`, uid, s.baseURL)
		}
		// One legacy snapshot per credential.
		ts := time.Now()
		if checked != nil {
			ts = *checked
		}
		_, _ = pg.ExecContext(ctx, `
INSERT INTO cred_probes(uid,ts,mode,status,http,ms,summary,pclass,provider,variant,quota_kind,quota,source)
VALUES ($1,$2,'legacy',$3,$4,$5,$6,$7,$8,$9,$10,$11,'legacy')`,
			uid, ts, status, s.http, int(s.ms), s.availDesc, s.pclass, s.provider, variant, s.quotaKind, quotaJSON)
		updated++
	}

	// Import the full probe_log history for matched credentials.
	histRows, err := db.Query(`SELECT uid,ts,mode,status,http,ms,summary,data FROM probe_log ORDER BY seq`)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read probe_log:", err)
		os.Exit(1)
	}
	defer histRows.Close()
	for histRows.Next() {
		var legacyUID, ts, mode, status, summary, data string
		var http int
		var ms float64
		if err := histRows.Scan(&legacyUID, &ts, &mode, &status, &http, &ms, &summary, &data); err != nil {
			continue
		}
		var uid, pclass, provider, variant, quotaKind string
		err := pg.QueryRowContext(ctx, `SELECT uid,coalesce(pclass,''),coalesce(provider,''),coalesce(variant,''),coalesce(quota_kind,'')
FROM cred_entries WHERE extra->>'legacy_uid'=$1 AND archived=false LIMIT 1`, legacyUID).Scan(&uid, &pclass, &provider, &variant, &quotaKind)
		if err != nil {
			continue
		}
		st, _ := legacyStatus(status, summary)
		quota := map[string]any{}
		if json.Valid([]byte(data)) && strings.TrimSpace(data) != "" {
			_ = json.Unmarshal([]byte(data), &quota)
		}
		quotaJSON, _ := json.Marshal(quota)
		_, _ = pg.ExecContext(ctx, `
INSERT INTO cred_probes(uid,ts,mode,status,http,ms,summary,pclass,provider,variant,quota_kind,quota,source)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'legacy')`,
			uid, parseTime(ts), mode, st, http, int(ms), summary, pclass, provider, variant, quotaKind, quotaJSON)
		histories++
	}
	// A snapshot and the first probe_log row often describe the same event; keep
	// the richer history row and drop the duplicate snapshot.
	_, _ = pg.ExecContext(ctx, `
DELETE FROM cred_probes a USING cred_probes b
WHERE a.uid=b.uid AND a.source='legacy' AND b.source='legacy'
  AND a.mode='legacy' AND b.mode<>'legacy'
  AND a.ts=b.ts AND a.summary=b.summary`)
	out, _ := json.MarshalIndent(map[string]any{"updated": updated, "histories": histories, "skipped": skipped}, "", "  ")
	fmt.Println(string(out))
}

func legacyStatus(avail, desc string) (string, string) {
	switch strings.TrimSpace(avail) {
	case "✅":
		return credhunt.StatusValid, desc
	case "❌":
		return credhunt.StatusInvalid, desc
	case "⚠️":
		if strings.Contains(desc, "余额") || strings.Contains(desc, "欠费") || strings.Contains(desc, "用尽") {
			return credhunt.StatusNoBalance, desc
		}
		if strings.Contains(desc, "限流") || strings.Contains(desc, "429") {
			return credhunt.StatusRateLimited, desc
		}
		return credhunt.StatusUnknown, desc
	case "❓", "":
		return "", desc
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
	return time.Now()
}

func parseTimePtr(s string) *time.Time {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	t := parseTime(s)
	return &t
}
