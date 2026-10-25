package credhunt

import (
	"net/url"
	"regexp"
	"strings"
)

// Endpoint is one protocol-specific API root a provider serves.
type Endpoint struct {
	Protocol string `json:"protocol"` // chat|responses|anthropic|gemini
	Base     string `json:"base"`
}

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
	Avail         string // openai_models|openai_chat|anthropic_chat|gemini_key|openrouter_key|oauth_usage|relay
	Headers       map[string]string
	Match         *regexp.Regexp
	Prefixes      []string
	Endpoint      string // oauth usage endpoint
	Variants      []Variant
	// Fine-grained preset data ported from magpie (MIT). Endpoints keeps one
	// root per protocol; Base remains the primary/legacy root.
	Preset     string
	Aliases    []string
	Hosts      []string
	Models     []string
	Endpoints  []Endpoint
	AuthHeader string // "", x-api-key, bearer
	Website    string
	KeysURL    string
}

// Variant is a provider sub-form (balance vs coding plan, Zen vs Go, token-plan
// region, z.ai vs bigmodel). A variant overrides base/model/avail/quota.
type Variant struct {
	ID            string
	Label         string
	Base          string
	BaseFallbacks []string
	Model         string
	Avail         string
	Quota         string
	Auto          *regexp.Regexp
	Endpoints     []Endpoint
	Models        []string
	Protocol      string
	AuthHeader    string
}

