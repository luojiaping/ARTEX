package credhunt

import (
	"net/url"
	"regexp"
	"strings"
)

// Provider describes one LLM vendor or relay class. Base is the default API
// root; BaseFallbacks are tried when the primary root returns 404.
type Provider struct {
	ID            string
	Name          string
	Class         string // key|oauth
	Style         string // openai|anthropic|gemini|oauth|relay
	Base          string
	BaseFallbacks []string
	Model         string
	Quota         string
	Headers       map[string]string
	Match         *regexp.Regexp
	Prefixes      []string
	Endpoint      string // oauth usage endpoint
}

var providers = []Provider{
	// ---- OAuth packages ----
	{ID: "codex_oauth", Name: "OpenAI Codex OAuth", Class: "oauth", Style: "oauth",
		Endpoint: "https://chatgpt.com/backend-api/wham/usage", Quota: "codex_wham",
		Match: regexp.MustCompile(`(?i)codex.*oauth|oauth.*codex|chatgpt.*oauth|oauth.*chatgpt|chatgpt\s*plus|wham`)},
	{ID: "claude_oauth", Name: "Claude Code OAuth", Class: "oauth", Style: "oauth",
		Endpoint: "https://api.anthropic.com/api/oauth/usage", Quota: "claude_usage",
		Prefixes: []string{"sk-ant-oat"},
		Match:    regexp.MustCompile(`(?i)claude.*oauth|oauth.*claude|anthropic.*oauth|oauth.*anthropic`)},
	{ID: "copilot", Name: "GitHub Copilot OAuth", Class: "oauth", Style: "oauth",
		Endpoint: "https://api.github.com/copilot_internal/user", Quota: "copilot_user",
		Prefixes: []string{"gho_", "ghu_"},
		Match:    regexp.MustCompile(`(?i)copilot`)},
	{ID: "kimi_oauth", Name: "Kimi Code OAuth", Class: "oauth", Style: "openai",
		Base: "https://api.kimi.com/coding/v1", BaseFallbacks: []string{"https://api.kimi.ai/coding/v1"},
		Match: regexp.MustCompile(`(?i)kimi.*oauth|oauth.*kimi|kimi\s*code\s*oauth`)},
	{ID: "google_oauth", Name: "Google OAuth", Class: "oauth", Style: "oauth",
		Match: regexp.MustCompile(`(?i)google.*(oauth|workspace)|workspace.*oauth`)},
	{ID: "nous_oauth", Name: "Nous Research OAuth", Class: "oauth", Style: "openai",
		Base:  "https://inference-api.nousresearch.com/v1",
		Match: regexp.MustCompile(`(?i)nous`)},
	// ---- API keys ----
	{ID: "mimo", Name: "Xiaomi MiMo", Class: "key", Style: "openai",
		Base: "https://api.xiaomimimo.com/v1", Model: "mimo-v2.5",
		Match: regexp.MustCompile(`(?i)xiaomi|\bmimo\b`)},
	{ID: "opencode", Name: "OpenCode Zen / Go", Class: "key", Style: "openai",
		Base: "https://opencode.ai/zen/v1", BaseFallbacks: []string{"https://opencode.ai/zen/go/v1"},
		Match: regexp.MustCompile(`(?i)opencode`)},
	{ID: "lkeap", Name: "腾讯混元 LKEAP", Class: "key", Style: "openai",
		Base: "https://api.lkeap.cloud.tencent.com/v1", Model: "hunyuan-turbos-latest",
		Match: regexp.MustCompile(`(?i)lkeap|hunyuan|混元`)},
	{ID: "openrouter", Name: "OpenRouter", Class: "key", Style: "openai",
		Base: "https://openrouter.ai/api/v1", Model: "meta-llama/llama-3.3-70b-instruct:free",
		Quota: "openrouter_key", Prefixes: []string{"sk-or-"},
		Headers: map[string]string{"HTTP-Referer": "https://artex.local", "X-Title": "ARTEX"},
		Match:   regexp.MustCompile(`(?i)openrouter`)},
	{ID: "deepseek", Name: "DeepSeek", Class: "key", Style: "openai",
		Base: "https://api.deepseek.com", Model: "deepseek-chat", Quota: "deepseek_balance",
		Match: regexp.MustCompile(`(?i)deepseek`)},
	{ID: "kimi", Name: "Kimi / Moonshot", Class: "key", Style: "openai",
		Base: "https://api.moonshot.cn/v1", BaseFallbacks: []string{"https://api.moonshot.ai/v1"},
		Model: "moonshot-v1-8k", Quota: "moonshot_balance",
		Match: regexp.MustCompile(`(?i)moonshot|kimi`)},
	{ID: "glm", Name: "智谱 GLM", Class: "key", Style: "openai",
		Base: "https://open.bigmodel.cn/api/paas/v4", Model: "glm-4-flash",
		Match: regexp.MustCompile(`(?i)glm|zhipu|智谱|bigmodel|z\.ai|glmcode`)},
	{ID: "dashscope", Name: "阿里百炼 DashScope", Class: "key", Style: "openai",
		Base: "https://dashscope.aliyuncs.com/compatible-mode/v1", Model: "qwen-turbo",
		Match: regexp.MustCompile(`(?i)dashscope|百炼|通义|qwen`)},
	{ID: "minimax", Name: "MiniMax", Class: "key", Style: "openai",
		Base: "https://api.minimaxi.com/v1", BaseFallbacks: []string{"https://api.minimax.io/v1"},
		Model: "MiniMax-Text-01",
		Match: regexp.MustCompile(`(?i)minimax`)},
	{ID: "ark", Name: "火山引擎 ARK", Class: "key", Style: "openai",
		Base: "https://ark.cn-beijing.volces.com/api/v3", Model: "doubao-1-5-lite-32k-250115",
		Match: regexp.MustCompile(`(?i)\bark\b|volces|火山|豆包|doubao|方舟`)},
	{ID: "siliconflow", Name: "SiliconFlow 硅基流动", Class: "key", Style: "openai",
		Base: "https://api.siliconflow.cn/v1", Model: "Qwen/Qwen2.5-7B-Instruct", Quota: "siliconflow_user",
		Match: regexp.MustCompile(`(?i)siliconflow|硅基`)},
	{ID: "groq", Name: "Groq", Class: "key", Style: "openai",
		Base: "https://api.groq.com/openai/v1", Model: "openai/gpt-oss-20b",
		Prefixes: []string{"gsk_"},
		Match:    regexp.MustCompile(`(?i)groq`)},
	{ID: "nvidia", Name: "NVIDIA NIM", Class: "key", Style: "openai",
		Base: "https://integrate.api.nvidia.com/v1", Model: "meta/llama-3.1-8b-instruct",
		Prefixes: []string{"nvapi-"},
		Match:    regexp.MustCompile(`(?i)nvidia|\bnim\b`)},
	{ID: "gemini", Name: "Google Gemini (API key)", Class: "key", Style: "gemini",
		Base:     "https://generativelanguage.googleapis.com/v1beta",
		Prefixes: []string{"aiza"},
		Match:    regexp.MustCompile(`(?i)gemini|generativelanguage|google ai|aistudio`)},
	{ID: "anthropic", Name: "Anthropic Claude (API key)", Class: "key", Style: "anthropic",
		Base: "https://api.anthropic.com/v1", Model: "claude-3-haiku-20240307",
		Prefixes: []string{"sk-ant-api"},
		Match:    regexp.MustCompile(`(?i)anthropic|claude`)},
	{ID: "xai", Name: "xAI Grok", Class: "key", Style: "openai",
		Base: "https://api.x.ai/v1", Prefixes: []string{"xai-"},
		Match: regexp.MustCompile(`(?i)\bgrok\b|\bxai\b|x\.ai`)},
	{ID: "stepfun", Name: "StepFun 阶跃星辰", Class: "key", Style: "openai",
		Base:  "https://api.stepfun.com/v1",
		Match: regexp.MustCompile(`(?i)stepfun|阶跃`)},
	{ID: "xfyun", Name: "讯飞星火", Class: "key", Style: "openai",
		Base:  "https://spark-api-open.xf-yun.com/v1",
		Match: regexp.MustCompile(`(?i)讯飞|spark|xinghuo`)},
	{ID: "sensenova", Name: "商汤 SenseNova", Class: "key", Style: "openai",
		Base:  "https://api.sensenova.cn/compatible-mode/v1",
		Match: regexp.MustCompile(`(?i)sensenova|商汤`)},
	{ID: "relay", Name: "中转 / 兼容网关", Class: "key", Style: "relay",
		Match: regexp.MustCompile(`(?i)中转|代理网关|gateway|网关|relay|私有|vllm|one-?api|new-?api|9router|anyrouter`)},
}

