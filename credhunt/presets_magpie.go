package credhunt

import (
	_ "embed"
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Magpie's provider presets, exported from yetone/magpie (MIT). Only the data
// is used here: endpoints, regions, model hints, and key/console links. The
// probe engine in probe.go is a small port of magpie's test.go semantics.
//
//go:embed presets_magpie.json
var magpiePresetsJSON []byte

type magpieRegion struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Chat      string   `json:"chat,omitempty"`
	Responses string   `json:"responses,omitempty"`
	Anthropic string   `json:"anthropic,omitempty"`
	Gemini    string   `json:"gemini,omitempty"`
	Decide    string   `json:"decide,omitempty"`
	Catalog   string   `json:"catalog,omitempty"`
	Website   string   `json:"website,omitempty"`
	KeysURL   string   `json:"keysUrl,omitempty"`
	Models    []string `json:"models,omitempty"`
	Lists     bool     `json:"lists,omitempty"`
}

type magpiePreset struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Icon        string         `json:"icon"`
	Kind        string         `json:"kind"`
	Chat        string         `json:"chat,omitempty"`
	Responses   string         `json:"responses,omitempty"`
	Anthropic   string         `json:"anthropic,omitempty"`
	Gemini      string         `json:"gemini,omitempty"`
	Decide      string         `json:"decide,omitempty"`
	Catalog     string         `json:"catalog,omitempty"`
	Website     string         `json:"website,omitempty"`
	KeysURL     string         `json:"keysUrl,omitempty"`
	Note        string         `json:"note,omitempty"`
	Short       string         `json:"short,omitempty"`
	RegionLabel string         `json:"regionLabel,omitempty"`
	NoKey       bool           `json:"noKey,omitempty"`
	NoList      bool           `json:"noList,omitempty"`
	Models      []string       `json:"models,omitempty"`
	Regions     []magpieRegion `json:"regions,omitempty"`
}

// presetTarget maps a magpie preset onto the canonical ARTEX provider id. Old
// ARTEX ids are kept so the existing ledger and filters do not move.
var presetTarget = map[string]string{
	"anthropic":          "anthropic",
	"openai":             "openai",
	"google":             "gemini",
	"deepseek":           "deepseek",
	"xai":                "xai",
	"moonshot":           "kimi",
	"moonshot-cn":        "kimi",
	"kimi-code":          "kimi",
	"kimi-code-cn":       "kimi",
	"zhipu":              "glm",
	"zai":                "glm",
	"minimax":            "minimax",
	"minimax-cn":         "minimax",
	"stepfun":            "stepfun",
	"stepfun-cn":         "stepfun",
	"xiaomi":             "mimo",
	"baidu-qianfan":      "baidu",
	"tencent-cloud":      "tencent",
	"huaweicloud":        "huawei",
	"volcengine":         "ark",
	"qwen":               "dashscope",
	"qwen-cn":            "dashscope",
	"qwen-token-plan":    "dashscope",
	"bailian-token-plan": "dashscope",
	"mistral":            "mistral",
	"groq":               "groq",
	"ollama-cloud":       "ollama",
	"nous":               "nous",
	"openrouter":         "openrouter",
	"opencode-go":        "opencode",
	"opencode-zen":       "opencode",
	"clinepass":          "clinepass",
	"kilo":               "kilo",
	"commandcode":        "commandcode",
	"together":           "together",
	"fireworks":          "fireworks",
	"siliconflow":        "siliconflow",
	"nvidia":             "nvidia",
	"modelscope":         "modelscope",
	"aihubmix":           "aihubmix",
	"pipellm":            "pipellm",
	"302ai":              "302ai",
	"cherryin":           "cherryin",
	"yylx":               "yylx",
}