// Classification is the complete output of the provider classifier.
type Classification struct {
	ProviderID    string     `json:"provider"`
	ProviderName  string     `json:"provider_name"`
	PClass        string     `json:"pclass"`
	VariantID     string     `json:"variant"`
	VariantLabel  string     `json:"variant_label"`
	Style         string     `json:"style"`
	Base          string     `json:"base"`
	BaseOverride  string     `json:"base_override,omitempty"`
	Model         string     `json:"model"`
	QuotaKind     string     `json:"quota_kind"`
	Avail         string     `json:"avail"`
	MatchedBy     string     `json:"matched_by"`
	Confidence    float64    `json:"confidence"`
	Domain        string     `json:"domain,omitempty"`
	ServiceFamily string     `json:"service_family"`
	Protocol      string     `json:"protocol,omitempty"`
	Endpoints     []Endpoint `json:"endpoints,omitempty"`
	Models        []string   `json:"models,omitempty"`
	AuthHeader    string     `json:"auth,omitempty"`
	Preset        string     `json:"preset,omitempty"`
	Website       string     `json:"website,omitempty"`
	KeysURL       string     `json:"keys_url,omitempty"`
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

// init attaches the fine-grained sub-forms (variants) and availability adapters
// ported from the legacy LLM credential service.
func init() {
	variant := func(id, label, base, model, avail, quota, auto string, fallbacks ...string) Variant {
		v := Variant{ID: id, Label: label, Base: base, Model: model, Avail: avail, Quota: quota,
			BaseFallbacks: fallbacks}
		if auto != "" {
			v.Auto = regexp.MustCompile(`(?i)` + auto)
		}
		return v
	}
	set := func(id, avail string, variants ...Variant) {
		for i := range providers {
			if providers[i].ID == id {
				providers[i].Avail = avail
				providers[i].Variants = variants
				return
			}
		}
	}
	set("mimo", "openai_models",
		variant("balance", "余额版（按量付费）", "https://api.xiaomimimo.com/v1", "mimo-v2.5", "openai_models", "none", ""),
		variant("tokenplan_cn", "Token Plan（国内）", "https://token-plan-cn.xiaomimimo.com/v1", "mimo-v2.5-pro", "openai_models", "none", `token[-_ ]?plan[-_ ]*cn|xiaomi.?coding`),
		variant("tokenplan_sgp", "Token Plan（新加坡）", "https://token-plan-sgp.xiaomimimo.com/v1", "mimo-v2.5-pro", "openai_models", "none", `token[-_ ]?plan[-_ ]*sgp`),
		variant("tokenplan_ams", "Token Plan（阿姆斯特丹）", "https://token-plan-ams.xiaomimimo.com/v1", "mimo-v2.5-pro", "openai_models", "none", `token[-_ ]?plan[-_ ]*ams`),
	)
	set("opencode", "opencode_go_usage",
		variant("zen", "Zen（按量付费）", "https://opencode.ai/zen/v1", "", "opencode_go_usage", "opencode_go_usage", "", "https://opencode.ai/zen/go/v1"),
		variant("go", "Go（订阅套餐）", "https://opencode.ai/zen/go/v1", "", "opencode_go_usage", "opencode_go_usage", `\bgo\b`),
	)
	set("kimi", "openai_models",
		variant("balance", "余额版（按量付费）", "https://api.moonshot.cn/v1", "moonshot-v1-8k", "openai_models", "moonshot_balance", "", "https://api.moonshot.ai/v1"),
		variant("coding", "Kimi Coding 套餐", "https://api.kimi.com/coding", "", "kimi_coding", "kimi_coding", `coding|kimi\s*code`),
	)
	set("glm", "openai_chat",
		variant("balance", "余额版（开放平台）", "https://open.bigmodel.cn/api/paas/v4", "glm-4-flash", "openai_chat", "none", ""),
		variant("coding", "GLM Coding（国内）", "https://open.bigmodel.cn/api/paas/v4", "", "zhipu_coding", "zhipu_coding", `glm.?(coding|code)|coding.?plan`),
		variant("coding_zai", "GLM Coding（z.ai）", "https://api.z.ai/api/paas/v4", "", "zhipu_coding", "zhipu_coding", `z\.ai.*(coding|code)|(coding|code).*z\.ai|glm.?coding.*z\.ai`),
	)
	set("minimax", "openai_chat",
		variant("balance", "余额版（按量付费）", "https://api.minimaxi.com/v1", "MiniMax-Text-01", "openai_chat", "none", ""),
		variant("coding", "Token / Coding 套餐", "https://api.minimaxi.com/v1", "MiniMax-Text-01", "minimax_coding", "minimax_coding", `coding|token.?plan`, "https://api.minimax.io/v1"),
	)
	set("ark", "openai_chat",
		variant("balance", "余额版（按量付费）", "https://ark.cn-beijing.volces.com/api/v3", "doubao-1-5-lite-32k-250115", "openai_chat", "none", ""),
		variant("coding", "Coding Plan（/api/coding/v3）", "https://ark.cn-beijing.volces.com/api/coding/v3", "doubao-seed-code", "openai_chat", "none", `coding|arkcode`),
	)
	for i := range providers {
		switch providers[i].ID {
		case "openrouter":
			providers[i].Avail = "openrouter_key"
			providers[i].Quota = "openrouter_key"
		case "deepseek":
			providers[i].Avail = "openai_models"
		case "anthropic":
			providers[i].Avail = "anthropic_chat"
		case "gemini":
			providers[i].Avail = "gemini_key"
		case "relay":
			providers[i].Avail = "relay"
		case "codex_oauth", "claude_oauth", "copilot", "google_oauth":
			providers[i].Avail = "oauth_usage"
		}
	}
	mergeMagpiePresets()
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
		"mistral.ai": true, "together.xyz": true, "together.ai": true, "fireworks.ai": true,
		"aihubmix.com": true, "302.ai": true, "cherryin.ai": true, "yylx.io": true,
		"cline.bot": true, "kilo.ai": true, "commandcode.ai": true, "ollama.com": true,
		"pipellm.ai": true, "tencentmaas.com": true, "modelarts-maas.com": true,
		"lkeap.cloud.tencent.com": true,
	}
	excludeRe     = regexp.MustCompile(`(?i)tavily|elevenlabs|\bfal\b|notion|apify|smtp|github|gitlab|aws|azure|gcp|ssh|ssl|vnc|jupyter|mysql|postgres|mariadb|wordpress|telegram|discord|feishu|dingtalk|wechat|weixin|slack|whatsapp|qq[\s_-]?bot|ilink|urlscan|fofa|shodan|zoomeye|censys|virustotal|securitytrails|leakix|binaryedge|greynoise|quake|hunter\.io|capsolver|serper|firecrawl|supabase|firebase|gmail|outlook|163\.com|qq\.com`)
	placeholderRe = regexp.MustCompile(`(?i)^(x{3,}|your[-_ ]|changeme|change[-_ ]me|example|sample|test[-_ ]?key|dummy|placeholder|redacted|replace[-_ ]?me|<[^>]+>|\$\{[^}]+\}|\{\{[^}]+\}|\.\.\.|…)$`)
	imServiceRe   = regexp.MustCompile(`(?i)(telegram|discord|feishu|飞书|lark|wechat|weixin|微信|公众号|dingtalk|钉钉|slack|wecom|企业微信|whatsapp|im[._-]?bot|ilink|qq[\s_-]?bot)`)
	domainRe      = regexp.MustCompile(`(?i)\b((?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+(?:com|cn|net|org|io|ai|app|dev|pro|xyz|me|co|top|site|cc|info|biz|shop|us|uk|de|fr|ru|su|eu|cloud|tech|link|live|online|store|fun|space|website|icu|club|art|chat|jp|kr|br|in|id|tw|hk|sg|my|vn|th|ph|one|best|win|vip|work|studio|news|blog|host|today|life|world|zone|digital|media|agency|solutions|services|systems|network|software|tools|run|sh|gg|st|nu|se|no|fi|dk|nl|be|ch|at|it|es|pt|gr|cz|sk|ro|bg|lt|lv|ee|ua|by|kz|uz|az|ge|am|md|rs|hr|si|ba|mk|al|mt|is|ie|lu|li|monster))\b`)
)

