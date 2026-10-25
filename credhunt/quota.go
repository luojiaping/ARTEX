package credhunt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// fetchQuotaForTarget asks the provider's balance or plan endpoint after a
// successful probe. A quota failure never changes the probe verdict.
func (v *Verifier) fetchQuotaForTarget(ctx context.Context, t ProbeTarget, key string) map[string]any {
	kind := strings.TrimSpace(t.QuotaKind)
	if kind == "" || kind == "none" {
		kind = presetQuota(strings.TrimSpace(t.ProviderID))
	}
	endpoints := quotaEndpoints(kind, t)
	if len(endpoints) == 0 {
		return nil
	}
	var lastErr string
	for _, endpoint := range endpoints {
		out, err := v.quotaRequest(ctx, endpoint, t, key)
		if err == nil && out != nil {
			out["kind"] = quotaKindName(kind)
			out["source"] = endpoint
			out["quota_kind"] = kind
			return out
		}
		if err != nil {
			lastErr = err.Error()
		}
	}
	if lastErr != "" {
		return map[string]any{"kind": quotaKindName(kind), "quota_kind": kind, "error": lastErr,
			"summary": "额度查询失败：" + lastErr}
	}
	return nil
}

func quotaKindName(kind string) string {
	switch kind {
	case "deepseek_balance", "moonshot_balance", "siliconflow_user", "openrouter_key",
		"stepfun_balance", "aihubmix_balance", "commandcode_balance", "ollama_balance":
		return "balance"
	case "zhipu_coding", "kimi_coding", "minimax_coding", "opencode_go_usage":
		return "plan"
	default:
		return "quota"
	}
}

func quotaEndpoints(kind string, t ProbeTarget) []string {
	base := strings.TrimRight(strings.TrimSpace(t.Base), "/")
	root := baseRoot(base)
	host := ""
	if u, err := url.Parse(base); err == nil {
		host = strings.ToLower(u.Hostname())
	}
	switch kind {
	case "deepseek_balance":
		return []string{root + "/user/balance"}
	case "moonshot_balance":
		if strings.Contains(host, "moonshot.ai") {
			return []string{"https://api.moonshot.ai/v1/users/me/balance", "https://api.moonshot.cn/v1/users/me/balance"}
		}
		return []string{"https://api.moonshot.cn/v1/users/me/balance", "https://api.moonshot.ai/v1/users/me/balance"}
	case "openrouter_key":
		return []string{"https://openrouter.ai/api/v1/credits"}
	case "siliconflow_user":
		if strings.Contains(host, "siliconflow.com") {
			return []string{"https://api.siliconflow.com/v1/user/info", "https://api.siliconflow.cn/v1/user/info"}
		}
		return []string{"https://api.siliconflow.cn/v1/user/info", "https://api.siliconflow.com/v1/user/info"}
	case "stepfun_balance":
		if strings.Contains(host, "stepfun.ai") {
			return []string{"https://api.stepfun.ai/v1/accounts", "https://api.stepfun.com/v1/accounts"}
		}
		return []string{"https://api.stepfun.com/v1/accounts", "https://api.stepfun.ai/v1/accounts"}
	case "aihubmix_balance":
		return []string{"https://aihubmix.com/dashboard/billing/remain"}
	case "commandcode_balance":
		return []string{"https://api.commandcode.ai/alpha/billing/credits"}
	case "ollama_balance":
		return []string{"https://ollama.com/api/balance"}
	case "zhipu_coding":
		if strings.Contains(host, "z.ai") {
			return []string{"https://api.z.ai/api/monitor/usage/quota/limit"}
		}
		return []string{"https://open.bigmodel.cn/api/monitor/usage/quota/limit"}
	case "kimi_coding":
		if strings.Contains(base, "api.kimi.ai") {
			return []string{"https://api.kimi.ai/coding/v1/usages", "https://api.kimi.com/coding/v1/usages"}
		}
		return []string{"https://api.kimi.com/coding/v1/usages", "https://api.kimi.ai/coding/v1/usages"}
	case "minimax_coding":
		if strings.Contains(host, "minimax.io") {
			return []string{"https://api.minimax.io/v1/token_plan/remains", "https://api.minimaxi.com/v1/token_plan/remains"}
		}
		return []string{"https://api.minimaxi.com/v1/token_plan/remains", "https://api.minimax.io/v1/token_plan/remains"}
	case "opencode_go_usage":
		return []string{"https://opencode.ai/zen/go/usage"}
	}
	return nil
}

