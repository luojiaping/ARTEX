package credhunt

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// Probe protocols, matching magpie's four wire APIs.
const (
	ProtoChat      = "chat"
	ProtoResponses = "responses"
	ProtoAnthropic = "anthropic"
	ProtoGemini    = "gemini"
)

// ProbeTarget is one protocol/base pair a credential can be tried at.
type ProbeTarget struct {
	ProviderID    string
	ProviderName  string
	VariantID     string
	VariantLabel  string
	Protocol      string
	Base          string
	AuthHeader    string
	Models        []string
	FallbackModel string
	QuotaKind     string
}

// ProbeOutcome is the result of one target. Kind is internal: valid, invalid,
// no_balance, rate_limited, unknown, endpoint_missing or model_missing.
type ProbeOutcome struct {
	Kind       string
	Detail     string
	HTTPStatus int
	Endpoint   string
	Model      string
	Protocol   string
	QuotaKind  string
	ErrorType  string
	ElapsedMS  int64
}

type modelCacheEntry struct {
	models  []string
	expires time.Time
}

func (v *Verifier) probeTargets(e Entry, p Provider) []ProbeTarget {
	variants := p.Variants
	var variant Variant
	for _, vv := range variants {
		if vv.ID == e.Variant {
			variant = vv
			break
		}
	}

	// Classification stores the exact endpoints and models found at ingest
	// time. They are the strongest evidence and survive a reclassification.
	var endpoints []Endpoint
	var models []string
	auth := ""
	classified := false
	if e.Classification != nil {
		endpoints = endpointsFromAny(e.Classification["endpoints"])
		models = stringsFromAny(e.Classification["models"])
		auth = asString(e.Classification["auth"])
		classified = len(endpoints) > 0
	}
	if len(endpoints) == 0 && variant.ID != "" {
		endpoints = variant.Endpoints
		if len(models) == 0 {
			models = variant.Models
		}
		if auth == "" {
			auth = variant.AuthHeader
		}
	}
	if len(endpoints) == 0 {
		endpoints = p.Endpoints
	}
	if len(models) == 0 {
		models = p.Models
	}
	if auth == "" {
		auth = p.AuthHeader
	}

	baseOverride := strings.TrimSpace(asString(e.Extra["base_url"]))
	if baseOverride != "" {
		endpoints = append(endpoints, Endpoint{Protocol: inferProtocol(baseOverride, p.Style), Base: baseOverride})
	}
	if len(endpoints) == 0 {
		base := p.Base
		if variant.Base != "" {
			base = variant.Base
		}
		if base != "" {
			endpoints = []Endpoint{{Protocol: inferProtocol(base, p.Style), Base: base}}
		}
	}
	fallbackModel := variant.Model
	if fallbackModel == "" {
		fallbackModel = p.Model
	}
	if fallbackModel == "" {
		fallbackModel = asString(e.Classification["model"])
	}

	targets := make([]ProbeTarget, 0, len(endpoints))
	seen := map[string]bool{}
	for _, ep := range endpoints {
		ep.Base = strings.TrimSpace(ep.Base)
		if ep.Base == "" {
			continue
		}
		proto := ep.Protocol
		if proto == "" {
			proto = inferProtocol(ep.Base, p.Style)
		}
		key := proto + "|" + strings.TrimRight(ep.Base, "/")
		if seen[key] {
			continue
		}
		seen[key] = true
		targets = append(targets, ProbeTarget{
			ProviderID: p.ID, ProviderName: p.Name, VariantID: variant.ID,
			VariantLabel: variant.Label, Protocol: proto, Base: ep.Base,
			AuthHeader: auth, Models: append([]string(nil), models...),
			FallbackModel: fallbackModel, QuotaKind: quotaKindFor(p, variant),
		})
	}
	if hint := asString(e.Classification["protocol"]); hint != "" {
		preferProtocol(targets, []string{hint, ProtoChat, ProtoResponses, ProtoAnthropic, ProtoGemini})
	} else if !classified {
		preferProtocol(targets, probeOrder(e, variant, p))
	}
	return targets
}