var (
	officialDomains = map[string]bool{
		"openai.com": true, "anthropic.com": true, "googleapis.com": true, "x.ai": true,
		"grok.com": true, "bigmodel.cn": true, "z.ai": true, "deepseek.com": true,
		"moonshot.cn": true, "kimi.com": true, "kimi.ai": true, "minimaxi.com": true,
		"minimax.chat": true, "minimax.io": true, "aliyuncs.com": true, "volces.com": true,
		"groq.com": true, "nvidia.com": true, "siliconflow.cn": true, "siliconflow.com": true,
		"openrouter.ai": true, "github.com": true, "githubusercontent.com": true,
		"cloudflare.com": true, "modelscope.cn": true, "sensenova.cn": true,
		"xf-yun.com": true, "stepfun.com": true, "baidubce.com": true,
		"tencentcloudapi.com": true, "aliyun.com": true, "volcengine.com": true,
		"xiaomimimo.com": true, "opencode.ai": true, "nousresearch.com": true,
	}
	excludeRe     = regexp.MustCompile(`(?i)tavily|elevenlabs|\bfal\b|notion|apify|smtp|github|gitlab|aws|azure|gcp|ssh|ssl|vnc|jupyter|mysql|postgres|mariadb|wordpress|telegram|discord|feishu|dingtalk|wechat|weixin|slack|whatsapp|qq[\s_-]?bot|ilink|urlscan|fofa|shodan|zoomeye|censys|virustotal|securitytrails|leakix|binaryedge|greynoise|quake|hunter\.io|capsolver|serper|firecrawl|supabase|firebase|gmail|outlook|163\.com|qq\.com`)
	placeholderRe = regexp.MustCompile(`(?i)^(x{3,}|your[-_ ]|changeme|change[-_ ]me|example|sample|test[-_ ]?key|dummy|placeholder|redacted|replace[-_ ]?me|<[^>]+>|\$\{[^}]+\}|\{\{[^}]+\}|\.\.\.|…)$`)
	imServiceRe   = regexp.MustCompile(`(?i)(telegram|discord|feishu|飞书|lark|wechat|weixin|微信|公众号|dingtalk|钉钉|slack|wecom|企业微信|whatsapp|im[._-]?bot|ilink|qq[\s_-]?bot)`)
	domainRe      = regexp.MustCompile(`(?i)\b((?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+(?:com|cn|net|org|io|ai|app|dev|pro|xyz|me|co|top|site|cc|info|biz|shop|us|uk|de|fr|ru|su|eu|cloud|tech|link|live|online|store|fun|space|website|icu|club|art|jp|kr|br|in|id|tw|hk|sg|my|vn|th|ph|one|best|win|vip|work|studio|news|blog|host|today|life|world|zone|digital|media|agency|solutions|services|systems|network|software|tools|run|sh|gg|st|nu|se|no|fi|dk|nl|be|ch|at|it|es|pt|gr|cz|sk|ro|bg|lt|lv|ee|ua|by|kz|uz|az|ge|am|md|rs|hr|si|ba|mk|al|mt|is|ie|lu|li|monster))\b`)
)

