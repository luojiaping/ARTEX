// Package credhunt implements the credential-hunting pipeline: safe evidence
// fetching, provider classification, encrypted credential ledger, LLM key
// verification and proxy-node probing.
package credhunt

import (
	"encoding/json"
	"time"
)

// Kinds stored in cred_entries.kind.
const (
	KindLLMKey            = "llm_key"
	KindOAuth             = "oauth"
	KindAPIKey            = "api_key"
	KindToken             = "token"
	KindPassword          = "credential"
	KindPrivateKey        = "private_key"
	KindProxyNode         = "proxy_node"
	KindProxySubscription = "proxy_subscription"
	KindOther             = "other"
)

// Statuses stored in cred_entries.status.
const (
	StatusUnverified  = "unverified"
	StatusValid       = "valid"
	StatusInvalid     = "invalid"
	StatusNoBalance   = "no_balance"
	StatusRateLimited = "rate_limited"
	StatusUnknown     = "unknown"
	StatusUnsupported = "unsupported"
	StatusBlocked     = "blocked"
	StatusArchived    = "archived"
)

// Input is one credential submitted by an agent or the API.
type Input struct {
	Kind           string          `json:"kind,omitempty"`
	Value          string          `json:"value"`
	Package        json.RawMessage `json:"package,omitempty"` // whole OAuth/token package; stored verbatim
	Provider       string          `json:"provider,omitempty"`
	Service        string          `json:"service,omitempty"`
	SourceURL      string          `json:"source_url"`
	EvidencePath   string          `json:"evidence_path"`
	Risk           string          `json:"risk,omitempty"`
	Context        string          `json:"context,omitempty"`
	BaseURL        string          `json:"base_url,omitempty"` // relay/unknown provider override
	AssetID        int64           `json:"asset_id,omitempty"`
	TaskID         int64           `json:"task_id,omitempty"`
	ConversationID int64           `json:"conversation_id,omitempty"`
	ParentUID      string          `json:"parent_uid,omitempty"` // proxy subscription parent
}

// Entry is the model-facing/API view of a credential. Secret is never included.
type Entry struct {
	ID             int64          `json:"id"`
	UID            string         `json:"uid"`
	Kind           string         `json:"kind"`
	Provider       string         `json:"provider"`
	Service        string         `json:"service"`
	Masked         string         `json:"masked"`
	Status         string         `json:"status"`
	StatusDetail   string         `json:"status_detail"`
	Quota          map[string]any `json:"quota,omitempty"`
	Risk           string         `json:"risk"`
	SourceURL      string         `json:"source_url"`
	EvidencePath   string         `json:"evidence_path"`
	AssetID        int64          `json:"asset_id,omitempty"`
	TaskID         int64          `json:"task_id,omitempty"`
	ConversationID int64          `json:"conversation_id,omitempty"`
	Context        string         `json:"context,omitempty"`
	Extra          map[string]any `json:"extra,omitempty"`
	FoundAt        time.Time      `json:"found_at"`
	CheckedAt      *time.Time     `json:"checked_at,omitempty"`
	Archived       bool           `json:"archived"`
	Proxy          *ProxyMeta     `json:"proxy,omitempty"`
}

// ProxyMeta is the probe-facing metadata for a proxy_node entry.
type ProxyMeta struct {
	Protocol    string     `json:"protocol"`
	Server      string     `json:"server"`
	Port        int        `json:"port"`
	Status      string     `json:"status"`
	DelayMS     int        `json:"delay_ms,omitempty"`
	ExitIP      string     `json:"exit_ip,omitempty"`
	ExitCountry string     `json:"exit_country,omitempty"`
	Message     string     `json:"message,omitempty"`
	CheckedAt   *time.Time `json:"checked_at,omitempty"`
}