// presetVariant maps a magpie region id onto an existing ARTEX variant id.
// Regions not listed keep their magpie id.
var presetVariant = map[string]map[string]string{
	"zhipu":              {"api": "balance", "coding": "coding"},
	"zai":                {"api": "zai_api", "coding": "coding_zai"},
	"xiaomi":             {"plan-cn": "tokenplan_cn", "plan-sgp": "tokenplan_sgp", "plan-ams": "tokenplan_ams", "api": "balance"},
	"volcengine":         {"coding": "coding", "agent": "agent", "api": "balance"},
	"stepfun":            {"plan": "plan", "api": "balance"},
	"stepfun-cn":         {"plan": "plan_cn", "api": "balance"},
	"baidu-qianfan":      {"personal": "personal", "team": "team", "api": "balance"},
	"huaweicloud":        {"plan": "plan", "api": "balance"},
	"tencent-cloud":      {"plan": "plan", "cn": "cn", "intl": "intl", "enterprise": "enterprise", "enterprise-intl": "enterprise_intl"},
	"siliconflow":        {"cn": "cn", "intl": "intl"},
	"minimax":            {"": "global"},
	"minimax-cn":         {"": "balance"},
	"moonshot":           {"": "global"},
	"moonshot-cn":        {"": "balance"},
	"kimi-code":          {"": "coding_global"},
	"kimi-code-cn":       {"": "coding"},
	"opencode-go":        {"": "go"},
	"opencode-zen":       {"": "zen"},
	"qwen":               {"": "intl"},
	"qwen-cn":            {"": "cn"},
	"qwen-token-plan":    {"": "token_plan"},
	"bailian-token-plan": {"": "bailian_plan"},
	"google":             {"": "native"},
}

type presetEndpoint struct {
	PresetID   string
	ProviderID string
	VariantID  string
	Protocol   string
	Base       string
	Models     []string
	AuthHeader string
}

var (
	presetByID        = map[string]magpiePreset{}
	presetEndpointIdx []presetEndpoint
	presetEndpoints   = map[string]map[string][]Endpoint{}
)

