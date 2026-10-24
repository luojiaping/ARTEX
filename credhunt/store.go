package credhunt

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	pgdb "github.com/Autumn-27/artex/db"
)

// Store is the encrypted credential ledger.
type Store struct {
	db  *pgdb.DB
	key []byte
}

// NewStore opens the ledger and loads/creates its encryption key.
func NewStore(db *pgdb.DB, keyPath string) (*Store, error) {
	key, err := loadOrCreateKey(keyPath)
	if err != nil {
		return nil, err
	}
	return &Store{db: db, key: key}, nil
}

// Submit validates, classifies, encrypts and deduplicates a batch.
func (s *Store) Submit(ctx context.Context, inputs []Input) (SubmitResult, error) {
	var out SubmitResult
	for _, in := range inputs {
		item, err := s.submitOne(ctx, in)
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
		case "excluded":
			out.Excluded++
		default:
			out.Skipped++
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (s *Store) submitOne(ctx context.Context, in Input) (SubmitItem, error) {
	value := strings.TrimSpace(in.Value)
	if len(in.Package) > 0 && string(in.Package) != "null" {
		value = string(in.Package)
	}
	if value == "" {
		return SubmitItem{Result: "skipped", Reason: "empty value"}, nil
	}
	if len(value) < 6 || LooksLikePlaceholder(value) {
		return SubmitItem{Result: "skipped", Reason: "placeholder or too short"}, nil
	}
	service := strings.TrimSpace(in.Service)
	if IsIMService(service) || strings.EqualFold(strings.TrimSpace(in.Kind), "bot-token") {
		return SubmitItem{Result: "excluded", Reason: "IM bot credentials are not stored"}, nil
	}
	if strings.TrimSpace(in.SourceURL) == "" || strings.TrimSpace(in.EvidencePath) == "" {
		return SubmitItem{Result: "skipped", Reason: "source_url and evidence_path are required"}, nil
	}

	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = inferKind(value)
	}
	provider := strings.TrimSpace(in.Provider)
	baseOverride := strings.TrimSpace(in.BaseURL)
	if provider == "" {
		var inferredBase string
		provider, inferredBase = Classify(kind, value, service, in.Context, in.SourceURL)
		if baseOverride == "" {
			baseOverride = inferredBase
		}
	}
	if provider == "" {
		provider = "unknown"
	}
	if kind == "" {
		kind = KindOther
	}

	extra := map[string]any{}
	if baseOverride != "" {
		extra["base_url"] = baseOverride
	}
	if strings.TrimSpace(in.Context) != "" {
		extra["context"] = strings.TrimSpace(in.Context)
	}
	extra["input_kind"] = kind

	fp := fingerprint(s.key, "", value)
	uid := uidFor(fp)

	var existing string
	err := s.db.QueryRowContext(ctx, `SELECT uid FROM cred_entries WHERE fingerprint=$1 AND archived=false LIMIT 1`, fp).Scan(&existing)
	if err == nil {
		_ = s.addSource(ctx, existing, in.SourceURL, in.EvidencePath, in.Context)
		return SubmitItem{UID: existing, Kind: kind, Provider: provider, Masked: maskSecret(value),
			Status: StatusUnverified, Result: "duplicate", Reason: "same value already in ledger"}, nil
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
	extraJSON, _ := json.Marshal(extra)
	risk := strings.TrimSpace(in.Risk)
	if risk == "" {
		risk = "P2"
	}
	if in.Kind == KindProxyNode || in.Kind == KindProxySubscription {
		risk = defaultRisk(risk, "P1")
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO cred_entries(uid,kind,provider,service,secret_enc,secret_hint,fingerprint,status,risk,
  source_url,evidence_path,asset_id,task_id,conversation_id,context,extra,found_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,now())`,
		uid, kind, provider, service, secretEnc, hint, fp, StatusUnverified, risk,
		strings.TrimSpace(in.SourceURL), strings.TrimSpace(in.EvidencePath),
		nullInt(in.AssetID), nullInt(in.TaskID), nullInt(in.ConversationID),
		strings.TrimSpace(in.Context), extraJSON)
	if err != nil {
		return SubmitItem{}, err
	}
	return SubmitItem{UID: uid, Kind: kind, Provider: provider, Masked: maskSecret(value),
		Status: StatusUnverified, Result: "inserted"}, nil
}

func (s *Store) addSource(ctx context.Context, uid, sourceURL, evidence, note string) error {
	if strings.TrimSpace(sourceURL) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO cred_sources(cred_id,url,evidence_path,note)
SELECT id,$2,$3,$4 FROM cred_entries WHERE uid=$1
ON CONFLICT (cred_id,url,evidence_path) DO NOTHING`, uid, strings.TrimSpace(sourceURL),
		strings.TrimSpace(evidence), strings.TrimSpace(note))
	return err
}

func inferKind(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.HasPrefix(v, "vless://"), strings.HasPrefix(v, "vmess://"), strings.HasPrefix(v, "trojan://"),
		strings.HasPrefix(v, "ss://"), strings.HasPrefix(v, "hysteria2://"), strings.HasPrefix(v, "hy2://"),
		strings.HasPrefix(v, "tuic://"), strings.HasPrefix(v, "anytls://"), strings.HasPrefix(v, "socks5://"),
		strings.HasPrefix(v, "http://"), strings.HasPrefix(v, "https://"):
		if strings.Contains(v, "subscribe") || strings.Contains(v, "/sub") || strings.HasSuffix(v, ".yaml") {
			return KindProxySubscription
		}
		return KindProxyNode
	case strings.Contains(v, `"refresh_token"`), strings.Contains(v, `"access_token"`),
		strings.HasPrefix(v, "sk-ant-oat"):
		return KindOAuth
	case strings.HasPrefix(v, "sk-"), strings.HasPrefix(v, "gsk_"), strings.HasPrefix(v, "nvapi-"),
		strings.HasPrefix(v, "xai-"), strings.HasPrefix(v, "aiza"):
		return KindLLMKey
	case strings.Contains(v, "-----begin"):
		return KindPrivateKey
	default:
		return KindOther
	}
}

func defaultRisk(current, fallback string) string {
	if strings.TrimSpace(current) == "" {
		return fallback
	}
	return current
}

func nullInt(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}

// List returns masked entries plus the total matching count.
func (s *Store) List(ctx context.Context, f Filter) ([]Entry, int64, error) {
	if f.Limit <= 0 || f.Limit > 2000 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	var where []string
	var args []any
	add := func(clause string, v any) string {
		args = append(args, v)
		return fmt.Sprintf(clause, len(args))
	}
	if !f.Archived {
		where = append(where, "c.archived=false")
	}
	if len(f.Kinds) > 0 {
		marks := make([]string, 0, len(f.Kinds))
		for _, k := range f.Kinds {
			args = append(args, k)
			marks = append(marks, fmt.Sprintf("$%d", len(args)))
		}
		where = append(where, "c.kind IN ("+strings.Join(marks, ",")+")")
	}
	if strings.TrimSpace(f.Provider) != "" {
		where = append(where, add("c.provider = $%d", strings.TrimSpace(f.Provider)))
	}
	if strings.TrimSpace(f.Status) != "" {
		where = append(where, add("c.status = $%d", strings.TrimSpace(f.Status)))
	}
	if f.TaskID > 0 {
		where = append(where, add("c.task_id = $%d", f.TaskID))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		like := "%" + q + "%"
		idx := len(args) + 1
		args = append(args, like)
		where = append(where, fmt.Sprintf("(c.service ILIKE $%d OR c.provider ILIKE $%d OR c.source_url ILIKE $%d OR c.context ILIKE $%d)", idx, idx, idx, idx))
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM cred_entries c`+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := s.db.QueryContext(ctx, `
SELECT c.id,c.uid,c.kind,c.provider,c.service,c.secret_hint,c.status,c.status_detail,c.quota,c.risk,
       c.source_url,c.evidence_path,COALESCE(c.asset_id,0),COALESCE(c.task_id,0),COALESCE(c.conversation_id,0),
       c.context,c.extra,c.found_at,c.checked_at,c.archived,
       COALESCE(p.protocol,''),COALESCE(p.server,''),COALESCE(p.port,0),COALESCE(p.status,''),
       COALESCE(p.delay_ms,0),COALESCE(p.exit_ip,''),COALESCE(p.exit_country,''),COALESCE(p.message,''),p.checked_at
FROM cred_entries c
LEFT JOIN proxy_nodes p ON p.cred_id=c.id`+clause+`
ORDER BY c.found_at DESC, c.id DESC LIMIT $`+fmt.Sprint(len(args)-1)+` OFFSET $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]Entry, 0)
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// Get returns one masked entry and its sources.
func (s *Store) Get(ctx context.Context, uid string) (*Entry, []Source, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT c.id,c.uid,c.kind,c.provider,c.service,c.secret_hint,c.status,c.status_detail,c.quota,c.risk,
       c.source_url,c.evidence_path,COALESCE(c.asset_id,0),COALESCE(c.task_id,0),COALESCE(c.conversation_id,0),
       c.context,c.extra,c.found_at,c.checked_at,c.archived,
       COALESCE(p.protocol,''),COALESCE(p.server,''),COALESCE(p.port,0),COALESCE(p.status,''),
       COALESCE(p.delay_ms,0),COALESCE(p.exit_ip,''),COALESCE(p.exit_country,''),COALESCE(p.message,''),p.checked_at
FROM cred_entries c LEFT JOIN proxy_nodes p ON p.cred_id=c.id WHERE c.uid=$1`, uid)
	e, err := scanEntry(row)
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT url,evidence_path,note,created_at FROM cred_sources WHERE cred_id=$1 ORDER BY id`, e.ID)
	if err != nil {
		return &e, nil, nil
	}
	defer rows.Close()
	var src []Source
	for rows.Next() {
		var one Source
		if err := rows.Scan(&one.URL, &one.EvidencePath, &one.Note, &one.CreatedAt); err == nil {
			src = append(src, one)
		}
	}
	return &e, src, nil
}