func baseRoot(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	for _, suffix := range []string{"/v1", "/v1beta", "/compatible-mode/v1", "/api/paas/v4", "/api/coding/paas/v4",
		"/api/v3", "/api/coding/v3", "/api/plan/v3", "/coding/v1", "/coding", "/plan/v3"} {
		if strings.HasSuffix(base, suffix) {
			return strings.TrimSuffix(base, suffix)
		}
	}
	return base
}

func (v *Verifier) quotaRequest(ctx context.Context, endpoint string, t ProbeTarget, key string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ARTEX-CredHunter/2.0")
	switch {
	case strings.Contains(endpoint, "/api/monitor/usage/quota/limit"):
		req.Header.Set("Authorization", "Bearer "+key)
	case strings.Contains(endpoint, "/token_plan/remains"):
		req.Header.Set("Authorization", "Bearer "+key)
	default:
		req.Header.Set("Authorization", "Bearer "+key)
	}
	// Kimi Code's usage endpoint is asked with the coding key as a Bearer.
	if strings.Contains(endpoint, "/coding/v1/usages") {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%d %s", resp.StatusCode, truncate(string(body), 200))
	}
	return parseQuotaBody(body), nil
}

func parseQuotaBody(body []byte) map[string]any {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return map[string]any{"summary": "额度接口返回非 JSON", "raw": truncate(string(body), 2000)}
	}
	out := map[string]any{"raw": truncate(string(body), 4000)}
	if success, ok := m["success"].(bool); ok && !success {
		msg := firstMapString(m, "msg", "message")
		if msg == "" {
			msg = "厂商拒绝了额度查询"
		}
		out["error"] = msg
		out["summary"] = "额度查询失败：" + msg
		return out
	}
	if code, ok := numberValue(m["code"]); ok && code >= 400 {
		msg := firstMapString(m, "msg", "message")
		if msg == "" {
			msg = fmt.Sprintf("code %v", m["code"])
		}
		out["error"] = msg
		out["summary"] = "额度查询失败：" + msg
		return out
	}

	// Balance shapes.
	if data, ok := m["data"].(map[string]any); ok {
		if infos, ok := data["balance_infos"].([]any); ok && len(infos) > 0 {
			parts := make([]string, 0, len(infos))
			for _, one := range infos {
				if row, ok := one.(map[string]any); ok {
					cur := asString(row["currency"])
					bal := asString(row["total_balance"])
					if bal == "" {
						bal = fmt.Sprint(row["total_balance"])
					}
					parts = append(parts, strings.TrimSpace(bal+" "+cur))
					out["balance"] = row["total_balance"]
					out["currency"] = cur
				}
			}
			out["summary"] = strings.Join(parts, " · ")
		}
		if v, ok := data["available_balance"]; ok {
			out["balance"] = v
			out["summary"] = moneyText(v, "¥")
		}
		if v, ok := data["totalBalance"]; ok {
			out["balance"] = v
			out["summary"] = moneyText(v, "¥")
		}
		if v, ok := data["total_credits"]; ok {
			out["credits"] = v
			used, _ := numberValue(data["total_usage"])
			total, _ := numberValue(v)
			out["balance"] = total - used
			out["summary"] = fmt.Sprintf("$%.2f / $%.2f", total-used, total)
		}
		if v, ok := data["quota"]; ok {
			if n, ok := numberValue(v); ok {
				out["balance"] = n / 500000
				out["summary"] = fmt.Sprintf("$%.2f", n/500000)
			}
		}
		if plan := asString(data["level"]); plan != "" {
			out["plan"] = plan
		}
		if limits, ok := data["limits"].([]any); ok {
			if ws := windowsFromLimits(limits); len(ws) > 0 {
				out["windows"] = ws
			}
		}
	}
	// Flat balance shapes.
	for _, k := range []string{"balance", "totalBalance", "total_usage", "total_available"} {
		if v, ok := m[k]; ok {
			if n, ok := numberValue(v); ok {
				out["balance"] = n
				switch k {
				case "total_usage":
					out["summary"] = fmt.Sprintf("$%.2f", n)
				default:
					out["summary"] = moneyText(n, "$")
				}
			}
		}
	}
	// OpenCode Go / rolling windows.
	if usage, ok := m["usage"].(map[string]any); ok {
		ws := windowsFromUsage(usage)
		if len(ws) > 0 {
			out["windows"] = ws
			out["summary"] = windowsSummary(ws)
		}
	}
	// MiniMax plan.
	if rows, ok := m["model_remains"].([]any); ok {
		if ws := windowsFromMiniMax(rows); len(ws) > 0 {
			out["windows"] = ws
			out["summary"] = windowsSummary(ws)
		}
	}
	// Kimi Code limits.
	if rows, ok := m["limits"].([]any); ok {
		if ws := windowsFromKimi(rows); len(ws) > 0 {
			out["windows"] = ws
			out["summary"] = windowsSummary(ws)
		}
	}
	if out["summary"] == nil {
		out["summary"] = "已读取额度"
	}
	return out
}