// mergeMagpiePresets is called from providers.go's init after the hand-written
// registry and variants are in place.
func mergeMagpiePresets() {
	var presets []magpiePreset
	if err := json.Unmarshal(magpiePresetsJSON, &presets); err != nil {
		return
	}
	for _, mp := range presets {
		if !mergeablePreset(mp) {
			continue
		}
		presetByID[mp.ID] = mp
		pid, ok := presetTarget[mp.ID]
		if !ok {
			pid = mp.ID
		}
		eps := presetProviderEndpoints(mp)
		if len(eps) == 0 {
			continue
		}
		p, idx := findProviderForMerge(pid, mp.ID, mp.Name)
		if idx < 0 {
			p = Provider{
				ID: pid, Name: mp.Name, Class: "key", Style: styleForEndpoints(eps),
				Base: primaryBase(eps), Model: firstModel(mp.Models), Preset: mp.ID,
				Match: presetMatchRegex(mp.ID, mp.Name), Quota: presetQuota(mp.ID),
				Website: mp.Website, KeysURL: mp.KeysURL, Models: append([]string(nil), mp.Models...),
				Endpoints: eps, AuthHeader: presetAuth(mp.ID),
			}
			providers = append(providers, p)
			idx = len(providers) - 1
		} else {
			cur := &providers[idx]
			if cur.Preset == "" {
				cur.Preset = mp.ID
			}
			if cur.Name == "" || cur.Name == strings.ToLower(cur.Name) {
				cur.Name = mp.Name
			}
			if cur.Website == "" {
				cur.Website = mp.Website
			}
			if cur.KeysURL == "" {
				cur.KeysURL = mp.KeysURL
			}
			if cur.AuthHeader == "" {
				cur.AuthHeader = presetAuth(mp.ID)
			}
			cur.Endpoints = mergeEndpoints(cur.Endpoints, eps)
			if len(cur.Models) == 0 {
				cur.Models = append([]string(nil), mp.Models...)
			}
			if cur.Model == "" {
				cur.Model = firstModel(mp.Models)
			}
			if cur.Base == "" {
				cur.Base = primaryBase(eps)
			}
			if cur.Quota == "" {
				cur.Quota = presetQuota(mp.ID)
			}
		}
		cur := &providers[idx]
		cur.Hosts = appendUnique(cur.Hosts, hostsOf(eps)...)
		cur.Aliases = appendUnique(cur.Aliases, mp.ID)
		if cur.Match == nil {
			cur.Match = presetMatchRegex(mp.ID, mp.Name)
		}
		defaultVariant := ""
		if len(mp.Regions) == 0 {
			defaultVariant = presetVariantID(mp.ID, "")
		}
		presetEndpoints[mp.ID] = map[string][]Endpoint{"": eps}
		addPresetIndex(mp.ID, pid, defaultVariant, eps, mp.Models, presetAuth(mp.ID))
		if defaultVariant != "" {
			presetEndpoints[mp.ID][defaultVariant] = eps
			vv := Variant{
				ID: defaultVariant, Label: defaultVariantLabel(mp.ID), Base: primaryBase(eps),
				Model: firstModel(mp.Models), Endpoints: eps, Models: append([]string(nil), mp.Models...),
				Protocol: firstProtocol(eps), AuthHeader: presetAuth(mp.ID),
				Avail: defaultAvailForStyle(styleForEndpoints(eps)), Quota: presetQuota(mp.ID),
			}
			merged := false
			for i := range cur.Variants {
				if cur.Variants[i].ID == defaultVariant {
					v := &cur.Variants[i]
					v.Endpoints = mergeEndpoints(v.Endpoints, eps)
					if v.Protocol == "" {
						v.Protocol = vv.Protocol
					}
					if v.AuthHeader == "" {
						v.AuthHeader = vv.AuthHeader
					}
					merged = true
					break
				}
			}
			if !merged {
				cur.Variants = append(cur.Variants, vv)
			}
		}

		for _, r := range mp.Regions {
			reps := presetRegionEndpoints(r)
			if len(reps) == 0 {
				continue
			}
			vid := presetVariantID(mp.ID, r.ID)
			presetEndpoints[mp.ID][vid] = reps
			addPresetIndex(mp.ID, pid, vid, reps, r.Models, presetAuth(mp.ID))
			vv := Variant{
				ID: vid, Label: regionLabel(r), Base: primaryBase(reps),
				Model: firstModel(r.Models), Endpoints: reps, Models: append([]string(nil), r.Models...),
				Protocol: firstProtocol(reps), AuthHeader: presetAuth(mp.ID),
				Avail: defaultAvailForStyle(styleForEndpoints(reps)),
			}
			merged := false
			for i := range cur.Variants {
				if cur.Variants[i].ID == vid {
					v := &cur.Variants[i]
					if v.Base == "" {
						v.Base = vv.Base
					}
					if v.Model == "" {
						v.Model = vv.Model
					}
					v.Endpoints = mergeEndpoints(v.Endpoints, reps)
					if len(v.Models) == 0 {
						v.Models = vv.Models
					}
					if v.Protocol == "" {
						v.Protocol = vv.Protocol
					}
					if v.AuthHeader == "" {
						v.AuthHeader = vv.AuthHeader
					}
					merged = true
					break
				}
			}
			if !merged {
				cur.Variants = append(cur.Variants, vv)
			}
		}
	}
	sort.SliceStable(presetEndpointIdx, func(i, j int) bool {
		li, lj := len(presetEndpointIdx[i].Base), len(presetEndpointIdx[j].Base)
		if li != lj {
			return li > lj
		}
		// Same endpoint under a region and under the preset's default: the
		// region is the more specific answer.
		vi := presetEndpointIdx[i].VariantID != ""
		vj := presetEndpointIdx[j].VariantID != ""
		if vi != vj {
			return vi
		}
		return false
	})
}

func mergeablePreset(mp magpiePreset) bool {
	if mp.Kind == "local" || mp.Kind == "remote" {
		return false
	}
	switch mp.ID {
	case "google-vertex", "bedrock", "bedrock-mantle", "azure", "typesafe", "vercel-jev",
		"cloudflare-jev", "bailian-decision", "qwen-decision", "remote-magpie":
		return false
	}
	return true
}

func findProviderForMerge(pid, presetID, name string) (Provider, int) {
	for i := range providers {
		if providers[i].ID == pid || providers[i].Preset == presetID {
			return providers[i], i
		}
		for _, a := range providers[i].Aliases {
			if a == pid || a == presetID {
				return providers[i], i
			}
		}
	}
	return Provider{}, -1
}

func presetProviderEndpoints(mp magpiePreset) []Endpoint {
	return endpointsOf(mp.Chat, mp.Responses, mp.Anthropic, mp.Gemini)
}

func presetRegionEndpoints(r magpieRegion) []Endpoint {
	return endpointsOf(r.Chat, r.Responses, r.Anthropic, r.Gemini)
}

