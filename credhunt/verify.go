package credhunt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Verifier performs minimal, read-only LLM credential checks.
type Verifier struct {
	store  *Store
	client *http.Client
}

// NewVerifier builds a verifier with a direct (non-proxy) HTTP client. The
// server's TUN/mihomo handles egress at the OS level.
func NewVerifier(store *Store) *Verifier {
	return &Verifier{store: store, client: &http.Client{Timeout: 12 * time.Second,
		Transport: &http.Transport{Proxy: nil, TLSHandshakeTimeout: 8 * time.Second}}}
}

// Verify checks entries concurrently and persists each verdict.
func (v *Verifier) Verify(ctx context.Context, entries []Entry, secrets []string) []VerifyResult {
	results := make([]VerifyResult, len(entries))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range entries {
		if i >= len(secrets) {
			break
		}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = v.verifyOne(ctx, entries[idx], secrets[idx])
			_ = v.store.UpdateVerification(ctx, results[idx].UID, results[idx].Status, results[idx].Detail, results[idx].Quota)
		}(i)
	}
	wg.Wait()
	return results
}

func (v *Verifier) verifyOne(ctx context.Context, e Entry, secret string) (res VerifyResult) {
	start := time.Now()
	res = VerifyResult{UID: e.UID, Kind: e.Kind, Provider: e.Provider, Service: e.Service,
		Status: StatusUnsupported, CheckedAt: start}
	defer func() { res.ElapsedMS = time.Since(start).Milliseconds() }()

	provider, ok := ProviderByID(e.Provider)
	base := strings.TrimSpace(asString(e.Extra["base_url"]))
	if !ok {
		if base == "" {
			res.Detail = "unknown provider and no base_url"
			return res
		}
		provider = Provider{ID: "relay", Name: "relay", Class: "key", Style: "openai", Base: base}
	}
	if base == "" {
		base = provider.Base
	}
	if base == "" {
		res.Detail = "provider has no API base"
		return res
	}
	res.Model = provider.Model

	if provider.Style == "oauth" {
		return v.verifyOAuth(ctx, res, provider, secret)
	}
	if provider.Class == "oauth" {
		if tok := extractOAuthToken(secret); tok != "" {
			secret = tok
		}
	}
	if e.Kind == KindPrivateKey || e.Kind == KindPassword || e.Kind == KindProxyNode || e.Kind == KindProxySubscription {
		res.Detail = "only LLM keys / OAuth packages are actively verified"
		return res
	}

	bases := append([]string{base}, provider.BaseFallbacks...)
	var lastDetail string
	for _, b := range bases {
		status, detail, httpStatus, endpoint, valid := v.tryModels(ctx, provider, b, secret)
		if status != "" {
			res.Status, res.Detail, res.HTTPStatus, res.Endpoint = status, detail, httpStatus, endpoint
			if valid {
				res.Quota = v.fetchQuota(ctx, provider, b, secret)
				return res
			}
			if status == StatusInvalid || status == StatusNoBalance || status == StatusRateLimited {
				return res
			}
			lastDetail = detail
			continue
		}
		// models endpoint unavailable → minimal chat
		status, detail, httpStatus, endpoint, valid = v.tryChat(ctx, provider, b, secret)
		if status != "" {
			res.Status, res.Detail, res.HTTPStatus, res.Endpoint = status, detail, httpStatus, endpoint
			if valid {
				res.Quota = v.fetchQuota(ctx, provider, b, secret)
				return res
			}
			if status == StatusInvalid || status == StatusNoBalance || status == StatusRateLimited {
				return res
			}
			lastDetail = detail
		}
	}
	if lastDetail == "" {
		lastDetail = "no usable endpoint"
	}
	if res.Status == StatusUnsupported {
		res.Status = StatusUnknown
	}
	res.Detail = lastDetail
	return res
}