// Classify returns the provider id and an optional base override inferred from
// the context. It never guesses a base from the secret itself.
func Classify(kind, value, service, context, sourceURL string) (providerID, baseOverride string) {
	c := ClassifyDetailed(kind, value, service, context, sourceURL)
	return c.ProviderID, c.BaseOverride
}

// ClassifyDetailed returns the full provider/variant/pclass classification used
// by the ledger facets and the verification adapters.
func ClassifyDetailed(kind, value, service, context, sourceURL string) Classification {
	text := strings.ToLower(strings.Join([]string{service, context, sourceURL}, " "))
	mainText := strings.ToLower(strings.TrimSpace(service))
	matchText := mainText
	if matchText == "" {
		matchText = text
	}
	v := strings.ToLower(strings.TrimSpace(value))
	c := Classification{ProviderName: "未识别", PClass: pclassForKind(kind), QuotaKind: "none",
		Avail: "none", MatchedBy: "unknown", Confidence: 0}
	switch {
	case strings.HasPrefix(kind, "proxy"):
		c.PClass = "proxy"
	case kind == KindOAuth:
		c.PClass = "oauth"
	case kind == KindLLMKey || kind == KindAPIKey || kind == KindToken:
		c.PClass = "key"
	case kind == KindPassword || kind == KindPrivateKey:
		c.PClass = "generic"
	}

	// OAuth package structure wins when the service is recognizable.
	if strings.Contains(v, `"refresh_token"`) || strings.Contains(v, `"access_token"`) ||
		strings.HasPrefix(v, "sk-ant-oat") || strings.HasPrefix(v, "gho_") || strings.HasPrefix(v, "ghu_") {
		for _, p := range providers {
			if p.Class != "oauth" {
				continue
			}
			if prefixMatch(p, v) || (p.Match != nil && p.Match.MatchString(text)) {
				return classificationFor(p, kind, text, "oauth", 0.96, "")
			}
		}
	}

	for _, p := range providers {
		if p.Class == "oauth" && !strings.Contains(text, "oauth") && !strings.Contains(v, "refresh_token") &&
			!strings.Contains(v, "access_token") && !prefixMatch(p, v) {
			continue
		}
		if prefixMatch(p, v) {
			return classificationFor(p, kind, text, "prefix", 0.97, "")
		}
	}
	// A concrete API base is the strongest evidence: it names the endpoint the
	// key is actually used at, even when the surrounding prose mentions
	// another vendor (a Kimi agent's .env holding a DashScope key).
	if out, ok := classifyByPreset(service+" "+context, kind); ok {
		return out
	}
	// A relay/gateway with a concrete non-official base URL wins over a vendor
	// keyword (e.g. "LongCat (Anthropic 兼容网关)").
	if relay, ok := ProviderByID("relay"); ok && relay.Match != nil && relay.Match.MatchString(matchText) {
		base := extractBaseURL(context)
		domain := extractDomain(context)
		if base != "" || domain != "" {
			out := classificationFor(relay, kind, text, "relay", 0.78, domain)
			if base != "" {
				out.Base, out.BaseOverride = base, base
				decorateRelayBase(&out, base, text)
			}
			return out
		}
	}
	for _, p := range providers {
		if p.Match != nil && p.Match.MatchString(matchText) {
			if p.ID == "gemini" && regexp.MustCompile(`(?i)places|maps|geocod|youtube|firebase|vision api|translate|cloud platform`).MatchString(text) {
				continue
			}
			if p.ID == "relay" {
				domain := extractDomain(sourceURL + " " + context)
				out := classificationFor(p, kind, text, "relay", 0.68, domain)
				if base := extractBaseURL(context); base != "" {
					out.Base, out.BaseOverride = base, base
					decorateRelayBase(&out, base, text)
				}
				return out
			}
			return classificationFor(p, kind, text, "regex", 0.9, "")
		}
	}
	if excludeRe.MatchString(text) {
		c.MatchedBy = "excluded"
		return c
	}
	if kind == KindLLMKey || kind == KindOAuth || kind == "" {
		if d := extractDomain(sourceURL + " " + context); d != "" {
			p, _ := ProviderByID("relay")
			out := classificationFor(p, kind, text, "relay-domain", 0.55, d)
			if base := extractBaseURL(context); base != "" {
				out.Base, out.BaseOverride = base, base
				decorateRelayBase(&out, base, text)
			}
			return out
		}
	}
	return c
}