func endpointsFromAny(v any) []Endpoint {
	rows, ok := v.([]any)
	if !ok {
		if bs, ok := v.([]Endpoint); ok {
			return append([]Endpoint(nil), bs...)
		}
		return nil
	}
	out := make([]Endpoint, 0, len(rows))
	for _, row := range rows {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		base := asString(m["base"])
		proto := asString(m["protocol"])
		if base == "" {
			continue
		}
		out = append(out, Endpoint{Protocol: proto, Base: base})
	}
	return out
}

func stringsFromAny(v any) []string {
	rows, ok := v.([]any)
	if !ok {
		if ss, ok := v.([]string); ok {
			return append([]string(nil), ss...)
		}
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if s := asString(row); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func quotaKindFor(p Provider, v Variant) string {
	if v.Quota != "" {
		return v.Quota
	}
	return defaultQuota(p)
}

func inferProtocol(base, style string) string {
	lower := strings.ToLower(base)
	switch {
	case strings.Contains(lower, "/anthropic") || strings.HasSuffix(lower, "/v1/messages"):
		return ProtoAnthropic
	case strings.Contains(lower, "generativelanguage.googleapis.com") && !strings.Contains(lower, "/openai"):
		return ProtoGemini
	case strings.Contains(lower, ":generatecontent"):
		return ProtoGemini
	case strings.Contains(lower, "/responses"):
		return ProtoResponses
	case style == "anthropic":
		return ProtoAnthropic
	case style == "gemini":
		return ProtoGemini
	default:
		return ProtoChat
	}
}

func probeOrder(e Entry, v Variant, p Provider) []string {
	if v.Protocol != "" {
		return []string{v.Protocol, ProtoChat, ProtoResponses, ProtoAnthropic, ProtoGemini}
	}
	// A key's own shape is the best hint for which wire API it speaks.
	if strings.HasPrefix(strings.ToLower(e.Masked), "sk-ant-") || p.ID == "anthropic" {
		return []string{ProtoAnthropic, ProtoChat, ProtoResponses, ProtoGemini}
	}
	if p.ID == "gemini" || strings.Contains(strings.ToLower(p.Name), "gemini") {
		return []string{ProtoGemini, ProtoChat, ProtoAnthropic, ProtoResponses}
	}
	return []string{ProtoChat, ProtoResponses, ProtoAnthropic, ProtoGemini}
}

func preferProtocol(targets []ProbeTarget, order []string) {
	rank := map[string]int{}
	for i, p := range order {
		rank[p] = i
	}
	sort.SliceStable(targets, func(i, j int) bool {
		ri, ok := rank[targets[i].Protocol]
		if !ok {
			ri = 99
		}
		rj, ok := rank[targets[j].Protocol]
		if !ok {
			rj = 99
		}
		return ri < rj
	})
}

func (v *Verifier) probeTarget(ctx context.Context, target ProbeTarget, key string) ProbeOutcome {
	models := v.modelsFor(ctx, target, key)
	if len(models) == 0 {
		models = fallbackModels(target)
	}
	var last ProbeOutcome
	for i, model := range models {
		if i >= 4 {
			break
		}
		out := v.probeOnce(ctx, target, key, model, true)
		out.Model, out.Protocol = model, target.Protocol
		switch out.Kind {
		case "valid", "invalid", "no_balance", "rate_limited":
			return out
		case "model_missing":
			last = out
			continue
		case "endpoint_missing":
			return out
		default:
			last = out
		}
	}
	if last.Kind == "" {
		last = ProbeOutcome{Kind: "unknown", Detail: "no model to try", Model: firstString(models), Protocol: target.Protocol}
	}
	return last
}

func (v *Verifier) modelsFor(ctx context.Context, target ProbeTarget, key string) []string {
	models := append([]string(nil), target.Models...)
	if len(models) == 0 {
		models = append(models, v.discoverModels(ctx, target, key)...)
	}
	if target.FallbackModel != "" {
		models = append(models, target.FallbackModel)
	}
	models = append(models, fallbackModels(target)...)
	out := make([]string, 0, 4)
	seen := map[string]bool{}
	for _, m := range models {
		m = strings.TrimSpace(m)
		if m == "" || seen[m] || !looksChatModel(m) {
			continue
		}
		seen[m] = true
		out = append(out, m)
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func looksChatModel(id string) bool {
	l := strings.ToLower(id)
	for _, bad := range []string{"embedding", "embed-", "whisper", "tts", "text-to-speech", "dall-e", "image", "stable-diffusion",
		"moderation", "rerank", "audio", "transcribe", "speech", "video", "veo", "sora"} {
		if strings.Contains(l, bad) {
			return false
		}
	}
	return true
}

func fallbackModels(t ProbeTarget) []string {
	switch t.Protocol {
	case ProtoAnthropic:
		return []string{"claude-3-5-haiku-latest", "claude-3-haiku-20240307"}
	case ProtoGemini:
		return []string{"gemini-2.0-flash", "gemini-1.5-flash"}
	}
	switch t.ProviderID {
	case "deepseek":
		return []string{"deepseek-chat"}
	case "kimi":
		return []string{"moonshot-v1-8k", "kimi-k2.5"}
	case "glm":
		return []string{"glm-4-flash", "glm-4.5-flash"}
	case "dashscope":
		return []string{"qwen-turbo", "qwen-plus"}
	case "minimax":
		return []string{"MiniMax-Text-01"}
	case "openai":
		return []string{"gpt-4o-mini", "gpt-3.5-turbo"}
	default:
		return []string{"gpt-4o-mini", "gpt-3.5-turbo"}
	}
}

func firstString(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	return ss[0]
}

func (v *Verifier) discoverModels(ctx context.Context, target ProbeTarget, key string) []string {
	cacheKey := target.ProviderID + "|" + target.Protocol + "|" + strings.TrimRight(target.Base, "/")
	v.modelMu.Lock()
	if v.modelCache == nil {
		v.modelCache = map[string]modelCacheEntry{}
	}
	if c, ok := v.modelCache[cacheKey]; ok && time.Now().Before(c.expires) {
		v.modelMu.Unlock()
		return append([]string(nil), c.models...)
	}
	v.modelMu.Unlock()

	models := v.fetchModels(ctx, target, key)
	if len(models) > 0 {
		v.modelMu.Lock()
		v.modelCache[cacheKey] = modelCacheEntry{models: append([]string(nil), models...), expires: time.Now().Add(10 * time.Minute)}
		v.modelMu.Unlock()
	}
	return models
}

func (v *Verifier) fetchModels(ctx context.Context, target ProbeTarget, key string) []string {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	for _, u := range modelListURLs(target) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		setModelListAuth(req, target, key)
		resp, err := v.client.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			continue
		}
		if ms := parseModelList(body); len(ms) > 0 {
			return ms
		}
	}
	return nil
}

func modelListURLs(t ProbeTarget) []string {
	base := strings.TrimRight(strings.TrimSpace(t.Base), "/")
	if base == "" {
		return nil
	}
	if t.Protocol == ProtoGemini {
		base = strings.TrimSuffix(base, "/openai")
		return []string{base + "/models?pageSize=1000"}
	}
	var out []string
	add := func(u string) {
		u = strings.TrimRight(u, "/")
		if u == "" {
			return
		}
		for _, x := range out {
			if x == u {
				return
			}
		}
		out = append(out, u)
	}
	if strings.HasSuffix(base, "/models") {
		add(base)
		return out
	}
	add(base + "/models")
	if !hasVersionSegment(base) {
		add(base + "/v1/models")
	}
	root := base
	for _, suffix := range []string{"/anthropic", "/apps/anthropic", "/api/anthropic", "/v1", "/v1beta", "/compatible-mode/v1",
		"/api/paas/v4", "/api/coding/paas/v4", "/api/v3", "/api/coding/v3", "/api/plan/v3", "/coding/v1", "/coding", "/plan/v3"} {
		if strings.HasSuffix(root, suffix) {
			root = strings.TrimSuffix(root, suffix)
			break
		}
	}
	if root != base {
		add(root + "/v1/models")
		add(root + "/models")
	}
	return out
}

func hasVersionSegment(base string) bool {
	path := base
	if _, after, ok := strings.Cut(base, "://"); ok {
		if i := strings.Index(after, "/"); i >= 0 {
			path = after[i:]
		} else {
			path = ""
		}
	}
	for _, seg := range strings.Split(path, "/") {
		if len(seg) >= 2 && (seg[0] == 'v' || seg[0] == 'V') && seg[1] >= '0' && seg[1] <= '9' {
			return true
		}
	}
	return false
}

func setModelListAuth(req *http.Request, t ProbeTarget, key string) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ARTEX-CredHunter/2.0")
	switch t.Protocol {
	case ProtoAnthropic:
		if t.AuthHeader == "bearer" {
			req.Header.Set("Authorization", "Bearer "+key)
		} else {
			req.Header.Set("x-api-key", key)
			req.Header.Set("anthropic-version", "2023-06-01")
		}
	case ProtoGemini:
		req.Header.Set("x-goog-api-key", key)
	default:
		req.Header.Set("Authorization", "Bearer "+key)
	}
}

func parseModelList(body []byte) []string {
	var out []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		id = strings.TrimPrefix(id, "models/")
		if id == "" || !looksChatModel(id) {
			return
		}
		for _, x := range out {
			if x == id {
				return
			}
		}
		out = append(out, id)
	}
	var v any
	if json.Unmarshal(body, &v) != nil {
		return nil
	}
	walkModelList(v, add)
	if len(out) > 200 {
		out = out[:200]
	}
	return out
}

