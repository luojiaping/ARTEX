package credhunt

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name, kind, value, service, context, source string
		wantProvider, wantBase                      string
	}{
		{"deepseek", KindLLMKey, "sk-abcdef123456", "DeepSeek 余额", "", "https://example.com/.env", "deepseek", ""},
		{"openrouter-prefix", "api_key", "sk-or-v1-abcdef", "gateway", "", "https://x.test/config", "openrouter", ""},
		{"claude-oauth", KindOAuth, `{"access_token":"x"}`, "Claude Code", "oauth", "https://x.test/auth.json", "claude_oauth", ""},
		{"relay-domain", KindLLMKey, "sk-unknown123456", "中转站", "api gateway", "https://api.relay-example.cn/v1", "relay", "https://api.relay-example.cn"},
		{"service-wins", KindLLMKey, "sk-dashscope123456", "Alibaba DashScope (Model Studio)", "Kimi agent 环境", "https://x.test/.env", "dashscope", ""},
		{"entry-domain", KindLLMKey, "sk-relay123456", "LongCat (Anthropic 兼容网关)", "ANTHROPIC_BASE_URL=https://api.longcat.chat/anthropic", "https://x.test/.env", "relay", "https://api.longcat.chat/anthropic"},
		{"im-service", "", "123456:ABCDEF", "Telegram Bot", "", "https://x.test/bot", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, base := Classify(tc.kind, tc.value, tc.service, tc.context, tc.source)
			if got != tc.wantProvider || base != tc.wantBase {
				t.Fatalf("Classify() = (%q,%q), want (%q,%q)", got, base, tc.wantProvider, tc.wantBase)
			}
		})
	}
}

func TestMagpiePresetCoverage(t *testing.T) {
	t.Logf("provider registry has %d entries", len(providers))
	for _, id := range []string{"openai", "mistral", "together", "fireworks", "modelscope", "aihubmix",
		"302ai", "cherryin", "commandcode", "clinepass", "kilo", "baidu", "tencent", "huawei", "ollama"} {
		if _, ok := ProviderByID(id); !ok {
			t.Errorf("provider %q was not merged from magpie presets", id)
		}
	}
	// Existing ids stay canonical.
	for _, id := range []string{"deepseek", "kimi", "glm", "dashscope", "ark", "mimo", "opencode", "lkeap"} {
		if _, ok := ProviderByID(id); !ok {
			t.Errorf("canonical provider %q disappeared", id)
		}
	}
}

func TestClassifyMagpieHosts(t *testing.T) {
	cases := []struct {
		name, kind, service, context string
		wantProvider, wantVariant    string
	}{
		{"tencent-tokenhub", KindLLMKey, "Tencent TokenHub", "BASE_URL=https://tokenhub.tencentmaas.com/v1", "tencent", "cn"},
		{"aihubmix", KindLLMKey, "AiHubMix", "OPENAI_BASE_URL=https://aihubmix.com/v1", "aihubmix", ""},
		{"kimi-code", KindLLMKey, "Kimi Code", "ANTHROPIC_BASE_URL=https://api.kimi.com/coding", "kimi", "coding"},
		{"mistral", KindLLMKey, "Mistral", "MISTRAL_BASE_URL=https://api.mistral.ai/v1", "mistral", ""},
		{"zai-coding", KindLLMKey, "Z.ai Coding", "OPENAI_BASE_URL=https://api.z.ai/api/coding/paas/v4", "glm", "coding_zai"},
		{"zai-coding-anthropic", KindLLMKey, "Z.ai Coding", "ANTHROPIC_BASE_URL=https://api.z.ai/api/anthropic", "glm", "coding_zai"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := ClassifyDetailed(tc.kind, "sk-1234567890", tc.service, tc.context, "https://x.test/.env")
			if c.ProviderID != tc.wantProvider || c.VariantID != tc.wantVariant {
				t.Fatalf("classification = %+v, want %s/%s", c, tc.wantProvider, tc.wantVariant)
			}
		})
	}
}

func TestClassifyRelayAnthropicBearer(t *testing.T) {
	c := ClassifyDetailed(KindLLMKey, "ak-test", "LongCat (Anthropic 兼容网关)",
		"ANTHROPIC_AUTH_TOKEN=ak-test\nANTHROPIC_BASE_URL=https://api.longcat.chat/anthropic；Authorization: Bearer",
		"https://x.test/.env")
	if c.ProviderID != "relay" || c.Protocol != ProtoAnthropic || c.AuthHeader != "bearer" {
		t.Fatalf("classification = %+v", c)
	}
	if len(c.Endpoints) != 1 || c.Endpoints[0].Base != "https://api.longcat.chat/anthropic" {
		t.Fatalf("endpoints = %#v", c.Endpoints)
	}
}

func TestLooksLikePlaceholder(t *testing.T) {
	for _, v := range []string{"", "xxxxxx", "your-api-key", "changeme", "<token>", "${KEY}", "aaaaaaa"} {
		if !LooksLikePlaceholder(v) {
			t.Errorf("%q should be a placeholder", v)
		}
	}
	for _, v := range []string{"sk-1234567890abcdef", "ghp_abcdefghijklmnop", "uuid:pass"} {
		if LooksLikePlaceholder(v) {
			t.Errorf("%q should be accepted", v)
		}
	}
}

func TestIsIMService(t *testing.T) {
	if !IsIMService("Telegram Bot") || !IsIMService("飞书应用") {
		t.Fatal("IM services must be excluded")
	}
	if IsIMService("OpenRouter") {
		t.Fatal("non-IM service misclassified")
	}
}