func pclassForKind(kind string) string {
	switch {
	case strings.HasPrefix(kind, "proxy"):
		return "proxy"
	case kind == KindOAuth:
		return "oauth"
	case kind == KindLLMKey || kind == KindAPIKey || kind == KindToken:
		return "key"
	case kind == KindPassword || kind == KindPrivateKey:
		return "generic"
	default:
		return "generic"
	}
}

func classificationFor(p Provider, kind, text, matchedBy string, confidence float64, domain string) Classification {
	c := Classification{ProviderID: p.ID, ProviderName: p.Name, PClass: pclassForKind(kind),
		Style: p.Style, Base: p.Base, Model: p.Model, QuotaKind: defaultQuota(p),
		Avail: defaultAvail(p), MatchedBy: matchedBy, Confidence: confidence, ServiceFamily: p.ID,
		Preset: p.Preset, Endpoints: append([]Endpoint(nil), p.Endpoints...),
		Models: append([]string(nil), p.Models...), AuthHeader: p.AuthHeader,
		Website: p.Website, KeysURL: p.KeysURL}
	if len(c.Endpoints) > 0 {
		c.Protocol = c.Endpoints[0].Protocol
	}
	if p.Class == "oauth" {
		c.PClass = "oauth"
	}
	if p.ID == "relay" {
		c.PClass = "relay"
		c.ProviderName = "中转 / 兼容网关"
		if domain != "" {
			c.Base = "https://" + domain
			c.BaseOverride = c.Base
			c.Domain = domain
		}
	}
	if variant, ok := pickVariant(p, text); ok {
		c.VariantID, c.VariantLabel = variant.ID, variant.Label
		if variant.Base != "" {
			c.Base = variant.Base
		}
		if variant.Model != "" {
			c.Model = variant.Model
		}
		if variant.Avail != "" {
			c.Avail = variant.Avail
		}
		if variant.Quota != "" {
			c.QuotaKind = variant.Quota
		}
		if len(variant.Endpoints) > 0 {
			c.Endpoints = append([]Endpoint(nil), variant.Endpoints...)
		}
		if len(variant.Models) > 0 {
			c.Models = append([]string(nil), variant.Models...)
		}
		if variant.Protocol != "" {
			c.Protocol = variant.Protocol
		}
		if variant.AuthHeader != "" {
			c.AuthHeader = variant.AuthHeader
		}
	}
	return c
}

