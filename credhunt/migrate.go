package credhunt

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// ImportRow is one row from the legacy console ledger. Import preserves the
// original status/timestamps and does not reject placeholders (the old ledger was
// already human/AI curated); it still skips empty values and IM bot credentials.
type ImportRow struct {
	Kind         string
	Provider     string
	Service      string
	Value        string
	SourceURL    string
	EvidencePath string
	Status       string
	StatusDetail string
	Risk         string
	Context      string
	Quota        map[string]any
	Extra        map[string]any
	FoundAt      time.Time
	CheckedAt    *time.Time
	Proxy        *ProxyMeta
	ParentUID    string
}

// ImportStats reports a migration batch.
type ImportStats struct {
	Inserted  int          `json:"inserted"`
	Duplicate int          `json:"duplicate"`
	Skipped   int          `json:"skipped"`
	Items     []SubmitItem `json:"items,omitempty"`
	Errors    []string     `json:"errors,omitempty"`
}

// Import writes legacy rows without the interactive placeholder gate. Dedupe is
// still enforced by fingerprint.
func (s *Store) Import(ctx context.Context, rows []ImportRow) ImportStats {
	var out ImportStats
	for _, row := range rows {
		item, err := s.importOne(ctx, row)
		if err != nil {
			out.Errors = append(out.Errors, err.Error())
			out.Skipped++
			continue
		}
		switch item.Result {
		case "inserted":
			out.Inserted++
		case "duplicate":
			out.Duplicate++
		default:
			out.Skipped++
		}
		out.Items = append(out.Items, item)
	}
	return out
}

func (s *Store) importOne(ctx context.Context, row ImportRow) (SubmitItem, error) {
	value := strings.TrimSpace(row.Value)
	if value == "" {
		return SubmitItem{Result: "skipped", Reason: "empty value"}, nil
	}
	if IsIMService(row.Service) {
		return SubmitItem{Result: "skipped", Reason: "IM bot credentials are not stored"}, nil
	}
	kind := sanitizeUTF8(strings.TrimSpace(row.Kind))
	if kind == "" {
		kind = inferKind(value)
	}
	provider := sanitizeUTF8(strings.TrimSpace(row.Provider))
	if provider == "" {
		provider, _ = Classify(kind, value, row.Service, row.Context, row.SourceURL)
	}
	if provider == "" {
		provider = "unknown"
	}
	status := sanitizeUTF8(strings.TrimSpace(row.Status))
	if status == "" {
		status = StatusUnverified
	}
	risk := sanitizeUTF8(strings.TrimSpace(row.Risk))
	if risk == "" {
		risk = "P2"
	}
	sourceURL := sanitizeUTF8(strings.TrimSpace(row.SourceURL))
	if sourceURL == "" {
		sourceURL = "legacy://console"
	}
	evidence := sanitizeUTF8(strings.TrimSpace(row.EvidencePath))
	if evidence == "" {
		evidence = "console.db"
	}
	service := sanitizeUTF8(row.Service)
	contextText := sanitizeUTF8(row.Context)
	statusDetail := sanitizeUTF8(row.StatusDetail)
	fp := fingerprint(s.key, "", value)
	uid := uidFor(fp)
	var existing string
	err := s.db.QueryRowContext(ctx, `SELECT uid FROM cred_entries WHERE fingerprint=$1 AND archived=false LIMIT 1`, fp).Scan(&existing)
	if err == nil {
		return SubmitItem{UID: existing, Kind: kind, Provider: provider, Masked: maskSecret(value),
			Status: status, Result: "duplicate", Reason: "same value already in ledger"}, nil
	}
	if err != sql.ErrNoRows {
		return SubmitItem{}, err
	}
	secretEnc, err := encrypt(s.key, []byte(value))
	if err != nil {
		return SubmitItem{}, err
	}
	hint := maskSecret(value)
	if kind == KindProxyNode && service != "" {
		hint = service
	}
	hint = sanitizeUTF8(hint)
	extra := row.Extra
	if extra == nil {
		extra = map[string]any{}
	}
	extra["imported"] = true
	extraJSON, _ := json.Marshal(extra)
	quota := row.Quota
	if quota == nil {
		quota = map[string]any{}
	}
	quotaJSON, _ := json.Marshal(quota)
	foundAt := row.FoundAt
	if foundAt.IsZero() {
		foundAt = time.Now()
	}
	var checked any
	if row.CheckedAt != nil && !row.CheckedAt.IsZero() {
		checked = *row.CheckedAt
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO cred_entries(uid,kind,provider,service,secret_enc,secret_hint,fingerprint,status,status_detail,
  quota,risk,source_url,evidence_path,context,extra,found_at,checked_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		uid, kind, provider, service, secretEnc, hint, fp, status, statusDetail,
		quotaJSON, risk, sourceURL, evidence, contextText, extraJSON, foundAt, checked)
	if err != nil {
		return SubmitItem{}, err
	}
	if row.Proxy != nil && kind == KindProxyNode {
		proxyStatus := row.Proxy.Status
		if proxyStatus == "" {
			proxyStatus = status
		}
		_ = s.UpsertProxyNode(ctx, uid, row.ParentUID, row.Proxy.Protocol, row.Proxy.Server, row.Proxy.Port, proxyStatus, row.Proxy.Message)
	}
	return SubmitItem{UID: uid, Kind: kind, Provider: provider, Masked: hint, Status: status, Result: "inserted"}, nil
}

// sanitizeUTF8 keeps PostgreSQL text columns happy when the legacy SQLite file
// contains bytes that are not valid UTF-8.
func sanitizeUTF8(s string) string {
	return strings.ToValidUTF8(s, "\uFFFD")
}