func walkModelList(v any, add func(string)) {
	switch x := v.(type) {
	case []any:
		for _, row := range x {
			walkModelList(row, add)
		}
	case map[string]any:
		for _, key := range []string{"id", "name", "model", "model_name"} {
			if s, ok := x[key].(string); ok && s != "" {
				add(s)
				break
			}
		}
		for _, key := range []string{"data", "models", "items"} {
			if child, ok := x[key]; ok {
				walkModelList(child, add)
			}
		}
	}
}

func (v *Verifier) probeOnce(ctx context.Context, target ProbeTarget, key, model string, allowRetry bool) ProbeOutcome {
	start := time.Now()
	out := ProbeOutcome{Endpoint: target.Base, Model: model, Protocol: target.Protocol, QuotaKind: target.QuotaKind}
	defer func() { out.ElapsedMS = time.Since(start).Milliseconds() }()
	if model == "" {
		out.Kind, out.Detail = "model_missing", "no model to try"
		return out
	}
	endpoint, body, err := buildProbeRequest(target, model)
	if err != nil {
		out.Kind, out.Detail = "unknown", err.Error()
		return out
	}
	out.Endpoint = endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		out.Kind, out.Detail = "unknown", err.Error()
		return out
	}
	setProbeHeaders(req, target, key)
	resp, err := v.client.Do(req)
	if err != nil {
		out.Kind, out.Detail = "unknown", "network: "+err.Error()
		return out
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	out.HTTPStatus = resp.StatusCode

	if resp.StatusCode == http.StatusUnauthorized && target.Protocol == ProtoAnthropic && target.AuthHeader == "" {
		if bothAuthRefused(raw) {
			for _, only := range []string{"x-api-key", "bearer"} {
				again := target
				again.AuthHeader = only
				r := v.probeOnce(ctx, again, key, model, false)
				if r.Kind == "valid" || r.Kind == "no_balance" || r.Kind == "rate_limited" {
					return r
				}
				if r.Kind == "invalid" {
					return r
				}
			}
		}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if kind, detail, typ := validateProbeSuccess(resp.Header.Get("Content-Type"), raw); kind != "valid" {
			out.Kind, out.Detail, out.ErrorType = kind, detail, typ
			return out
		}
		out.Kind, out.Detail = "valid", "probe accepted by "+endpoint
		return out
	}

	if allowRetry && isZhipuNoBalance(target, resp.StatusCode, raw) {
		coding := target
		coding.Base = strings.Replace(target.Base, "/api/paas/v4", "/api/coding/paas/v4", 1)
		if coding.Base != target.Base {
			r := v.probeOnce(ctx, coding, key, model, false)
			if r.Kind == "valid" || r.Kind == "rate_limited" || r.Kind == "no_balance" || r.Kind == "invalid" {
				if r.Kind == "no_balance" {
					r.Detail = r.Detail + "（pay-as-you-go 与 Coding Plan 入口均未通过）"
				}
				r.QuotaKind = "zhipu_coding"
				return r
			}
		}
	}

	kind, detail, typ, modelMissing := classifyProbeFailure(resp.StatusCode, raw)
	out.Kind, out.Detail, out.ErrorType = kind, detail, typ
	if modelMissing {
		out.Kind = "model_missing"
	}
	return out
}