// decorateRelayBase records the protocol and endpoint of a relay's concrete
// base, and the one-header auth style when the context says so. LongCat's
// Anthropic gateway is a common case: it accepts Authorization: Bearer only.
func decorateRelayBase(c *Classification, base, text string) {
	c.Protocol = inferProtocol(base, "relay")
	c.Endpoints = []Endpoint{{Protocol: c.Protocol, Base: base}}
	l := strings.ToLower(text)
	if strings.Contains(l, "anthropic_auth_token") || strings.Contains(l, "authorization: bearer") ||
		strings.Contains(l, "authorization=bearer") || strings.Contains(l, "authorization：bearer") {
		c.AuthHeader = "bearer"
	}
}

func defaultQuota(p Provider) string {
	if p.Quota != "" {
		return p.Quota
	}
	return "none"
}

func defaultAvail(p Provider) string {
	if p.Avail != "" {
		return p.Avail
	}
	switch p.Style {
	case "oauth":
		return "oauth_usage"
	case "anthropic":
		return "anthropic_chat"
	case "gemini":
		return "gemini_key"
	case "relay":
		return "relay"
	default:
		return "openai_models"
	}
}

func pickVariant(p Provider, text string) (Variant, bool) {
	for _, v := range p.Variants {
		if v.Auto != nil && v.Auto.MatchString(text) {
			return v, true
		}
	}
	return Variant{}, false
}

// ProviderForEntry resolves the variant-adjusted provider used by verification.
func ProviderForEntry(e Entry) (Provider, bool) {
	p, ok := ProviderByID(e.Provider)
	if !ok {
		base := strings.TrimSpace(asString(e.Extra["base_url"]))
		if base == "" {
			return Provider{}, false
		}
		p = Provider{ID: "relay", Name: "中转 / 兼容网关", Class: "key", Style: "openai", Base: base}
	}
	if e.Variant != "" {
		for _, v := range p.Variants {
			if v.ID == e.Variant {
				if v.Base != "" {
					p.Base = v.Base
				}
				if len(v.BaseFallbacks) > 0 {
					p.BaseFallbacks = v.BaseFallbacks
				}
				if v.Model != "" {
					p.Model = v.Model
				}
				if v.Avail != "" {
					p.Avail = v.Avail
				}
				if v.Quota != "" {
					p.Quota = v.Quota
				}
				break
			}
		}
	}
	if base := strings.TrimSpace(asString(e.Extra["base_url"])); base != "" && p.Base == "" {
		p.Base = base
	}
	return p, true
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

var baseURLRe = regexp.MustCompile(`(?i)https?://[^\s"'<>]+`)

// extractBaseURL pulls a concrete non-official API URL from context/source.
func extractBaseURL(text string) string {
	for _, m := range baseURLRe.FindAllString(text, -1) {
		u := strings.TrimRight(m, ".,;:)]}")
		if i := strings.IndexFunc(u, func(r rune) bool { return r > 127 }); i >= 0 {
			u = u[:i]
		}
		u = strings.TrimRight(u, ".,;:)]}")
		parsed, err := url.Parse(u)
		if err != nil || parsed.Hostname() == "" {
			continue
		}
		host := strings.ToLower(parsed.Hostname())
		if officialDomains[host] {
			continue
		}
		skip := false
		for root := range officialDomains {
			if strings.HasSuffix(host, "."+root) {
				skip = true
				break
			}
		}
		if !skip {
			return u
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
		for _, a := range p.Aliases {
			if a == id {
				return p, true
			}
		}
	}
	return Provider{}, false
}

// ProviderRegistry returns the model-facing catalog.
func ProviderRegistry() []map[string]any {
	out := make([]map[string]any, 0, len(providers))
	for _, p := range providers {
		variants := make([]map[string]any, 0, len(p.Variants))
		for _, v := range p.Variants {
			variants = append(variants, map[string]any{"id": v.ID, "label": v.Label,
				"base": v.Base, "model": v.Model, "avail": v.Avail, "quota": v.Quota,
				"protocol": v.Protocol, "endpoints": v.Endpoints, "models": v.Models})
		}
		out = append(out, map[string]any{
			"id": p.ID, "name": p.Name, "class": p.Class, "pclass": p.Class, "style": p.Style,
			"base": p.Base, "model": p.Model, "quota": p.Quota, "avail": defaultAvail(p),
			"preset": p.Preset, "endpoints": p.Endpoints, "models": p.Models,
			"website": p.Website, "keys_url": p.KeysURL, "variants": variants,
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