func endpointsOf(chat, responses, anthropic, gemini string) []Endpoint {
	var out []Endpoint
	add := func(protocol, base string) {
		base = strings.TrimSpace(base)
		if base == "" {
			return
		}
		for _, e := range out {
			if e.Protocol == protocol && strings.TrimRight(e.Base, "/") == strings.TrimRight(base, "/") {
				return
			}
		}
		out = append(out, Endpoint{Protocol: protocol, Base: base})
	}
	add("chat", chat)
	add("responses", responses)
	add("anthropic", anthropic)
	add("gemini", gemini)
	return out
}

func primaryBase(eps []Endpoint) string {
	for _, p := range []string{"chat", "anthropic", "responses", "gemini"} {
		for _, e := range eps {
			if e.Protocol == p {
				return e.Base
			}
		}
	}
	return ""
}

func firstProtocol(eps []Endpoint) string {
	if len(eps) == 0 {
		return ""
	}
	return eps[0].Protocol
}

func styleForEndpoints(eps []Endpoint) string {
	has := func(p string) bool {
		for _, e := range eps {
			if e.Protocol == p {
				return true
			}
		}
		return false
	}
	switch {
	case has("chat"):
		return "openai"
	case has("anthropic"):
		return "anthropic"
	case has("gemini"):
		return "gemini"
	default:
		return "openai"
	}
}

func defaultAvailForStyle(style string) string {
	switch style {
	case "anthropic":
		return "anthropic_chat"
	case "gemini":
		return "gemini_key"
	case "oauth":
		return "oauth_usage"
	case "relay":
		return "relay"
	default:
		return "openai_models"
	}
}

func firstModel(models []string) string {
	for _, m := range models {
		if strings.TrimSpace(m) != "" {
			return m
		}
	}
	return ""
}

func regionLabel(r magpieRegion) string {
	if strings.TrimSpace(r.Name) != "" {
		return r.Name
	}
	return r.ID
}

func defaultVariantLabel(presetID string) string {
	switch presetID {
	case "moonshot":
		return "余额版（国际）"
	case "moonshot-cn":
		return "余额版（按量付费）"
	case "kimi-code":
		return "Kimi Coding（国际）"
	case "kimi-code-cn":
		return "Kimi Coding 套餐"
	case "minimax":
		return "余额版（国际）"
	case "minimax-cn":
		return "余额版（按量付费）"
	case "opencode-go":
		return "Go（订阅套餐）"
	case "opencode-zen":
		return "Zen（按量付费）"
	case "qwen":
		return "Qwen 国际"
	case "qwen-cn":
		return "Qwen 国内"
	case "qwen-token-plan":
		return "Qwen Token Plan"
	case "bailian-token-plan":
		return "百炼 Token Plan"
	case "google":
		return "Gemini 原生"
	default:
		return presetID
	}
}

func presetVariantID(presetID, regionID string) string {
	if m, ok := presetVariant[presetID]; ok {
		if v, ok := m[regionID]; ok {
			return v
		}
	}
	if regionID == "" {
		return ""
	}
	return regionID
}

func presetAuth(presetID string) string {
	switch presetID {
	case "anthropic":
		return "x-api-key"
	}
	return ""
}

func presetQuota(presetID string) string {
	switch presetID {
	case "deepseek":
		return "deepseek_balance"
	case "moonshot", "moonshot-cn":
		return "moonshot_balance"
	case "openrouter":
		return "openrouter_key"
	case "siliconflow":
		return "siliconflow_user"
	case "stepfun", "stepfun-cn":
		return "stepfun_balance"
	case "aihubmix":
		return "aihubmix_balance"
	case "commandcode":
		return "commandcode_balance"
	case "zhipu", "zai":
		// Only the Coding Plan variants ask the plan quota endpoint; the
		// provider-level (pay-as-you-go) default has no balance adapter.
		return "none"
	case "kimi-code", "kimi-code-cn":
		return "kimi_coding"
	case "minimax", "minimax-cn":
		// The Coding Plan variant carries the quota; the balance variants do not.
		return "none"
	case "opencode-go":
		return "opencode_go_usage"
	case "ollama-cloud":
		return "ollama_balance"
	default:
		return "none"
	}
}

