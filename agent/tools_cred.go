package agent

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Autumn-27/artex/credhunt"
	actool "github.com/Autumn-27/norma/tool"
)

// CredServiceProvider returns the process-wide credential-hunting service. The
// server wires it in server.go; nil means the tools report a clear error.
var CredServiceProvider func() *credhunt.Service

func credService() *credhunt.Service {
	if CredServiceProvider == nil {
		return nil
	}
	return CredServiceProvider()
}

// CredTools returns the credential-hunting domain tools. They are bound to the
// credhunter agent by default and can be re-bound in the tools page.
func (t *ToolSet) CredTools() []actool.CoreTool {
	return []actool.CoreTool{
		t.huntBatch(), t.huntFetch(), t.credSubmit(), t.credList(),
		t.credVerify(), t.proxySubmit(), t.proxyProbe(), t.huntReport(),
	}
}

func (t *ToolSet) huntBatch() actool.CoreTool {
	return writeTool("hunt_batch",
		"批量扫描授权目标的目录列表：只读 GET，识别 Apache/nginx/Python/IIS/S3 列目录，按凭据检查清单给候选文件打分。返回每个目标的候选文件（不含文件内容）与证据路径。必须填写 authorization_note（授权来源/范围）。不递归整站，不抓文件内容。",
		obj(map[string]any{
			"urls": map[string]any{"type": "array", "items": map[string]any{"type": "string"},
				"description": "目录/首页 URL 列表，单次最多 20 个"},
			"authorization_note": str("授权来源与范围（必填，写入审计记录）"),
			"max_entries":        intp("每个目标最多返回多少条候选，默认 60，上限 200"),
			"run_note":           str("本次扫描说明（可选）"),
		}, "urls", "authorization_note"),
		func(ctx context.Context, raw json.RawMessage) (actool.Result, error) {
			svc := credService()
			if svc == nil {
				return actool.Errorf("credhunt service unavailable"), nil
			}
			var in struct {
				URLs              []string `json:"urls"`
				AuthorizationNote string   `json:"authorization_note"`
				MaxEntries        int      `json:"max_entries"`
				RunNote           string   `json:"run_note"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return actool.Errorf("invalid input: " + err.Error()), nil
			}
			if strings.TrimSpace(in.AuthorizationNote) == "" {
				return actool.Errorf("authorization_note is required"), nil
			}
			if len(in.URLs) == 0 {
				return actool.Errorf("urls is empty"), nil
			}
			if len(in.URLs) > 20 {
				in.URLs = in.URLs[:20]
			}
			note := strings.TrimSpace(in.RunNote)
			if note == "" {
				note = in.AuthorizationNote
			}
			runID, targets := svc.Batch(ctx, in.URLs, in.MaxEntries, note, t.taskID)
			listing := 0
			for _, target := range targets {
				if target.IsListing {
					listing++
				}
			}
			return jsonResult(map[string]any{"run_id": runID, "targets": targets,
				"totals": map[string]any{"targets": len(targets), "listings": listing}})
		})
}

func (t *ToolSet) huntFetch() actool.CoreTool {
	return writeTool("hunt_fetch",
		"只读抓取一个 HTTP(S) 文件/页面并落证据（2MB 上限、SSRF 拦截私网、最多 3 次跳转）。返回状态码、类型、大小、SHA256、证据路径和最多 8KB 文本。不要抓二进制/静态资源；不要用它扫端口。",
		obj(map[string]any{
			"url":                str("完整 http(s) URL"),
			"authorization_note": str("授权来源与范围（必填）"),
			"run_id":             str("可选：沿用 hunt_batch 返回的 run_id"),
		}, "url", "authorization_note"),
		func(ctx context.Context, raw json.RawMessage) (actool.Result, error) {
			svc := credService()
			if svc == nil {
				return actool.Errorf("credhunt service unavailable"), nil
			}
			var in struct {
				URL               string `json:"url"`
				AuthorizationNote string `json:"authorization_note"`
				RunID             string `json:"run_id"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return actool.Errorf("invalid input: " + err.Error()), nil
			}
			if strings.TrimSpace(in.AuthorizationNote) == "" {
				return actool.Errorf("authorization_note is required"), nil
			}
			runID := strings.TrimSpace(in.RunID)
			if runID == "" {
				runID = "adhoc"
			}
			res := svc.Fetch(ctx, in.URL, runID)
			return jsonResult(res)
		})
}

