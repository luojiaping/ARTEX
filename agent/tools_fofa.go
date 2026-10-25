package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Autumn-27/artex/fofa"
	actool "github.com/Autumn-27/norma/tool"
)

// FofaKeyProvider returns the current FOFA credentials. The server wires it in
// wireTools() to the settings table, so editing the key takes effect without a
// restart. nil (tests / no DB) leaves the tools reporting "not configured".
var FofaKeyProvider func() (key, baseURL string, ratePerSec float64)

const (
	fofaDefaultFields = "host,ip,port,title,domain,server,protocol,icp,country,province,city,asn,org"
	fofaWarnQuota     = 1000
)

var (
	fofaClientMu   sync.Mutex
	fofaClient     *fofa.Client
	fofaClientFP   string
	fofaClientBase string
	fofaClientRate float64

	fofaQuotaMu sync.Mutex
	fofaQuotaFP string
	fofaQuota   *fofa.Account
	fofaQuotaAt time.Time
)

func fofaFingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:8])
}

// fofaClientFor returns a cached client so every tool call shares one rate
// limiter. Rebuilds only when the key/base/rate actually change.
func fofaClientFor() (*fofa.Client, error) {
	if FofaKeyProvider == nil {
		return nil, fmt.Errorf("FOFA 未配置：缺少运行时设置提供者")
	}
	key, base, rate := FofaKeyProvider()
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("FOFA 未配置：请在「系统配置 → FOFA」填入 API Key（或设置 fofa_key）")
	}
	fp := fofaFingerprint(key)

	fofaClientMu.Lock()
	defer fofaClientMu.Unlock()
	if fofaClient != nil && fofaClientFP == fp && fofaClientBase == base && fofaClientRate == rate {
		return fofaClient, nil
	}
	c := fofa.New(key, base)
	if rate > 0 {
		c.SetRatePerSecond(rate)
	}
	fofaClient, fofaClientFP, fofaClientBase, fofaClientRate = c, fp, base, rate
	return c, nil
}

// fofaQuotaSnapshot returns a cached account/quota snapshot (5 min TTL, 0 query
// cost). Failures are silent: quota info is a hint, never a reason to fail a query.
func fofaQuotaSnapshot(ctx context.Context, c *fofa.Client) *fofa.Account {
	key, _, _ := FofaKeyProvider()
	fp := fofaFingerprint(key)

	fofaQuotaMu.Lock()
	if fofaQuota != nil && fofaQuotaFP == fp && time.Since(fofaQuotaAt) < 5*time.Minute {
		acc := fofaQuota
		fofaQuotaMu.Unlock()
		return acc
	}
	fofaQuotaMu.Unlock()

	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	acc, err := c.Account(cctx)
	if err != nil {
		return nil
	}
	fofaQuotaMu.Lock()
	fofaQuota, fofaQuotaFP, fofaQuotaAt = acc, fp, time.Now()
	fofaQuotaMu.Unlock()
	return acc
}

func fofaErrText(err error) string {
	if errors.Is(err, fofa.ErrNoKey) {
		return "FOFA 未配置 API Key"
	}
	var ae *fofa.APIError
	if errors.As(err, &ae) {
		if ae.IsKeyInvalid() {
			return "FOFA 账号无效（key 失效或与账号不匹配）：" + ae.Msg
		}
		if ae.IsQuota() {
			return "FOFA 查询额度不足：" + ae.Msg
		}
		if ae.Retryable() {
			return "FOFA 请求频率超限（已自动重试）：" + ae.Msg
		}
		if ae.IsNextForbidden() {
			return "当前账号未开通连续翻页接口（search/next），请改用 mode=page： " + ae.Msg
		}
		if ae.IsForbidden() {
			return "当前 FOFA 账号未开通该接口（stats/host 需要更高权限），请改用 fofa_search： " + ae.Msg
		}
		if ae.IsBadQuery() {
			return "FOFA 查询语法错误：" + ae.Msg
		}
		return "FOFA 请求失败：" + ae.Msg
	}
	return "FOFA 请求失败：" + err.Error()
}