// Reveal decrypts one secret. Callers must authorise and audit this.
func (s *Store) Reveal(ctx context.Context, uid string) (string, error) {
	var enc []byte
	if err := s.db.QueryRowContext(ctx, `SELECT secret_enc FROM cred_entries WHERE uid=$1 AND archived=false`, uid).Scan(&enc); err != nil {
		return "", err
	}
	raw, err := decrypt(s.key, enc)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// GetByUID returns the raw value and metadata for verification.
func (s *Store) GetByUID(ctx context.Context, uid string) (Entry, string, error) {
	e, _, err := s.Get(ctx, uid)
	if err != nil {
		return Entry{}, "", err
	}
	raw, err := s.Reveal(ctx, uid)
	return *e, raw, err
}

// ByUIDs loads verification inputs.
func (s *Store) ByUIDs(ctx context.Context, uids []string) ([]Entry, []string, error) {
	var out []Entry
	var secrets []string
	for _, uid := range uids {
		e, raw, err := s.GetByUID(ctx, uid)
		if err != nil {
			continue
		}
		out = append(out, e)
		secrets = append(secrets, raw)
	}
	return out, secrets, nil
}

// SelectForVerify picks rows by filter for batch verification.
func (s *Store) SelectForVerify(ctx context.Context, f Filter, limit int) ([]Entry, []string, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	f.Archived = false
	f.Limit = limit
	if len(f.Kinds) == 0 {
		// Proxy nodes use proxy_probe; generic password/private keys are not actively verified.
		f.Kinds = []string{KindLLMKey, KindOAuth, KindAPIKey, KindToken}
	}
	entries, _, err := s.List(ctx, f)
	if err != nil {
		return nil, nil, err
	}
	var secrets []string
	for _, e := range entries {
		raw, err := s.Reveal(ctx, e.UID)
		if err != nil {
			continue
		}
		secrets = append(secrets, raw)
	}
	return entries, secrets, nil
}

// UpdateVerification persists a verdict and quota.
func (s *Store) UpdateVerification(ctx context.Context, uid, status, detail string, quota map[string]any) error {
	var q []byte
	if quota != nil {
		q, _ = json.Marshal(quota)
	}
	if len(q) == 0 {
		q = []byte("{}")
	}
	_, err := s.db.ExecContext(ctx, `UPDATE cred_entries SET status=$2,status_detail=$3,quota=$4,checked_at=now(),updated_at=now() WHERE uid=$1`,
		uid, status, detail, q)
	return err
}

// Archive hides an entry without deleting its evidence.
func (s *Store) Archive(ctx context.Context, uid string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE cred_entries SET archived=true,status=$2,updated_at=now() WHERE uid=$1`, uid, StatusArchived)
	return err
}

// Stats aggregates the dashboard.
func (s *Store) Stats(ctx context.Context) (Stats, error) {
	out := Stats{ByKind: map[string]int64{}, ByProvider: map[string]int64{}, ByStatus: map[string]int64{}}
	if err := s.db.QueryRowContext(ctx, `
SELECT count(*),
       count(*) FILTER (WHERE status='valid'),
       count(*) FILTER (WHERE status='invalid'),
       count(*) FILTER (WHERE status='unverified'),
       count(*) FILTER (WHERE kind IN ('proxy_node','proxy_subscription') AND status='valid'),
       count(*) FILTER (WHERE kind IN ('proxy_node','proxy_subscription') AND status='invalid')
FROM cred_entries WHERE archived=false`).Scan(&out.Total, &out.Valid, &out.Invalid, &out.Unverified, &out.ProxyAlive, &out.ProxyDead); err != nil {
		return out, err
	}
	read := func(sqlText string, dst map[string]int64) error {
		rows, err := s.db.QueryContext(ctx, sqlText)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k string
			var n int64
			if err := rows.Scan(&k, &n); err != nil {
				return err
			}
			dst[k] = n
		}
		return rows.Err()
	}
	if err := read(`SELECT kind,count(*) FROM cred_entries WHERE archived=false GROUP BY kind`, out.ByKind); err != nil {
		return out, err
	}
	if err := read(`SELECT provider,count(*) FROM cred_entries WHERE archived=false GROUP BY provider`, out.ByProvider); err != nil {
		return out, err
	}
	if err := read(`SELECT status,count(*) FROM cred_entries WHERE archived=false GROUP BY status`, out.ByStatus); err != nil {
		return out, err
	}
	return out, nil
}

// UpsertProxyNode records probe metadata for a proxy_node entry.
func (s *Store) UpsertProxyNode(ctx context.Context, credUID, parentUID, protocol, server string, port int, status, message string) error {
	var credID int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM cred_entries WHERE uid=$1`, credUID).Scan(&credID); err != nil {
		return err
	}
	var parentID any
	if parentUID != "" {
		var pid int64
		if err := s.db.QueryRowContext(ctx, `SELECT id FROM cred_entries WHERE uid=$1`, parentUID).Scan(&pid); err == nil {
			parentID = pid
		}
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO proxy_nodes(cred_id,parent_id,protocol,server,port,status,message)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (cred_id) DO UPDATE SET parent_id=EXCLUDED.parent_id,protocol=EXCLUDED.protocol,
 server=EXCLUDED.server,port=EXCLUDED.port,status=EXCLUDED.status,message=EXCLUDED.message`,
		credID, parentID, protocol, server, port, status, message)
	return err
}

// UpdateProxyProbe stores a probe verdict.
func (s *Store) UpdateProxyProbe(ctx context.Context, uid, status string, delayMS int, message string) error {
	var credID int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM cred_entries WHERE uid=$1`, uid).Scan(&credID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE proxy_nodes SET status=$2,delay_ms=$3,message=$4,checked_at=now() WHERE cred_id=$1`,
		credID, status, delayMS, message)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE cred_entries SET status=$2,checked_at=now(),updated_at=now() WHERE uid=$1`, uid, status)
	return err
}

