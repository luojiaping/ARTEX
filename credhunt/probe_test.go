package credhunt

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testVerifier() *Verifier {
	return &Verifier{client: &http.Client{Timeout: 5 * time.Second}, modelCache: map[string]modelCacheEntry{}}
}

func TestProbeChatOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"valid", 200, `{"choices":[{"message":{"content":"hi"}}]}`, "valid"},
		{"invalid", 401, `{"error":{"message":"invalid api key","type":"invalid_api_key"}}`, "invalid"},
		{"no-balance", 402, `{"error":{"message":"payment required"}}`, "no_balance"},
		{"rate", 429, `{"error":{"message":"rate limit exceeded","type":"rate_limit_error"}}`, "rate_limited"},
		{"endpoint", 404, `{"error":{"message":"not found"}}`, "endpoint_missing"},
		{"model", 400, `{"error":{"message":"model gpt-4o-mini does not exist","type":"invalid_request_error"}}`, "model_missing"},
		{"zhipu-envelope", 200, `{"success":false,"code":1000,"msg":"身份验证失败"}`, "invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
					t.Errorf("authorization = %q", got)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			out := testVerifier().probeOnce(context.Background(), ProbeTarget{
				ProviderID: "test", Protocol: ProtoChat, Base: srv.URL, Models: []string{"gpt-4o-mini"},
			}, "sk-test", "gpt-4o-mini", true)
			if out.Kind != tc.want {
				t.Fatalf("kind = %q, want %q (detail %q)", out.Kind, tc.want, out.Detail)
			}
		})
	}
}