func buildProbeRequest(t ProbeTarget, model string) (string, []byte, error) {
	base := strings.TrimRight(strings.TrimSpace(t.Base), "/")
	if base == "" {
		return "", nil, fmt.Errorf("empty base")
	}
	switch t.Protocol {
	case ProtoChat:
		if strings.HasSuffix(base, "/chat/completions") {
			return base, chatBody(model), nil
		}
		return base + "/chat/completions", chatBody(model), nil
	case ProtoResponses:
		if strings.HasSuffix(base, "/responses") {
			return base, responsesBody(model), nil
		}
		return base + "/responses", responsesBody(model), nil
	case ProtoAnthropic:
		switch {
		case strings.HasSuffix(base, "/v1/messages"), strings.HasSuffix(base, "/messages"):
			return base, anthropicBody(model), nil
		case strings.HasSuffix(base, "/v1"):
			return base + "/messages", anthropicBody(model), nil
		default:
			return base + "/v1/messages", anthropicBody(model), nil
		}
	case ProtoGemini:
		if strings.Contains(base, "/openai") {
			return base + "/chat/completions", chatBody(model), nil
		}
		m := strings.TrimPrefix(model, "models/")
		return base + "/models/" + url.PathEscape(m) + ":generateContent", geminiBody(model), nil
	}
	return "", nil, fmt.Errorf("unknown protocol %q", t.Protocol)
}