func (t *ToolSet) credSubmit() actool.CoreTool {
	return writeTool("cred_submit",
		"把 AI 已判定的凭据写入加密台账：自动分类厂商、跨目标指纹去重、拒绝占位符/截断值/IM 机器人 Token。每条必须带 source_url + evidence_path。OAuth/令牌包请把完整 JSON 放在 package（或 value）里，禁止只取一行。返回 inserted/duplicate/excluded/skipped 与掩码后的 uid。",
		obj(map[string]any{
			"entries": map[string]any{"type": "array", "items": obj(map[string]any{
				"value":         str("凭据原文（单值）；若为整包请用 package"),
				"package":       map[string]any{"type": "object", "description": "OAuth/令牌整包 JSON（可替代 value）"},
				"kind":          str("llm_key|oauth|api_key|token|credential|private_key|proxy_node|proxy_subscription|other（可留空自动判断）"),
				"provider":      str("厂商 id（可留空自动分类）"),
				"service":       str("服务名/来源描述，如 OpenRouter、x-ui、example.com"),
				"source_url":    str("证据来源 URL（必填）"),
				"evidence_path": str("本地证据文件路径（必填）"),
				"risk":          str("P0/P1/P2/P3（可留空）"),
				"context":       str("上下文/判断依据（可选）"),
				"base_url":      str("中转/未知厂商的 API 根地址（可选）"),
				"asset_id":      intp("关联资产 id（可选）"),
			}, "source_url", "evidence_path")},
			"dry_run": map[string]any{"type": "boolean", "description": "只校验不写库"},
		}, "entries"),
		func(ctx context.Context, raw json.RawMessage) (actool.Result, error) {
			svc := credService()
			if svc == nil {
				return actool.Errorf("credhunt service unavailable"), nil
			}
			var in struct {
				Entries []credhunt.Input `json:"entries"`
				DryRun  bool             `json:"dry_run"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return actool.Errorf("invalid input: " + err.Error()), nil
			}
			for i := range in.Entries {
				if in.Entries[i].TaskID == 0 {
					in.Entries[i].TaskID = t.taskID
				}
			}
			if in.DryRun {
				return jsonResult(map[string]any{"dry_run": true, "entries": len(in.Entries)})
			}
			res, err := svc.Submit(ctx, in.Entries)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return jsonResult(res)
		})
}

func (t *ToolSet) credList() actool.CoreTool {
	return readTool("cred_list",
		"查询加密凭据台账（默认只返回掩码）。可按类型/厂商/状态/任务/关键词过滤，用于查重、汇报和挑选待验活项。",
		obj(map[string]any{
			"kind":     str("llm_key|oauth|api_key|token|private_key|proxy_node|proxy_subscription|other"),
			"provider": str("厂商 id"),
			"status":   str("unverified|valid|invalid|no_balance|rate_limited|unknown|unsupported"),
			"q":        str("关键词（服务/厂商/来源/上下文）"),
			"task_id":  intp("按任务过滤（默认当前任务）"),
			"limit":    intp("返回条数，默认 50，上限 200"),
		}),
		func(ctx context.Context, raw json.RawMessage) (actool.Result, error) {
			svc := credService()
			if svc == nil {
				return actool.Errorf("credhunt service unavailable"), nil
			}
			var in struct {
				Kind     string `json:"kind"`
				Provider string `json:"provider"`
				Status   string `json:"status"`
				Q        string `json:"q"`
				TaskID   int64  `json:"task_id"`
				Limit    int    `json:"limit"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return actool.Errorf("invalid input: " + err.Error()), nil
			}
			f := credhunt.Filter{Provider: in.Provider, Status: in.Status, Query: in.Q,
				TaskID: in.TaskID, Limit: in.Limit}
			if f.TaskID == 0 {
				f.TaskID = t.taskID
			}
			if strings.TrimSpace(in.Kind) != "" {
				f.Kinds = []string{strings.TrimSpace(in.Kind)}
			}
			items, total, err := svc.Store.List(ctx, f)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return jsonResult(map[string]any{"total": total, "items": items})
		})
}