func presetMatchRegex(id, name string) *regexp.Regexp {
	parts := []string{regexp.QuoteMeta(id), regexp.QuoteMeta(name)}
	for _, kw := range presetKeywords[id] {
		parts = append(parts, regexp.QuoteMeta(kw))
	}
	return regexp.MustCompile(`(?i)(` + strings.Join(parts, "|") + `)`)
}

// presetKeywords are the short service names that identify a preset. They are
// deliberately curated: splitting a display name into words makes "Cloud"
// match Cloudflare and "AI" match half the ledger.
var presetKeywords = map[string][]string{
	"openai":      {"openai", "sk-proj-"},
	"huawei":      {"huawei", "modelarts"},
	"baidu":       {"baidu", "qianfan", "千帆"},
	"mistral":     {"mistral"},
	"together":    {"together ai", "togetherai"},
	"fireworks":   {"fireworks"},
	"modelscope":  {"modelscope", "魔搭"},
	"aihubmix":    {"aihubmix"},
	"302ai":       {"302.ai", "302ai"},
	"cherryin":    {"cherryin", "cherry in"},
	"commandcode": {"commandcode", "command code"},
	"clinepass":   {"clinepass", "cline pass"},
	"kilo":        {"kilo gateway", "kilocode"},
	"ollama":      {"ollama"},
	"pipellm":     {"pipellm"},
	"yylx":        {"yylx", "鱼鱼连线"},
}

func hostsOf(eps []Endpoint) []string {
	var out []string
	for _, e := range eps {
		u, err := url.Parse(strings.TrimSpace(e.Base))
		if err != nil || u.Hostname() == "" {
			continue
		}
		out = appendUnique(out, strings.ToLower(u.Hostname()))
	}
	return out
}

func appendUnique(dst []string, vals ...string) []string {
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		found := false
		for _, x := range dst {
			if x == v {
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, v)
		}
	}
	return dst
}

func mergeEndpoints(dst, add []Endpoint) []Endpoint {
	for _, e := range add {
		found := false
		for _, x := range dst {
			if x.Protocol == e.Protocol && strings.TrimRight(x.Base, "/") == strings.TrimRight(e.Base, "/") {
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, e)
		}
	}
	return dst
}

func addPresetIndex(presetID, providerID, variantID string, eps []Endpoint, models []string, auth string) {
	for _, e := range eps {
		presetEndpointIdx = append(presetEndpointIdx, presetEndpoint{
			PresetID: presetID, ProviderID: providerID, VariantID: variantID,
			Protocol: e.Protocol, Base: e.Base, Models: append([]string(nil), models...), AuthHeader: auth,
		})
	}
}

// classifyByPreset looks for a concrete API base in the context. Host and path
// are stronger evidence than a vendor keyword (a LongCat gateway mentions
// Anthropic; a DashScope key may sit in a Kimi agent's .env).
func classifyByPreset(text string, kind string) (Classification, bool) {
	if kind == KindOAuth {
		// OAuth packages are classified by their own package/usage endpoint,
		// not by a Chat/API base that happens to appear in the context.
		return Classification{}, false
	}
	// A non-official URL in the context is the endpoint the key is actually
	// used at. If it is a known vendor, trust it; if it is an unknown relay,
	// do not let an official upstream URL elsewhere in the prose win.
	if custom := extractBaseURL(text); custom != "" {
		if c, ok := matchPresetURL(custom, kind, text); ok {
			return c, true
		}
		return Classification{}, false
	}
	for _, raw := range presetURLCandidates(text) {
		if c, ok := matchPresetURL(raw, kind, text); ok {
			return c, true
		}
	}
	return Classification{}, false
}

func matchPresetURL(raw, kind, text string) (Classification, bool) {
	raw = strings.TrimRight(strings.TrimSpace(raw), ".,;:)]}\"'")
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return Classification{}, false
	}
	host := strings.ToLower(u.Hostname())
	for _, pe := range presetEndpointIdx {
		if !baseMatches(raw, pe.Base) {
			continue
		}
		if kind == KindOAuth && !strings.Contains(strings.ToLower(pe.PresetID), "oauth") &&
			!strings.Contains(strings.ToLower(pe.PresetID), "code") {
			continue
		}
		return classificationForPreset(pe, kind, text), true
	}
	// Host-only match: the context names the vendor but not the path.
	for _, pe := range presetEndpointIdx {
		pu, err := url.Parse(pe.Base)
		if err != nil || strings.ToLower(pu.Hostname()) != host {
			continue
		}
		if pe.VariantID != "" {
			continue
		}
		return classificationForPreset(pe, kind, text), true
	}
	return Classification{}, false
}