func chatBody(model string) []byte {
	b, _ := json.Marshal(map[string]any{
		"model": model, "messages": []map[string]string{{"role": "user", "content": "hi"}}, "max_tokens": 16,
	})
	return b
}

func responsesBody(model string) []byte {
	b, _ := json.Marshal(map[string]any{
		"model": model,
		"input": []map[string]any{{"type": "message", "role": "user",
			"content": []map[string]string{{"type": "input_text", "text": "hi"}}}},
		"max_output_tokens": 16,
	})
	return b
}

func anthropicBody(model string) []byte {
	b, _ := json.Marshal(map[string]any{
		"model": model, "max_tokens": 16,
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	return b
}

func geminiBody(model string) []byte {
	b, _ := json.Marshal(map[string]any{
		"contents":         []map[string]any{{"role": "user", "parts": []map[string]string{{"text": "hi"}}}},
		"generationConfig": map[string]any{"maxOutputTokens": 16},
	})
	return b
}

func setProbeHeaders(req *http.Request, t ProbeTarget, key string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ARTEX-CredHunter/2.0")
	host := ""
	if u, err := url.Parse(t.Base); err == nil {
		host = strings.ToLower(u.Hostname())
	}
	switch t.Protocol {
	case ProtoAnthropic:
		req.Header.Set("anthropic-version", "2023-06-01")
		if host == "api.anthropic.com" || strings.Contains(host, "bedrock") || t.AuthHeader == "x-api-key" {
			req.Header.Set("x-api-key", key)
		} else if t.AuthHeader == "bearer" {
			req.Header.Set("Authorization", "Bearer "+key)
		} else {
			// Third-party Anthropic-compatible APIs differ: some take either,
			// some refuse both. Send both and let the 401 retry narrow it.
			req.Header.Set("x-api-key", key)
			req.Header.Set("Authorization", "Bearer "+key)
		}
	case ProtoGemini:
		req.Header.Set("x-goog-api-key", key)
	default:
		req.Header.Set("Authorization", "Bearer "+key)
	}
}

func bothAuthRefused(body []byte) bool {
	l := strings.ToLower(string(body))
	if !strings.Contains(l, "x-api-key") || !strings.Contains(l, "authorization") {
		return false
	}
	for _, s := range []string{"both", "together", "at the same time", "simultaneous", "mutually exclusive",
		"only one", "both present", "both provided", "同时", "并存", "共存", "只能", "不能"} {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

func isZhipuNoBalance(t ProbeTarget, status int, body []byte) bool {
	if status != http.StatusTooManyRequests {
		return false
	}
	u, err := url.Parse(t.Base)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host != "open.bigmodel.cn" && host != "api.z.ai" {
		return false
	}
	if !strings.Contains(u.Path, "/api/paas/v4") {
		return false
	}
	_, _, code := parseAPIError(body)
	return strings.Contains(string(body), "1113") || code == "1113" || strings.Contains(strings.ToLower(string(body)), "余额")
}

func validateProbeSuccess(contentType string, body []byte) (kind, detail, errType string) {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "event-stream") || bytes.HasPrefix(bytes.TrimSpace(body), []byte("data:")) {
		if msg, failed := scanProbeStream(body); failed {
			return "unknown", msg, "stream_error"
		}
		return "valid", "", ""
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return "valid", "", ""
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '<' {
		return "unknown", "endpoint returned HTML instead of JSON", "html"
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		// Some gateways answer 200 with plain text. It is not proof of a
		// working model, but it is not an auth refusal either.
		return "valid", "", ""
	}
	if success, ok := m["success"].(bool); ok && !success {
		msg, typ := errorMessage(m)
		if msg == "" {
			msg = "request failed"
		}
		return "invalid", msg, typ
	}
	if _, ok := m["error"]; ok && !hasProbePayload(m) {
		msg, typ := errorMessage(m)
		if msg == "" {
			msg = "error in response"
		}
		return "invalid", msg, typ
	}
	return "valid", "", ""
}

func hasProbePayload(m map[string]any) bool {
	for _, k := range []string{"choices", "content", "candidates", "output", "data", "result"} {
		if v, ok := m[k]; ok && v != nil {
			return true
		}
	}
	return false
}

func scanProbeStream(body []byte) (string, bool) {
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "" || data == "[DONE]" {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(data), &m) != nil {
			continue
		}
		if msg, _ := errorMessage(m); msg != "" {
			return msg, true
		}
		if typ, _ := m["type"].(string); typ == "response.failed" {
			return "response failed", true
		}
		if hasProbePayload(m) {
			return "", false
		}
	}
	return "", false
}

func classifyProbeFailure(status int, body []byte) (kind, detail, errType string, modelMissing bool) {
	msg, typ, code := parseAPIError(body)
	if msg == "" {
		msg = http.StatusText(status)
	}
	detail = fmt.Sprintf("%d %s", status, msg)
	text := strings.ToLower(msg + " " + typ + " " + code + " " + string(body))
	auth := isAuthText(text)
	quota := isQuotaText(text)
	model := isModelText(text)

	switch {
	case status == http.StatusPaymentRequired:
		return "no_balance", detail, typ, false
	case status == http.StatusTooManyRequests && (quota || code == "1113"):
		return "no_balance", detail, typ, false
	case status == http.StatusForbidden && quota:
		return "no_balance", detail, typ, false
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "invalid", detail, typ, false
	case status == http.StatusTooManyRequests:
		return "rate_limited", detail, typ, false
	case auth:
		return "invalid", detail, typ, false
	case model:
		return "model_missing", detail, typ, true
	case status == http.StatusNotFound || status == http.StatusMethodNotAllowed:
		return "endpoint_missing", detail, typ, false
	case quota:
		return "no_balance", detail, typ, false
	case status >= 500:
		return "unknown", detail, typ, false
	default:
		return "unknown", detail, typ, false
	}
}

func isAuthText(s string) bool {
	for _, x := range []string{"invalid api key", "incorrect api key", "api key not valid", "invalid_api_key",
		"authentication_error", "authentication fail", "unauthorized", "invalid token", "invalid_token",
		"permission_denied", "account_deactivated", "account disabled", "身份验证失败", "无效的令牌", "鉴权失败", "未授权"} {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

func isQuotaText(s string) bool {
	for _, x := range []string{"insufficient_quota", "insufficient quota", "quota exceeded", "quota_exceeded",
		"insufficient balance", "insufficient_balance", "no balance", "not enough balance", "balance is not enough",
		"billing", "payment required", "credit", "exceeded your current quota", "key limit exceeded",
		"total limit", "1113", "余额", "欠费", "用尽", "额度不足"} {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

func isModelText(s string) bool {
	for _, x := range []string{"model_not_found", "model not found", "no such model", "invalid model", "unknown model",
		"model does not exist", "does not exist", "unsupported model", "model is not available", "model is unavailable",
		"model unavailable", "unavailable for free", "use this slug instead", "no available channel",
		"模型不存在", "模型不可用", "无可用渠道"} {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

func parseAPIError(body []byte) (msg, typ, code string) {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		s := strings.Join(strings.Fields(string(body)), " ")
		if s == "" || strings.HasPrefix(s, "<") {
			return "", "", ""
		}
		if len([]rune(s)) > 300 {
			s = string([]rune(s)[:300]) + "…"
		}
		return s, "", ""
	}
	if success, ok := m["success"].(bool); ok && !success {
		msg = firstMapString(m, "msg", "message")
		typ, code = firstMapString(m, "type"), firstMapString(m, "code")
		return
	}
	msg, typ = errorMessage(m)
	if msg == "" {
		msg = firstMapString(m, "message", "msg")
	}
	if typ == "" {
		typ = firstMapString(m, "type")
	}
	if code == "" {
		code = firstMapString(m, "code")
	}
	return
}

func errorMessage(m map[string]any) (msg, typ string) {
	raw, ok := m["error"]
	if !ok {
		return "", ""
	}
	switch e := raw.(type) {
	case string:
		return strings.TrimSpace(e), ""
	case map[string]any:
		msg = firstMapString(e, "message", "msg", "detail")
		typ = firstMapString(e, "type")
		if typ == "" {
			typ = firstMapString(e, "code")
		}
		if msg == "" && e["message"] != nil {
			msg = fmt.Sprint(e["message"])
		}
	}
	if msg == "" {
		if arr, ok := m["errors"].([]any); ok && len(arr) > 0 {
			if one, ok := arr[0].(map[string]any); ok {
				msg = firstMapString(one, "message")
			}
		}
	}
	if msg == "" {
		msg = firstMapString(m, "message", "msg")
	}
	return
}

func firstMapString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		switch v := m[k].(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		case float64:
			return fmt.Sprintf("%.0f", v)
		case json.Number:
			return v.String()
		}
	}
	return ""
}

// probeState is kept per verification to avoid repeating a model list fetch
// for every target of the same credential.
type probeState struct {
	mu     sync.Mutex
	models map[string][]string
}