func (t *ToolSet) credVerify() actool.CoreTool {
	return writeTool("cred_verify",
		"对 LLM Key / OAuth 包做最小验活：优先 GET /models（不消耗生成额度），不可用才发 max_tokens=1 的 hi 请求；成功后再查余额（支持的厂商）。401/403 判失效，失败不重试。密码/私钥/代理节点不走这里。",
		obj(map[string]any{
			"uids":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "指定凭据 uid（最多 20）"},
			"kind":     str("不指定 uids 时按类型筛选"),
			"provider": str("不指定 uids 时按厂商筛选"),
			"status":   str("不指定 uids 时按状态筛选，默认 unverified"),
			"limit":    intp("批量上限，默认 20，最大 50"),
			"dry_run":  map[string]any{"type": "boolean", "description": "只统计匹配数量，不发起请求"},
		}),
		func(ctx context.Context, raw json.RawMessage) (actool.Result, error) {
			svc := credService()
			if svc == nil {
				return actool.Errorf("credhunt service unavailable"), nil
			}
			var in struct {
				UIDs     []string `json:"uids"`
				Kind     string   `json:"kind"`
				Provider string   `json:"provider"`
				Status   string   `json:"status"`
				Limit    int      `json:"limit"`
				DryRun   bool     `json:"dry_run"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return actool.Errorf("invalid input: " + err.Error()), nil
			}
			if len(in.UIDs) > 20 {
				in.UIDs = in.UIDs[:20]
			}
			if in.DryRun {
				f := credhunt.Filter{Provider: in.Provider, Status: in.Status, Limit: 1}
				if in.Kind != "" {
					f.Kinds = []string{in.Kind}
				}
				_, total, err := svc.Store.List(ctx, f)
				if err != nil {
					return actool.Errorf(err.Error()), nil
				}
				return jsonResult(map[string]any{"dry_run": true, "matched": total})
			}
			var results []credhunt.VerifyResult
			if len(in.UIDs) > 0 {
				results = svc.Verify(ctx, in.UIDs)
			} else {
				status := in.Status
				if status == "" {
					status = credhunt.StatusUnverified
				}
				f := credhunt.Filter{Provider: in.Provider, Status: status}
				if in.Kind != "" {
					f.Kinds = []string{in.Kind}
				}
				results = svc.VerifyFilter(ctx, f, in.Limit)
			}
			return jsonResult(map[string]any{"count": len(results), "results": results})
		})
}

func (t *ToolSet) proxySubmit() actool.CoreTool {
	return writeTool("proxy_submit",
		"登记代理节点/订阅：支持 vless/vmess/trojan/ss/hysteria2/tuic/anytls/socks5/http 分享链接、WireGuard conf、Clash/mihomo YAML、base64 订阅、SS CSV、user:pass:host:port 列表，以及订阅 URL（服务端只读拉取）。订阅会展开为子节点。",
		obj(map[string]any{
			"entries": map[string]any{"type": "array", "items": obj(map[string]any{
				"value":         str("分享链接/订阅内容/订阅 URL"),
				"service":       str("来源描述（可选）"),
				"source_url":    str("证据来源 URL（必填）"),
				"evidence_path": str("本地证据文件路径（必填）"),
				"context":       str("上下文（可选）"),
				"parent_uid":    str("父订阅 uid（可选）"),
			}, "value", "source_url", "evidence_path")},
			"expand":  map[string]any{"type": "boolean", "description": "展开订阅为子节点，默认 true"},
			"dry_run": map[string]any{"type": "boolean", "description": "只解析不写库"},
		}, "entries"),
		func(ctx context.Context, raw json.RawMessage) (actool.Result, error) {
			svc := credService()
			if svc == nil {
				return actool.Errorf("credhunt service unavailable"), nil
			}
			var in struct {
				Entries []credhunt.Input `json:"entries"`
				Expand  *bool            `json:"expand"`
				DryRun  bool             `json:"dry_run"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return actool.Errorf("invalid input: " + err.Error()), nil
			}
			expand := true
			if in.Expand != nil {
				expand = *in.Expand
			}
			if in.DryRun {
				type preview struct {
					Value string `json:"value"`
					Count int    `json:"count"`
				}
				var out []preview
				for _, e := range in.Entries {
					parsed := credhunt.ParseProxyContent(e.Value)
					out = append(out, preview{Value: maskURL(e.Value), Count: len(parsed)})
				}
				return jsonResult(map[string]any{"dry_run": true, "entries": out})
			}
			for i := range in.Entries {
				if in.Entries[i].TaskID == 0 {
					in.Entries[i].TaskID = t.taskID
				}
				if in.Entries[i].Kind == "" {
					in.Entries[i].Kind = credhunt.KindProxyNode
				}
			}
			res, err := svc.SubmitProxies(ctx, in.Entries, expand)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return jsonResult(res)
		})
}