func presetURLCandidates(text string) []string {
	var out []string
	for _, m := range baseURLRe.FindAllString(text, -1) {
		out = append(out, m)
	}
	for _, m := range domainRe.FindAllString(text, -1) {
		d := strings.Trim(strings.TrimSpace(m), ".,;:)]}\"'")
		if d == "" {
			continue
		}
		out = append(out, "https://"+d)
	}
	return out
}

func baseMatches(raw, base string) bool {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" || len(raw) < len(base) {
		return false
	}
	if !strings.EqualFold(raw[:len(base)], base) {
		return false
	}
	if len(raw) == len(base) {
		return true
	}
	switch raw[len(base)] {
	case '/', '?', '#':
		return true
	}
	return false
}

func classificationForPreset(pe presetEndpoint, kind, text string) Classification {
	p, ok := ProviderByID(pe.ProviderID)
	if !ok {
		p = Provider{ID: pe.ProviderID, Name: pe.ProviderID, Class: "key", Style: "openai",
			Base: pe.Base, Endpoints: []Endpoint{{pe.Protocol, pe.Base}}}
	}
	c := classificationFor(p, kind, text, "preset-host", 0.99, "")
	c.Preset = pe.PresetID
	c.Protocol = pe.Protocol
	c.Base = pe.Base
	c.BaseOverride = pe.Base
	c.Endpoints = endpointSet(pe.PresetID, pe.VariantID)
	if len(c.Endpoints) == 0 {
		c.Endpoints = []Endpoint{{Protocol: pe.Protocol, Base: pe.Base}}
	}
	c.Models = append([]string(nil), pe.Models...)
	if len(c.Models) == 0 {
		c.Models = append([]string(nil), p.Models...)
	}
	c.AuthHeader = pe.AuthHeader
	c.Website = p.Website
	c.KeysURL = p.KeysURL
	// A host match may land on a vendor's default entry while the service
	// text names a plan ("GLM Coding", "Kimi Coding"). Keep the variant the
	// regex classifier picked, and use that variant's endpoints/quota.
	picked := pe.VariantID
	if c.VariantID != "" {
		if pe.VariantID == "" || (c.VariantID != pe.VariantID && variantHasEndpoint(p, c.VariantID, pe.Base)) {
			// The host match may land on a shared endpoint (Zhipu's
			// /api/anthropic is the same for pay-as-you-go and Coding Plan),
			// while the service text names the plan. The plan wins when its
			// endpoints actually include the matched base.
			picked = c.VariantID
		}
	}
	c.VariantID = picked
	if picked != "" {
		for _, v := range p.Variants {
			if v.ID != picked {
				continue
			}
			c.VariantLabel = v.Label
			if pe.VariantID == "" {
				if len(v.Endpoints) > 0 {
					c.Endpoints = append([]Endpoint(nil), v.Endpoints...)
				}
				if len(v.Models) > 0 {
					c.Models = append([]string(nil), v.Models...)
				}
				if v.Protocol != "" {
					c.Protocol = v.Protocol
				}
			}
			if v.Quota != "" && v.Quota != "none" {
				c.QuotaKind = v.Quota
			}
			if v.Avail != "" {
				c.Avail = v.Avail
			}
			if v.AuthHeader != "" {
				c.AuthHeader = v.AuthHeader
			}
			break
		}
	}
	if c.VariantLabel == "" {
		c.VariantLabel = c.VariantID
	}
	return c
}

func variantHasEndpoint(p Provider, variantID, base string) bool {
	for _, v := range p.Variants {
		if v.ID != variantID {
			continue
		}
		for _, e := range v.Endpoints {
			if baseMatches(base, e.Base) || baseMatches(e.Base, base) {
				return true
			}
		}
	}
	return false
}

func endpointSet(presetID, variantID string) []Endpoint {
	if m, ok := presetEndpoints[presetID]; ok {
		if eps, ok := m[variantID]; ok {
			return append([]Endpoint(nil), eps...)
		}
		if eps, ok := m[""]; ok {
			return append([]Endpoint(nil), eps...)
		}
	}
	return nil
}