// Classify returns the provider id and an optional base override inferred from
// the context. It never guesses a base from the secret itself.
func Classify(kind, value, service, context, sourceURL string) (providerID, baseOverride string) {
	text := strings.ToLower(strings.Join([]string{service, context, sourceURL}, " "))
	v := strings.ToLower(strings.TrimSpace(value))

	// OAuth package structure wins when the service is recognizable.
	if strings.Contains(v, `"refresh_token"`) || strings.Contains(v, `"access_token"`) ||
		strings.HasPrefix(v, "sk-ant-oat") || strings.HasPrefix(v, "gho_") || strings.HasPrefix(v, "ghu_") {
		for _, p := range providers {
			if p.Class != "oauth" {
				continue
			}
			if prefixMatch(p, v) || (p.Match != nil && p.Match.MatchString(text)) {
				return p.ID, ""
			}
		}
	}

	for _, p := range providers {
		if p.Class == "oauth" && !strings.Contains(text, "oauth") && !strings.Contains(v, "refresh_token") &&
			!strings.Contains(v, "access_token") && !prefixMatch(p, v) {
			continue
		}
		if prefixMatch(p, v) {
			return p.ID, ""
		}
	}
	for _, p := range providers {
		if p.Match != nil && p.Match.MatchString(text) {
			if p.ID == "gemini" && regexp.MustCompile(`(?i)places|maps|geocod|youtube|firebase|vision api|translate|cloud platform`).MatchString(text) {
				continue
			}
			if p.ID == "relay" {
				if d := extractDomain(sourceURL + " " + context); d != "" {
					return "relay", "https://" + d
				}
			}
			return p.ID, ""
		}
	}
	if excludeRe.MatchString(text) {
		return "", ""
	}
	if kind == KindLLMKey || kind == KindOAuth || kind == "" {
		if d := extractDomain(sourceURL + " " + context); d != "" {
			return "relay", "https://" + d
		}
	}
	return "", ""
}