func (t *ToolSet) proxyProbe() actool.CoreTool {
	return writeTool("proxy_probe",
		"用独立 mihomo sidecar（不碰生产代理）对代理节点做延迟探测：只访问两个 204 URL，并发 4，5 秒超时。返回 valid/invalid/error 与延迟。",
		obj(map[string]any{
			"uids":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "指定节点 uid（最多 20）"},
			"limit": intp("不指定 uids 时探测最近多少个，默认 20，最大 50"),
		}),
		func(ctx context.Context, raw json.RawMessage) (actool.Result, error) {
			svc := credService()
			if svc == nil {
				return actool.Errorf("credhunt service unavailable"), nil
			}
			var in struct {
				UIDs  []string `json:"uids"`
				Limit int      `json:"limit"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return actool.Errorf("invalid input: " + err.Error()), nil
			}
			if len(in.UIDs) > 20 {
				in.UIDs = in.UIDs[:20]
			}
			results := svc.Probe(ctx, in.UIDs, in.Limit)
			return jsonResult(map[string]any{"count": len(results), "results": results})
		})
}

func (t *ToolSet) huntReport() actool.CoreTool {
	return readTool("hunt_report",
		"汇总本次/本任务的凭据挖掘结果：按类型与状态计数、有效项清单（掩码）、代理节点存活情况。用于收尾汇报，不返回明文。",
		obj(map[string]any{
			"task_id": intp("任务 id（默认当前任务）"),
			"limit":   intp("明细条数，默认 50"),
		}),
		func(ctx context.Context, raw json.RawMessage) (actool.Result, error) {
			svc := credService()
			if svc == nil {
				return actool.Errorf("credhunt service unavailable"), nil
			}
			var in struct {
				TaskID int64 `json:"task_id"`
				Limit  int   `json:"limit"`
			}
			if err := json.Unmarshal(raw, &in); err != nil {
				return actool.Errorf("invalid input: " + err.Error()), nil
			}
			if in.TaskID == 0 {
				in.TaskID = t.taskID
			}
			stats, err := svc.Store.Stats(ctx)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			f := credhunt.Filter{TaskID: in.TaskID, Limit: in.Limit}
			items, total, err := svc.Store.List(ctx, f)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return jsonResult(map[string]any{"stats": stats, "task_id": in.TaskID,
				"total": total, "items": items})
		})
}

func maskURL(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 120 {
		return v[:117] + "..."
	}
	return v
}