func (v *Verifier) verifyOAuth(ctx context.Context, res VerifyResult, p Provider, secret string) VerifyResult {
	token := extractOAuthToken(secret)
	if token == "" {
		res.Status = StatusUnsupported
		res.Detail = "OAuth package has no access_token"
		return res
	}
	base := p.Endpoint
	if base == "" {
		base = p.Base
	}
	if base == "" {
		res.Status = StatusUnsupported
		res.Detail = "OAuth provider has no usage endpoint"
		return res
	}
	res.Endpoint = base
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base, nil)
	if err != nil {
		res.Status = StatusUnknown
		res.Detail = err.Error()
		return res
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "ARTEX-CredHunter/1.0")
	req.Header.Set("Accept", "application/json")
	if p.ID == "claude_oauth" {
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	resp, err := v.client.Do(req)
	if err != nil {
		res.Status = StatusUnknown
		res.Detail = "network: " + err.Error()
		return res
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	res.HTTPStatus = resp.StatusCode
	res.Quota = map[string]any{"source": base, "raw": truncate(string(body), 4000)}
	switch {
	case resp.StatusCode == 200:
		res.Status = StatusValid
		res.Detail = "OAuth usage endpoint accepted the token"
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		res.Status = StatusInvalid
		res.Detail = fmt.Sprintf("%d token rejected", resp.StatusCode)
	case resp.StatusCode == 429:
		res.Status = StatusRateLimited
		res.Detail = "429 rate limited"
	default:
		res.Status = StatusUnknown
		res.Detail = fmt.Sprintf("%d unexpected response", resp.StatusCode)
	}
	return res
}

func (v *Verifier) tryModels(ctx context.Context, p Provider, base, key string) (status, detail string, httpStatus int, endpoint string, valid bool) {
	if p.ID == "relay" {
		// Unknown relays often expose /models without auth; force the 1-token chat.
		return "", "", 0, "", false
	}
	switch p.Style {
	case "anthropic":
		endpoint = strings.TrimRight(base, "/") + "/models"
	case "gemini":
		endpoint = strings.TrimRight(base, "/") + "/models?key=" + urlQueryEscape(key)
	default:
		endpoint = strings.TrimRight(base, "/") + "/models"
	}
	if p.ID == "openrouter" {
		// OpenRouter's /models is public; /credits requires the key and is free.
		endpoint = strings.TrimRight(base, "/") + "/credits"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return StatusUnknown, err.Error(), 0, endpoint, false
	}
	switch p.Style {
	case "anthropic":
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	case "gemini":
		// key is in the query string
	default:
		req.Header.Set("Authorization", "Bearer "+key)
	}
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("User-Agent", "ARTEX-CredHunter/1.0")
	req.Header.Set("Accept", "application/json")
	resp, err := v.client.Do(req)
	if err != nil {
		return StatusUnknown, "network: " + err.Error(), 0, endpoint, false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	httpStatus = resp.StatusCode
	switch {
	case resp.StatusCode == 200:
		okDetail := "GET /models accepted the key"
		if p.ID == "openrouter" {
			okDetail = "GET /credits accepted the key"
		}
		return StatusValid, okDetail, httpStatus, endpoint, true
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return StatusInvalid, fmt.Sprintf("%d key rejected", resp.StatusCode), httpStatus, endpoint, false
	case resp.StatusCode == 402:
		return StatusNoBalance, "402 payment required", httpStatus, endpoint, false
	case resp.StatusCode == 429:
		return StatusRateLimited, "429 rate limited", httpStatus, endpoint, false
	case resp.StatusCode == 404 || resp.StatusCode == 405:
		return "", "", httpStatus, endpoint, false // try chat
	case resp.StatusCode >= 500:
		return StatusUnknown, fmt.Sprintf("%d server error", resp.StatusCode), httpStatus, endpoint, false
	default:
		return "", fmt.Sprintf("%d endpoint unavailable: %s", resp.StatusCode, truncate(string(body), 160)), httpStatus, endpoint, false
	}
}

func (v *Verifier) tryChat(ctx context.Context, p Provider, base, key string) (status, detail string, httpStatus int, endpoint string, valid bool) {
	model := p.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	var body []byte
	switch p.Style {
	case "anthropic":
		endpoint = strings.TrimRight(base, "/") + "/messages"
		body, _ = json.Marshal(map[string]any{"model": model, "max_tokens": 1,
			"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	case "gemini":
		endpoint = strings.TrimRight(base, "/") + "/models/" + model + ":generateContent?key=" + urlQueryEscape(key)
		body, _ = json.Marshal(map[string]any{"contents": []map[string]any{{"parts": []map[string]string{{"text": "hi"}}}},
			"generationConfig": map[string]any{"maxOutputTokens": 1}})
	default:
		endpoint = strings.TrimRight(base, "/") + "/chat/completions"
		body, _ = json.Marshal(map[string]any{"model": model, "max_tokens": 1, "stream": false,
			"messages": []map[string]string{{"role": "user", "content": "hi"}}})
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return StatusUnknown, err.Error(), 0, endpoint, false
	}
	req.Header.Set("Content-Type", "application/json")
	switch p.Style {
	case "anthropic":
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	case "gemini":
	default:
		req.Header.Set("Authorization", "Bearer "+key)
	}
	for k, val := range p.Headers {
		req.Header.Set(k, val)
	}
	req.Header.Set("User-Agent", "ARTEX-CredHunter/1.0")
	resp, err := v.client.Do(req)
	if err != nil {
		return StatusUnknown, "network: " + err.Error(), 0, endpoint, false
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	httpStatus = resp.StatusCode
	switch {
	case resp.StatusCode == 200:
		return StatusValid, fmt.Sprintf("minimal chat accepted (%s)", model), httpStatus, endpoint, true
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return StatusInvalid, fmt.Sprintf("%d key rejected", resp.StatusCode), httpStatus, endpoint, false
	case resp.StatusCode == 402:
		return StatusNoBalance, "402 balance exhausted", httpStatus, endpoint, false
	case resp.StatusCode == 429:
		return StatusRateLimited, "429 rate limited", httpStatus, endpoint, false
	case resp.StatusCode >= 500:
		return StatusUnknown, fmt.Sprintf("%d server error", resp.StatusCode), httpStatus, endpoint, false
	default:
		return StatusUnknown, fmt.Sprintf("%d rejected: %s", resp.StatusCode, truncate(string(respBody), 160)), httpStatus, endpoint, false
	}
}

func (v *Verifier) fetchQuota(ctx context.Context, p Provider, base, key string) map[string]any {
	var endpoint string
	switch p.Quota {
	case "deepseek_balance":
		endpoint = strings.TrimRight(base, "/") + "/user/balance"
	case "moonshot_balance":
		endpoint = strings.TrimRight(base, "/") + "/users/me/balance"
	case "siliconflow_user":
		endpoint = strings.TrimRight(base, "/") + "/user/info"
	case "openrouter_key":
		endpoint = strings.TrimRight(base, "/") + "/credits"
	default:
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "ARTEX-CredHunter/1.0")
	req.Header.Set("Accept", "application/json")
	resp, err := v.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode != 200 {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(body, &out) != nil {
		return map[string]any{"source": endpoint, "raw": truncate(string(body), 2000)}
	}
	out["source"] = endpoint
	out["summary"] = quotaSummary(p.Quota, out)
	return out
}

func quotaSummary(kind string, data map[string]any) string {
	switch kind {
	case "openrouter_key":
		total, _ := data["data"].(map[string]any)
		if total != nil {
			if v, ok := total["total_credits"]; ok {
				return fmt.Sprintf("credits=%v usage=%v", v, total["total_usage"])
			}
		}
	case "deepseek_balance":
		if infos, ok := data["balance_infos"].([]any); ok && len(infos) > 0 {
			if one, ok := infos[0].(map[string]any); ok {
				return fmt.Sprintf("balance=%v %v", one["total_balance"], one["currency"])
			}
		}
	}
	return "quota retrieved"
}

func extractOAuthToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "sk-ant-oat") || strings.HasPrefix(raw, "gho_") || strings.HasPrefix(raw, "ghu_") {
		return raw
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return ""
	}
	for _, k := range []string{"access_token", "accessToken", "token", "id_token"} {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	if tokens, ok := m["tokens"].(map[string]any); ok {
		for _, k := range []string{"access_token", "accessToken", "id_token"} {
			if v, ok := tokens[k].(string); ok && v != "" {
				return v
			}
		}
	}
	return ""
}

func urlQueryEscape(s string) string {
	return url.QueryEscape(s)
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