func windowsFromLimits(rows []any) []map[string]any {
	var out []map[string]any
	for _, row := range rows {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		name := asString(m["type"])
		if strings.EqualFold(name, "TIME_LIMIT") {
			name = "MCP · Month"
		} else if unit, _ := numberValue(m["unit"]); unit == 3 {
			n, _ := numberValue(m["number"])
			if n <= 0 {
				n = 5
			}
			name = fmt.Sprintf("%.0fh", n)
		} else if unit == 6 {
			name = "Week"
		}
		used, _ := numberValue(m["percentage"])
		w := map[string]any{"name": name, "used": used, "remaining": maxFloat(0, 100-used)}
		if ts, _ := numberValue(m["nextResetTime"]); ts > 0 {
			t := time.UnixMilli(int64(ts))
			w["resets_at"] = t.Format(time.RFC3339)
		}
		out = append(out, w)
	}
	return out
}

func windowsFromUsage(usage map[string]any) []map[string]any {
	var out []map[string]any
	for _, x := range []struct {
		key, name string
	}{{"rolling", "5h"}, {"weekly", "Week"}, {"monthly", "Month"}} {
		row, ok := usage[x.key].(map[string]any)
		if !ok {
			continue
		}
		used, ok := numberValue(row["percent"])
		if !ok {
			continue
		}
		w := map[string]any{"name": x.name, "used": used, "remaining": maxFloat(0, 100-used)}
		if s := asString(row["resetsAt"]); s != "" {
			w["resets_at"] = s
		}
		out = append(out, w)
	}
	return out
}

func windowsFromMiniMax(rows []any) []map[string]any {
	var out []map[string]any
	for _, row := range rows {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		model := asString(m["model_name"])
		if model == "" {
			model = "plan"
		}
		for _, x := range []struct {
			left, total, used any
			name              string
		}{
			{m["current_interval_remaining_percent"], m["current_interval_total_count"], m["current_interval_usage_count"], model + " · 5h"},
			{m["current_weekly_remaining_percent"], m["current_weekly_total_count"], m["current_weekly_usage_count"], model + " · Week"},
		} {
			left, ok := numberValue(x.left)
			if !ok {
				continue
			}
			w := map[string]any{"name": x.name, "remaining": left, "used": 100 - left}
			if total, ok := numberValue(x.total); ok && total > 0 {
				used, _ := numberValue(x.used)
				w["used"] = maxFloat(0, minFloat(100, used/total*100))
				w["remaining"] = 100 - w["used"].(float64)
				w["display"] = fmt.Sprintf("%.0f / %.0f", total-used, total)
			}
			out = append(out, w)
		}
	}
	return out
}

func windowsFromKimi(rows []any) []map[string]any {
	var out []map[string]any
	for _, row := range rows {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		detail, _ := m["detail"].(map[string]any)
		if detail == nil {
			detail, _ = m["window"].(map[string]any)
		}
		limit, ok := numberValue(detail["limit"])
		if !ok || limit <= 0 {
			continue
		}
		used, ok := numberValue(detail["used"])
		if !ok {
			left, _ := numberValue(detail["remaining"])
			used = limit - left
		}
		win, _ := m["window"].(map[string]any)
		name := asString(win["type"])
		if name == "" {
			name = "Window"
		}
		w := map[string]any{"name": name, "used": maxFloat(0, minFloat(100, used/limit*100))}
		w["remaining"] = 100 - w["used"].(float64)
		w["display"] = fmt.Sprintf("%.0f / %.0f", used, limit)
		for _, k := range []string{"resetTime", "resetAt", "reset_at", "reset_time"} {
			if s := asString(detail[k]); s != "" {
				w["resets_at"] = s
				break
			}
		}
		out = append(out, w)
	}
	return out
}

func windowsSummary(ws []map[string]any) string {
	parts := make([]string, 0, len(ws))
	for _, w := range ws {
		name := asString(w["name"])
		used, _ := numberValue(w["used"])
		parts = append(parts, fmt.Sprintf("%s %.0f%%", name, used))
	}
	sort.Strings(parts)
	return strings.Join(parts, " · ")
}

func moneyText(v any, sign string) string {
	n, ok := numberValue(v)
	if !ok {
		return fmt.Sprint(v)
	}
	return fmt.Sprintf("%s%.2f", sign, n)
}

func numberValue(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		var f float64
		if _, err := fmt.Sscanf(strings.TrimSpace(x), "%f", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