func splitFields(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func mdCell(v any) string {
	s := fmt.Sprint(v)
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

// fofaSearch searches the FOFA index. Page mode is for normal lookups; next
// mode is the cursor API for large pulls (max 10000 rows per page).
func (t *ToolSet) fofaSearch() actool.CoreTool {
	return readTool(
		"fofa_search",
		"用 FOFA 网络空间测绘检索资产。query 使用 FOFA 语法（如 `app=\"Apache\" && country=\"CN\"`、"+
			"`title=\"Index of /\"`、`domain=\"example.com\"`）。返回 markdown 表格 + 本页行数 + 翻页游标 + 剩余额度。\n"+
			"注意：search 接口不返回命中总数；判断规模要用 fofa_stats（若返回 -403 则账号无该接口权限，改为小 size 试拉）。\n"+
			"使用建议：① size 默认 50，单页最大 10000；② 结果超过一页用 mode=next + 返回的 next 继续（比 page 深翻更稳）；"+
			"③ 拉到的 IP/服务用 insert_assets 写回资产图；④ 查询按返回行数消耗数据额度，不要用无意义的大 size 空转。",
		obj(map[string]any{
			"query": str("FOFA 查询语法（必填）。示例：app=\"Apache\" && country=\"CN\"；title=\"Index of /\"；domain=\"example.com\" && port=\"443\""),
			"fields": map[string]any{
				"type":        "string",
				"description": "逗号分隔的返回字段。常用：host,ip,port,title,domain,server,protocol,icp,country,province,city,asn,org,link,header,banner,body,cert",
				"default":     fofaDefaultFields,
			},
			"size": map[string]any{
				"type": "integer", "description": "单页条数（默认 50，最大 10000）。每条消耗 1 个数据量额度。",
				"default": 50, "minimum": 1, "maximum": 10000,
			},
			"page": map[string]any{"type": "integer", "description": "页码（mode=page 时有效，默认 1）", "default": 1, "minimum": 1},
			"mode": map[string]any{
				"type": "string", "enum": []string{"page", "next"},
				"description": "page=普通分页（search/all）；next=连续翻页（search/next，适合大批量）", "default": "page",
			},
			"next": map[string]any{"type": "string", "description": "mode=next 时的游标，首次留空；后续填上次返回的 next"},
			"full": map[string]any{"type": "boolean", "description": "mode=next 时是否检索全部历史数据（默认 false=近一年）", "default": false},
		}, "query"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				Query  string `json:"query"`
				Fields string `json:"fields"`
				Size   int    `json:"size"`
				Page   int    `json:"page"`
				Mode   string `json:"mode"`
				Next   string `json:"next"`
				Full   bool   `json:"full"`
			}
			_ = json.Unmarshal(in, &a)
			if strings.TrimSpace(a.Query) == "" {
				return actool.Errorf("query 不能为空"), nil
			}
			if a.Fields == "" {
				a.Fields = fofaDefaultFields
			}
			if a.Size <= 0 {
				a.Size = 50
			}
			if a.Mode == "" {
				a.Mode = "page"
			}
			c, err := fofaClientFor()
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			fields := splitFields(a.Fields)

			var res *fofa.SearchResult
			if a.Mode == "next" {
				res, err = c.SearchNext(ctx, a.Query, fields, a.Size, a.Next, a.Full)
			} else {
				res, err = c.Search(ctx, a.Query, fields, a.Page, a.Size)
			}
			if err != nil {
				return actool.Errorf(fofaErrText(err)), nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "mode=%s page=%d returned=%d", a.Mode, res.Page, len(res.Results))
			if res.ConsumedFpoint > 0 {
				fmt.Fprintf(&b, " consumed_fpoint=%g", res.ConsumedFpoint)
			}
			if id := res.NextID(); id != "" {
				fmt.Fprintf(&b, " next=%s", id)
			}
			if acc := fofaQuotaSnapshot(ctx, c); acc != nil {
				fmt.Fprintf(&b, " quota_left=%d/%d", acc.RemainAPIQuery, acc.RemainAPIData)
				if acc.RemainAPIQuery < fofaWarnQuota {
					fmt.Fprintf(&b, " ⚠️额度不足%d次", acc.RemainAPIQuery)
				}
			}
			b.WriteString("\n\n")
			if len(res.Results) == 0 {
				b.WriteString("（无结果）\n")
			} else {
				b.WriteString("| " + strings.Join(fields, " | ") + " |\n")
				b.WriteString("|" + strings.Repeat(" --- |", len(fields)) + "\n")
				for _, row := range res.Results {
					cells := make([]string, len(fields))
					for i := range fields {
						if i < len(row) {
							cells[i] = mdCell(row[i])
						}
					}
					b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
				}
			}
			return actool.Text(b.String()), nil
		},
	)
}

// fofaStats aggregates a query without pulling rows — the recommended first
// step to size a result set.
func (t *ToolSet) fofaStats() actool.CoreTool {
	return readTool(
		"fofa_stats",
		"对 FOFA 查询做聚合统计（不拉明细，适合先摸清结果规模与分布）。返回 total 去重计数 + 指定字段的 TopN 分布。\n"+
			"注意：部分账号/套餐未开通该接口，会返回 `-403 访问权限不足`；此时改用 fofa_search 小 size 试拉。",
		obj(map[string]any{
			"query":  str("FOFA 查询语法（必填）"),
			"fields": map[string]any{"type": "string", "description": "聚合字段，逗号分隔（如 port,title,domain,protocol,country）", "default": "port,title,domain"},
			"size":   map[string]any{"type": "integer", "description": "每个字段返回的 TopN 数量（默认 5）", "default": 5, "minimum": 1, "maximum": 50},
		}, "query"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				Query  string `json:"query"`
				Fields string `json:"fields"`
				Size   int    `json:"size"`
			}
			_ = json.Unmarshal(in, &a)
			if strings.TrimSpace(a.Query) == "" {
				return actool.Errorf("query 不能为空"), nil
			}
			if a.Fields == "" {
				a.Fields = "port,title,domain"
			}
			if a.Size <= 0 {
				a.Size = 5
			}
			c, err := fofaClientFor()
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			res, err := c.Stats(ctx, a.Query, splitFields(a.Fields), a.Size)
			if err != nil {
				return actool.Errorf(fofaErrText(err)), nil
			}

			var b strings.Builder
			b.WriteString("## 去重计数\n")
			if len(res.Distinct) == 0 {
				b.WriteString("（无）\n")
			} else {
				keys := make([]string, 0, len(res.Distinct))
				for k := range res.Distinct {
					keys = append(keys, k)
				}
				sortStrings(keys)
				for _, k := range keys {
					fmt.Fprintf(&b, "- %s: %d\n", k, res.Distinct[k])
				}
			}
			for _, field := range splitFields(a.Fields) {
				items := res.Aggs[field]
				if len(items) == 0 {
					continue
				}
				fmt.Fprintf(&b, "\n## %s Top%d\n", field, len(items))
				for _, it := range items {
					fmt.Fprintf(&b, "- %s: %d\n", mdCell(it.Name), it.Count)
				}
			}
			if res.LastUpdateTime != "" {
				fmt.Fprintf(&b, "\nlast_update=%s\n", res.LastUpdateTime)
			}
			return actool.Text(b.String()), nil
		},
	)
}