func TestProbeAnthropicRetriesOneAuth(t *testing.T) {
	var sawBoth, sawSingle bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %s", r.URL.Path)
		}
		x, a := r.Header.Get("x-api-key"), r.Header.Get("Authorization")
		switch {
		case x != "" && a != "":
			sawBoth = true
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"x-api-key and Authorization cannot both be sent"}}`))
		case x != "":
			sawSingle = true
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"hi"}]}`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"missing key"}}`))
		}
	}))
	defer srv.Close()
	out := testVerifier().probeOnce(context.Background(), ProbeTarget{
		ProviderID: "relay", Protocol: ProtoAnthropic, Base: srv.URL, Models: []string{"claude-3-5-haiku-latest"},
	}, "sk-ant-test", "claude-3-5-haiku-latest", true)
	if out.Kind != "valid" || !sawBoth || !sawSingle {
		t.Fatalf("out=%+v both=%v single=%v", out, sawBoth, sawSingle)
	}
}

func TestProbeGeminiUsesGoogleHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "AIza-test" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"missing key"}}`))
			return
		}
		if !strings.Contains(r.URL.Path, ":generateContent") {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]}`))
	}))
	defer srv.Close()
	out := testVerifier().probeOnce(context.Background(), ProbeTarget{
		ProviderID: "gemini", Protocol: ProtoGemini, Base: srv.URL + "/v1beta", Models: []string{"gemini-2.0-flash"},
	}, "AIza-test", "gemini-2.0-flash", true)
	if out.Kind != "valid" {
		t.Fatalf("kind=%q detail=%q", out.Kind, out.Detail)
	}
}

func TestProbeStreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"error\",\"error\":{\"message\":\"quota exceeded\"}}\n\n"))
	}))
	defer srv.Close()
	out := testVerifier().probeOnce(context.Background(), ProbeTarget{
		ProviderID: "relay", Protocol: ProtoChat, Base: srv.URL, Models: []string{"gpt-4o-mini"},
	}, "sk-test", "gpt-4o-mini", true)
	if out.Kind != "unknown" {
		t.Fatalf("kind=%q detail=%q", out.Kind, out.Detail)
	}
}

func TestDiscoverModelsFallsBackToV1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"deepseek-chat"},{"id":"text-embedding-3-small"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	models := testVerifier().fetchModels(context.Background(), ProbeTarget{
		ProviderID: "deepseek", Protocol: ProtoChat, Base: srv.URL,
	}, "sk-test")
	if len(models) != 1 || models[0] != "deepseek-chat" {
		t.Fatalf("models = %v", models)
	}
}

func TestModelsForDiscoversBeforeFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"discovered-model"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	models := testVerifier().modelsFor(context.Background(), ProbeTarget{
		ProviderID: "relay", Protocol: ProtoChat, Base: srv.URL, FallbackModel: "fallback-model",
	}, "sk-test")
	if len(models) < 2 || models[0] != "discovered-model" || models[1] != "fallback-model" {
		t.Fatalf("models = %v", models)
	}
}

func TestClassifyByPresetHostWinsOverKeyword(t *testing.T) {
	c := ClassifyDetailed(KindLLMKey, "sk-1234567890", "Kimi agent 环境",
		"DASHSCOPE_API_KEY=sk-123\nBASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1", "https://x.test/.env")
	if c.ProviderID != "dashscope" {
		t.Fatalf("provider = %q (%+v)", c.ProviderID, c)
	}
	if c.Protocol != ProtoChat || !strings.Contains(c.Base, "dashscope.aliyuncs.com") {
		t.Fatalf("endpoint = %q %q", c.Protocol, c.Base)
	}
}

func TestClassifyByPresetCodingPath(t *testing.T) {
	c := ClassifyDetailed(KindLLMKey, "sk-1234567890", "GLM Coding",
		"ANTHROPIC_BASE_URL=https://open.bigmodel.cn/api/coding/paas/v4", "https://x.test/.env")
	if c.ProviderID != "glm" || c.VariantID != "coding" {
		t.Fatalf("classification = %+v", c)
	}
	if c.Base != "https://open.bigmodel.cn/api/coding/paas/v4" {
		t.Fatalf("base = %q", c.Base)
	}
}

func TestParseQuotaBalanceShapes(t *testing.T) {
	q := parseQuotaBody([]byte(`{"data":{"balance_infos":[{"currency":"CNY","total_balance":"12.34"}]}}`))
	if q["summary"] != "12.34 CNY" {
		t.Fatalf("summary = %v", q["summary"])
	}
	q = parseQuotaBody([]byte(`{"usage":{"rolling":{"percent":37,"resetsAt":"2026-08-26T14:12:03Z"},"weekly":{"percent":80}}}`))
	ws, _ := q["windows"].([]map[string]any)
	if len(ws) != 2 {
		t.Fatalf("windows = %#v", q["windows"])
	}
	if ws[0]["name"] != "5h" || ws[0]["used"].(float64) != 37 {
		t.Fatalf("window = %#v", ws[0])
	}
}

func TestProbeTargetsUseClassificationEndpoints(t *testing.T) {
	e := Entry{
		Kind: KindLLMKey, Provider: "glm", Variant: "coding",
		Classification: map[string]any{
			"endpoints": []any{
				map[string]any{"protocol": "anthropic", "base": "https://open.bigmodel.cn/api/anthropic"},
				map[string]any{"protocol": "chat", "base": "https://open.bigmodel.cn/api/coding/paas/v4"},
			},
			"models": []any{"glm-4.5"},
		},
	}
	p, ok := ProviderByID("glm")
	if !ok {
		t.Fatal("glm provider missing")
	}
	v := &Verifier{}
	targets := v.probeTargets(e, p)
	if len(targets) != 2 {
		t.Fatalf("targets = %#v", targets)
	}
	if targets[0].Protocol != ProtoAnthropic {
		t.Fatalf("first target = %#v", targets[0])
	}
	if len(targets[0].Models) != 1 || targets[0].Models[0] != "glm-4.5" {
		t.Fatalf("models = %#v", targets[0].Models)
	}
}

func TestVerifyOneUsesBaseOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/models") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi"}}]}`))
	}))
	defer srv.Close()
	v := testVerifier()
	e := Entry{UID: "crd_test", Kind: KindLLMKey, Provider: "relay", Service: "test relay",
		Extra: map[string]any{"base_url": srv.URL + "/v1"}}
	res := v.verifyOne(context.Background(), e, "sk-test")
	if res.Status != StatusValid {
		t.Fatalf("status = %q detail = %q", res.Status, res.Detail)
	}
	if res.Protocol != ProtoChat || res.Model == "" {
		t.Fatalf("result = %+v", res)
	}
}

func TestProbeJSONErrorEnvelopeNotValid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"message":"account suspended"}}`))
	}))
	defer srv.Close()
	out := testVerifier().probeOnce(context.Background(), ProbeTarget{
		ProviderID: "relay", Protocol: ProtoChat, Base: srv.URL, Models: []string{"gpt-4o-mini"},
	}, "sk-test", "gpt-4o-mini", true)
	if out.Kind == "valid" {
		b, _ := json.Marshal(out)
		t.Fatalf("accepted an error envelope: %s", b)
	}
}