// ProxyRows returns proxy entries with decrypted configs for probing.
func (s *Store) ProxyRows(ctx context.Context, uids []string, limit int) ([]Entry, []string, []map[string]any, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var rows *sql.Rows
	var err error
	if len(uids) > 0 {
		marks := make([]string, 0, len(uids))
		args := make([]any, 0, len(uids)+1)
		for _, uid := range uids {
			args = append(args, uid)
			marks = append(marks, fmt.Sprintf("$%d", len(args)))
		}
		args = append(args, limit)
		rows, err = s.db.QueryContext(ctx, `
SELECT c.id,c.uid,c.kind,c.provider,c.service,c.secret_hint,c.status,c.status_detail,c.quota,c.risk,
       c.source_url,c.evidence_path,COALESCE(c.asset_id,0),COALESCE(c.task_id,0),COALESCE(c.conversation_id,0),
       c.context,c.extra,c.found_at,c.checked_at,c.archived,
       COALESCE(p.protocol,''),COALESCE(p.server,''),COALESCE(p.port,0),COALESCE(p.status,''),
       COALESCE(p.delay_ms,0),COALESCE(p.exit_ip,''),COALESCE(p.exit_country,''),COALESCE(p.message,''),p.checked_at
FROM cred_entries c LEFT JOIN proxy_nodes p ON p.cred_id=c.id
WHERE c.archived=false AND c.kind='proxy_node' AND c.uid IN (`+strings.Join(marks, ",")+`) ORDER BY c.id LIMIT $`+fmt.Sprint(len(args)), args...)
	} else {
		rows, err = s.db.QueryContext(ctx, `
SELECT c.id,c.uid,c.kind,c.provider,c.service,c.secret_hint,c.status,c.status_detail,c.quota,c.risk,
       c.source_url,c.evidence_path,COALESCE(c.asset_id,0),COALESCE(c.task_id,0),COALESCE(c.conversation_id,0),
       c.context,c.extra,c.found_at,c.checked_at,c.archived,
       COALESCE(p.protocol,''),COALESCE(p.server,''),COALESCE(p.port,0),COALESCE(p.status,''),
       COALESCE(p.delay_ms,0),COALESCE(p.exit_ip,''),COALESCE(p.exit_country,''),COALESCE(p.message,''),p.checked_at
FROM cred_entries c LEFT JOIN proxy_nodes p ON p.cred_id=c.id
WHERE c.archived=false AND c.kind='proxy_node' ORDER BY c.id DESC LIMIT $1`, limit)
	}
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()
	var entries []Entry
	var secrets []string
	var configs []map[string]any
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, nil, nil, err
		}
		raw, err := s.Reveal(ctx, e.UID)
		if err != nil {
			continue
		}
		var cfg map[string]any
		if json.Unmarshal([]byte(raw), &cfg) != nil {
			cfg = map[string]any{}
		}
		entries = append(entries, e)
		secrets = append(secrets, raw)
		configs = append(configs, cfg)
	}
	return entries, secrets, configs, rows.Err()
}

