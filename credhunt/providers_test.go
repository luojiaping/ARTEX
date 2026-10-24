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
