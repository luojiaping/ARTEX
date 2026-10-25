package fofa

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Account is the /api/v1/info/my payload.
type Account struct {
	Username        string  `json:"username"`
	Email           string  `json:"email"`
	IsVIP           bool    `json:"isvip"`
	VIPLevel        int     `json:"vip_level"`
	FCoin           float64 `json:"fcoin"`
	FofaPoint       float64 `json:"fofa_point"`
	RemainFreePoint int     `json:"remain_free_point"`
	RemainAPIQuery  int     `json:"remain_api_query"`
	RemainAPIData   int     `json:"remain_api_data"`
	IsVerified      bool    `json:"is_verified"`
}

// SearchResult is the payload of search/all and search/next.
type SearchResult struct {
	Error           bool            `json:"error"`
	Mode            string          `json:"mode"`
	Page            int             `json:"page"`
	Size            int             `json:"size"` // rows in THIS page, not the total hit count
	Query           string          `json:"query"`
	Results         [][]any         `json:"results"`
	Next            json.RawMessage `json:"next"`
	ConsumedFpoint  float64         `json:"consumed_fpoint"`
	RequiredFpoints float64         `json:"required_fpoints"`
	Tip             string          `json:"tip"`
}

// NextID renders the cursor id as a string regardless of whether FOFA returned
// a JSON string or number.
func (r *SearchResult) NextID() string {
	if len(r.Next) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(r.Next, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(r.Next, &n); err == nil {
		return n.String()
	}
	return strings.Trim(string(r.Next), `"`)
}

// AggItem is one bucket of a stats aggregation.
type AggItem struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// StatsResult is the /api/v1/search/stats payload.
type StatsResult struct {
	Error          bool                 `json:"error"`
	Distinct       map[string]int       `json:"distinct"`
	Aggs           map[string][]AggItem `json:"aggs"`
	LastUpdateTime string               `json:"lastupdatetime"`
}

// HostInfo is the /api/v1/host/{host} payload.
type HostInfo struct {
	Error       bool     `json:"error"`
	Host        string   `json:"host"`
	IP          string   `json:"ip"`
	ASN         int      `json:"asn"`
	Org         string   `json:"org"`
	CountryName string   `json:"country_name"`
	CountryCode string   `json:"country_code"`
	Protocol    []string `json:"protocol"`
	Port        []int    `json:"port"`
	Category    []string `json:"category"`
	Product     []string `json:"product"`
	Domain      []string `json:"domain"`
	UpdateTime  string   `json:"update_time"`
}

// APIError is a FOFA application-level error.
type APIError struct {
	Code int    // FOFA code (-700, -4, -5, -1, 820000) or 0 when unknown
	Msg  string // raw errmsg as returned
	HTTP int    // HTTP status, 0 for transport errors
}

func (e *APIError) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return fmt.Sprintf("FOFA API error (code=%d http=%d)", e.Code, e.HTTP)
}

// Retryable reports whether another attempt may succeed (rate limit / 5xx).
func (e *APIError) Retryable() bool {
	return e.Code == -5 || e.HTTP == 429 || e.HTTP >= 500
}

func (e *APIError) IsKeyInvalid() bool {
	return e.Code == -700 || e.Code == -401 || e.HTTP == 401 || e.HTTP == 403
}

func (e *APIError) IsQuota() bool { return e.Code == -4 || e.Code == -6 }

func (e *APIError) IsBadQuery() bool { return e.Code == -1 }

func (e *APIError) IsNextForbidden() bool { return e.Code == 820000 }

// IsForbidden covers endpoints the account is not entitled to (e.g. stats/host
// on some API tiers return [-403] 访问权限不足).
func (e *APIError) IsForbidden() bool { return e.Code == -403 }

// ErrNoKey is returned when the client has no API key configured.
var ErrNoKey = fmt.Errorf("未配置 FOFA API Key")

var codeRe = regexp.MustCompile(`\[(-?\d+)\]`)

func parseErrCode(env envelope) int {
	if env.Code != 0 {
		return env.Code
	}
	return codeFromMessage(env.ErrMsg)
}

func codeFromMessage(msg string) int {
	if m := codeRe.FindStringSubmatch(msg); len(m) == 2 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	return 0
}