// fofaHost returns the host aggregation (ports, protocols, products, ASN…).
func (t *ToolSet) fofaHost() actool.CoreTool {
	return readTool(
		"fofa_host",
		"查询单个 IP 或域名的 FOFA 聚合信息（ASN/ORG/开放端口/协议/产品/分类/更新时间）。"+
			"用于在扫描单个目标前快速了解它的服务面。\n"+
			"注意：部分账号/套餐未开通该接口，会返回 `-403 访问权限不足`；此时改用 fofa_search 的 `ip=\"...\"` 查询。",
		obj(map[string]any{
			"host":   str("IP 或域名（必填）"),
			"detail": map[string]any{"type": "boolean", "description": "是否返回详细字段（默认 true）", "default": true},
		}, "host"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				Host   string `json:"host"`
				Detail *bool  `json:"detail"`
			}
			_ = json.Unmarshal(in, &a)
			if strings.TrimSpace(a.Host) == "" {
				return actool.Errorf("host 不能为空"), nil
			}
			detail := true
			if a.Detail != nil {
				detail = *a.Detail
			}
			c, err := fofaClientFor()
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			res, err := c.Host(ctx, a.Host, detail)
			if err != nil {
				return actool.Errorf(fofaErrText(err)), nil
			}
			return jsonResult(res)
		},
	)
}

// fofaAccount reports the account profile and remaining quota (0 query cost).
func (t *ToolSet) fofaAccount() actool.CoreTool {
	return readTool(
		"fofa_account",
		"查看当前 FOFA 账号信息与剩余额度（不消耗查询次数）。在开始批量资产检索前先确认额度是否充足。",
		obj(map[string]any{}),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			c, err := fofaClientFor()
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			acc, err := c.Account(ctx)
			if err != nil {
				return actool.Errorf(fofaErrText(err)), nil
			}
			return jsonResult(map[string]any{
				"username":          acc.Username,
				"email":             acc.Email,
				"isvip":             acc.IsVIP,
				"vip_level":         acc.VIPLevel,
				"remain_api_query":  acc.RemainAPIQuery,
				"remain_api_data":   acc.RemainAPIData,
				"remain_free_point": acc.RemainFreePoint,
				"fofa_point":        acc.FofaPoint,
			})
		},
	)
}

// sortStrings is a tiny wrapper kept for stable ordering of distinct keys.
func sortStrings(ss []string) { sort.Strings(ss) }

// helper used by tests to reset the cached client between cases.
func resetFofaCacheForTest() {
	fofaClientMu.Lock()
	fofaClient, fofaClientFP, fofaClientBase, fofaClientRate = nil, "", "", 0
	fofaClientMu.Unlock()
	fofaQuotaMu.Lock()
	fofaQuota, fofaQuotaFP = nil, ""
	fofaQuotaMu.Unlock()
}
