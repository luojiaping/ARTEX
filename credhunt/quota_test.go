package credhunt

import (
	"testing"
)

func TestQuotaEndpoints(t *testing.T) {
	cases := []struct {
		kind string
		t    ProbeTarget
		want string
	}{
		{"deepseek_balance", ProbeTarget{Base: "https://api.deepseek.com/v1"}, "https://api.deepseek.com/user/balance"},
		{"openrouter_key", ProbeTarget{Base: "https://openrouter.ai/api/v1"}, "https://openrouter.ai/api/v1/credits"},
		{"zhipu_coding", ProbeTarget{Base: "https://open.bigmodel.cn/api/coding/paas/v4"}, "https://open.bigmodel.cn/api/monitor/usage/quota/limit"},
		{"zhipu_coding", ProbeTarget{Base: "https://api.z.ai/api/coding/paas/v4"}, "https://api.z.ai/api/monitor/usage/quota/limit"},
		{"kimi_coding", ProbeTarget{Base: "https://api.kimi.com/coding"}, "https://api.kimi.com/coding/v1/usages"},
		{"minimax_coding", ProbeTarget{Base: "https://api.minimaxi.com/v1"}, "https://api.minimaxi.com/v1/token_plan/remains"},
		{"stepfun_balance", ProbeTarget{Base: "https://api.stepfun.com/v1"}, "https://api.stepfun.com/v1/accounts"},
		{"aihubmix_balance", ProbeTarget{Base: "https://aihubmix.com/v1"}, "https://aihubmix.com/dashboard/billing/remain"},
	}
	for _, tc := range cases {
		got := quotaEndpoints(tc.kind, tc.t)
		if len(got) == 0 || got[0] != tc.want {
			t.Errorf("%s %s: got %v, want first %s", tc.kind, tc.t.Base, got, tc.want)
		}
	}
}

func TestParseQuotaZhipu(t *testing.T) {
	q := parseQuotaBody([]byte(`{"code":200,"success":true,"data":{"level":"pro","limits":[
		{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":12,"nextResetTime":1790000000000},
		{"type":"TOKENS_LIMIT","unit":6,"number":1,"percentage":40,"nextResetTime":1790500000000}]}}`))
	ws, _ := q["windows"].([]map[string]any)
	if len(ws) != 2 || ws[0]["name"] != "5h" || ws[0]["used"].(float64) != 12 {
		t.Fatalf("windows = %#v", ws)
	}
	if q["plan"] != "pro" {
		t.Fatalf("plan = %v", q["plan"])
	}
}

func TestParseQuotaKimiAndMiniMax(t *testing.T) {
	q := parseQuotaBody([]byte(`{"limits":[{"window":{"type":"5h","duration":5,"timeUnit":"HOUR"},"detail":{"limit":100,"used":25,"resetTime":"2026-10-25T21:00:00Z"}}]}`))
	ws, _ := q["windows"].([]map[string]any)
	if len(ws) != 1 || ws[0]["name"] != "5h" || ws[0]["used"].(float64) != 25 {
		t.Fatalf("kimi windows = %#v", ws)
	}
	q = parseQuotaBody([]byte(`{"model_remains":[{"model_name":"general","current_interval_remaining_percent":80,"current_interval_total_count":100,"current_interval_usage_count":20}]}`))
	ws, _ = q["windows"].([]map[string]any)
	if len(ws) != 1 || ws[0]["used"].(float64) != 20 {
		t.Fatalf("minimax windows = %#v", ws)
	}
}