func prefixMatch(p Provider, lowerValue string) bool {
	for _, prefix := range p.Prefixes {
		if strings.HasPrefix(lowerValue, strings.ToLower(prefix)) {
			return true
		}
	}
	return false
}

func extractDomain(text string) string {
	for _, m := range domainRe.FindAllString(text, -1) {
		d := strings.ToLower(strings.Trim(strings.TrimSpace(m), ".,;:)"))
		if d == "" || officialDomains[d] {
			continue
		}
		skip := false
		for root := range officialDomains {
			if strings.HasSuffix(d, "."+root) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if _, err := url.Parse("https://" + d); err == nil {
			return d
		}
	}
	return ""
}

// ProviderByID looks up a provider definition.
func ProviderByID(id string) (Provider, bool) {
	for _, p := range providers {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}

// ProviderRegistry returns the model-facing catalog.
func ProviderRegistry() []map[string]any {
	out := make([]map[string]any, 0, len(providers))
	for _, p := range providers {
		out = append(out, map[string]any{
			"id": p.ID, "name": p.Name, "class": p.Class, "style": p.Style,
			"base": p.Base, "model": p.Model, "quota": p.Quota,
		})
	}
	return out
}

// LooksLikePlaceholder rejects example/sample/redacted values.
func LooksLikePlaceholder(value string) bool {
	v := strings.TrimSpace(value)
	if v == "" {
		return true
	}
	if placeholderRe.MatchString(v) {
		return true
	}
	low := strings.ToLower(v)
	for _, prefix := range []string{"your-", "your_", "your ", "example", "sample", "changeme", "change-me", "test-key", "dummy", "placeholder", "redacted", "replace-me", "xxxx"} {
		if strings.HasPrefix(low, prefix) {
			return true
		}
	}
	// A value made of one repeated character is not a credential.
	if len(v) > 6 {
		first := v[0]
		allSame := true
		for i := 1; i < len(v); i++ {
			if v[i] != first {
				allSame = false
				break
			}
		}
		if allSame {
			return true
		}
	}
	return false
}

// IsIMService marks bot/IM credentials that must never enter the ledger.
func IsIMService(service string) bool { return imServiceRe.MatchString(service) }