// RecordRun writes one batch-scan audit row.
func (s *Store) RecordRun(ctx context.Context, taskID int64, note string, stats map[string]any) (int64, error) {
	b, _ := json.Marshal(stats)
	var id int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO hunt_runs(task_id,scope_note,stats) VALUES ($1,$2,$3) RETURNING id`,
		nullInt(taskID), strings.TrimSpace(note), b).Scan(&id)
	return id, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEntry(sc rowScanner) (Entry, error) {
	var e Entry
	var quota, extra []byte
	var checked, proxyChecked sql.NullTime
	var assetID, taskID, convID int64
	var pProto, pServer, pStatus, pExitIP, pExitCountry, pMessage string
	var pPort, pDelay int
	err := sc.Scan(&e.ID, &e.UID, &e.Kind, &e.Provider, &e.Service, &e.Masked, &e.Status, &e.StatusDetail,
		&quota, &e.Risk, &e.SourceURL, &e.EvidencePath, &assetID, &taskID, &convID,
		&e.Context, &extra, &e.FoundAt, &checked, &e.Archived,
		&pProto, &pServer, &pPort, &pStatus, &pDelay, &pExitIP, &pExitCountry, &pMessage, &proxyChecked)
	if err != nil {
		return e, err
	}
	e.AssetID, e.TaskID, e.ConversationID = assetID, taskID, convID
	if checked.Valid {
		e.CheckedAt = &checked.Time
	}
	if len(quota) > 0 {
		_ = json.Unmarshal(quota, &e.Quota)
	}
	if len(extra) > 0 {
		_ = json.Unmarshal(extra, &e.Extra)
	}
	if pProto != "" || pServer != "" {
		e.Proxy = &ProxyMeta{Protocol: pProto, Server: pServer, Port: pPort, Status: pStatus,
			DelayMS: pDelay, ExitIP: pExitIP, ExitCountry: pExitCountry, Message: pMessage}
		if proxyChecked.Valid {
			e.Proxy.CheckedAt = &proxyChecked.Time
		}
	}
	return e, nil
}

// Ensure schema columns exist in tests that create a minimal DB.