// Source is one provenance row for a credential.
type Source struct {
	URL          string    `json:"url"`
	EvidencePath string    `json:"evidence_path"`
	Note         string    `json:"note,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// SubmitResult reports a batch insert.
type SubmitResult struct {
	Inserted  int          `json:"inserted"`
	Duplicate int          `json:"duplicate"`
	Excluded  int          `json:"excluded"`
	Skipped   int          `json:"skipped"`
	Items     []SubmitItem `json:"items"`
	Errors    []string     `json:"errors,omitempty"`
}

// SubmitItem is the per-input outcome.
type SubmitItem struct {
	UID      string `json:"uid,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Provider string `json:"provider,omitempty"`
	Masked   string `json:"masked,omitempty"`
	Status   string `json:"status,omitempty"`
	Result   string `json:"result"` // inserted|duplicate|excluded|skipped
	Reason   string `json:"reason,omitempty"`
}

// Filter selects ledger rows.
type Filter struct {
	Kinds    []string
	Provider string
	Status   string
	Query    string
	TaskID   int64
	Archived bool
	Limit    int
	Offset   int
}

// Stats is the ledger dashboard aggregate.
type Stats struct {
	Total      int64            `json:"total"`
	Valid      int64            `json:"valid"`
	Invalid    int64            `json:"invalid"`
	Unverified int64            `json:"unverified"`
	ProxyAlive int64            `json:"proxy_alive"`
	ProxyDead  int64            `json:"proxy_dead"`
	ByKind     map[string]int64 `json:"by_kind"`
	ByProvider map[string]int64 `json:"by_provider"`
	ByStatus   map[string]int64 `json:"by_status"`
}

// VerifyResult is one verification outcome.
type VerifyResult struct {
	UID        string         `json:"uid"`
	Kind       string         `json:"kind"`
	Provider   string         `json:"provider"`
	Service    string         `json:"service"`
	Status     string         `json:"status"`
	Detail     string         `json:"detail"`
	HTTPStatus int            `json:"http_status,omitempty"`
	Endpoint   string         `json:"endpoint,omitempty"`
	Model      string         `json:"model,omitempty"`
	ElapsedMS  int64          `json:"elapsed_ms"`
	Quota      map[string]any `json:"quota,omitempty"`
	CheckedAt  time.Time      `json:"checked_at"`
}

// ProbeResult is one proxy-node probe outcome.
type ProbeResult struct {
	UID      string `json:"uid"`
	Protocol string `json:"protocol"`
	Server   string `json:"server"`
	Port     int    `json:"port"`
	Status   string `json:"status"` // alive|dead|error
	DelayMS  int    `json:"delay_ms,omitempty"`
	URL      string `json:"url,omitempty"`
	Message  string `json:"message,omitempty"`
}

// Link is one entry parsed from a directory listing.
type Link struct {
	URL   string `json:"url"`
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size,omitempty"`
	Score int    `json:"score,omitempty"`
}

// FetchResult is the safe-fetch output used by the hunt tools.
type FetchResult struct {
	URL          string `json:"url"`
	FinalURL     string `json:"final_url"`
	Status       int    `json:"status"`
	ContentType  string `json:"content_type"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
	Title        string `json:"title,omitempty"`
	EvidencePath string `json:"evidence_path"`
	IsListing    bool   `json:"is_listing"`
	Truncated    bool   `json:"truncated"`
	Text         string `json:"text,omitempty"`
	Body         []byte `json:"-"` // full body for internal parsers; never serialized
	Dirs         []Link `json:"dirs,omitempty"`
	Files        []Link `json:"files,omitempty"`
	Candidates   []Link `json:"candidates,omitempty"`
	Error        string `json:"error,omitempty"`
}

// BatchTarget is one target summary from hunt_batch.
type BatchTarget struct {
	URL          string `json:"url"`
	Status       int    `json:"status"`
	Title        string `json:"title,omitempty"`
	IsListing    bool   `json:"is_listing"`
	DirCount     int    `json:"dir_count"`
	FileCount    int    `json:"file_count"`
	EvidencePath string `json:"evidence_path,omitempty"`
	Candidates   []Link `json:"candidates,omitempty"`
	Error        string `json:"error,omitempty"`
}
