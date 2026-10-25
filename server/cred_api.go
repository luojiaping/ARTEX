package server

import (
	"encoding/csv"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/Autumn-27/artex/credhunt"
)

// credSvc returns the process-wide credential service or writes a 503.
func (s *Server) credSvc(w http.ResponseWriter) *credhunt.Service {
	svc := s.m.Cred()
	if svc == nil {
		writeErr(w, 503, "凭据服务未初始化（检查 data/credhunt.key 与数据库）")
		return nil
	}
	return svc
}

func credFilterFrom(r *http.Request) credhunt.Filter {
	q := r.URL.Query()
	f := credhunt.Filter{
		PClass:         strings.TrimSpace(q.Get("pclass")),
		Provider:       strings.TrimSpace(q.Get("provider")),
		Variant:        strings.TrimSpace(q.Get("variant")),
		ServiceFamily:  strings.TrimSpace(q.Get("service_family")),
		Protocol:       strings.TrimSpace(q.Get("protocol")),
		Status:         strings.TrimSpace(q.Get("status")),
		Risk:           strings.TrimSpace(q.Get("risk")),
		Query:          strings.TrimSpace(q.Get("q")),
		QuotaSupported: q.Get("quota_supported") == "1" || q.Get("quota_supported") == "true",
		Manual:         q.Get("manual") == "1" || q.Get("manual") == "true",
		Archived:       q.Get("archived") == "1" || q.Get("archived") == "true",
	}
	if v := strings.TrimSpace(q.Get("kind")); v != "" {
		for _, one := range strings.Split(v, ",") {
			if one = strings.TrimSpace(one); one != "" {
				f.Kinds = append(f.Kinds, one)
			}
		}
	}
	if v := strings.TrimSpace(q.Get("task_id")); v != "" {
		f.TaskID, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		f.Limit, _ = strconv.Atoi(v)
	}
	if v := strings.TrimSpace(q.Get("offset")); v != "" {
		f.Offset, _ = strconv.Atoi(v)
	}
	return f
}

// GET /api/credentials
func (s *Server) credList(w http.ResponseWriter, r *http.Request) {
	svc := s.credSvc(w)
	if svc == nil {
		return
	}
	f := credFilterFrom(r)
	items, total, err := svc.Store.List(r.Context(), f)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"total": total, "items": items})
}

// GET /api/credentials/stats
func (s *Server) credStats(w http.ResponseWriter, r *http.Request) {
	svc := s.credSvc(w)
	if svc == nil {
		return
	}
	stats, err := svc.Store.Stats(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, stats)
}

// GET /api/credentials/facets
func (s *Server) credFacets(w http.ResponseWriter, r *http.Request) {
	svc := s.credSvc(w)
	if svc == nil {
		return
	}
	facets, err := svc.Store.Facets(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, facets)
}

// GET /api/credentials/providers
func (s *Server) credProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"items": credhunt.ProviderRegistry()})
}

// POST /api/credentials/classify
func (s *Server) credClassify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind      string `json:"kind"`
		Value     string `json:"value"`
		Service   string `json:"service"`
		Context   string `json:"context"`
		SourceURL string `json:"source_url"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, credhunt.ClassifyDetailed(req.Kind, req.Value, req.Service, req.Context, req.SourceURL))
}

// POST /api/credentials/{uid}/status
func (s *Server) credStatus(w http.ResponseWriter, r *http.Request) {
	svc := s.credSvc(w)
	if svc == nil {
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len([]rune(req.Status)) > 32 {
		writeErr(w, 400, "手动状态最长 32 字")
		return
	}
	uid := r.PathValue("uid")
	if err := svc.Store.SetManualStatus(r.Context(), uid, req.Status); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	entry, _, err := svc.Store.Get(r.Context(), uid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, entry)
}

// POST /api/credentials/{uid}/variant
func (s *Server) credVariant(w http.ResponseWriter, r *http.Request) {
	svc := s.credSvc(w)
	if svc == nil {
		return
	}
	var req struct {
		Variant string `json:"variant"`
		BaseURL string `json:"base_url"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	uid := r.PathValue("uid")
	if err := svc.Store.SetVariant(r.Context(), uid, strings.TrimSpace(req.Variant), req.BaseURL); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	entry, _, err := svc.Store.Get(r.Context(), uid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, entry)
}

// GET /api/credentials/{uid}/probes
func (s *Server) credProbes(w http.ResponseWriter, r *http.Request) {
	svc := s.credSvc(w)
	if svc == nil {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := svc.Store.Probes(r.Context(), r.PathValue("uid"), limit)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

// GET /api/credentials/{uid}
func (s *Server) credGet(w http.ResponseWriter, r *http.Request) {
	svc := s.credSvc(w)
	if svc == nil {
		return
	}
	uid := r.PathValue("uid")
	entry, sources, err := svc.Store.Get(r.Context(), uid)
	if err != nil {
		writeErr(w, 404, "凭据不存在: "+err.Error())
		return
	}
	probes, _ := svc.Store.Probes(r.Context(), uid, 20)
	writeJSON(w, 200, map[string]any{"entry": entry, "sources": sources, "probes": probes})
}

// POST /api/credentials/{uid}/reveal  {"confirm":true}
func (s *Server) credReveal(w http.ResponseWriter, r *http.Request) {
	svc := s.credSvc(w)
	if svc == nil {
		return
	}
	var req struct {
		Confirm bool `json:"confirm"`
	}
	_ = decode(r, &req)
	if !req.Confirm {
		writeErr(w, 400, "需要 confirm=true 才能查看明文")
		return
	}
	uid := r.PathValue("uid")
	value, err := svc.Store.Reveal(r.Context(), uid)
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	log.Printf("[credhunt] reveal uid=%s", uid)
	writeJSON(w, 200, map[string]any{"uid": uid, "value": value})
}

// POST /api/credentials/{uid}/verify
func (s *Server) credVerify(w http.ResponseWriter, r *http.Request) {
	svc := s.credSvc(w)
	if svc == nil {
		return
	}
	uid := r.PathValue("uid")
	results := svc.Verify(r.Context(), []string{uid})
	if len(results) == 0 {
		writeErr(w, 404, "凭据不存在或无法验活")
		return
	}
	writeJSON(w, 200, results[0])
}

// POST /api/credentials/verify
func (s *Server) credVerifyBatch(w http.ResponseWriter, r *http.Request) {
	svc := s.credSvc(w)
	if svc == nil {
		return
	}
	var req struct {
		UIDs     []string `json:"uids"`
		Kind     string   `json:"kind"`
		Provider string   `json:"provider"`
		Status   string   `json:"status"`
		Limit    int      `json:"limit"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(req.UIDs) > 50 {
		req.UIDs = req.UIDs[:50]
	}
	var results []credhunt.VerifyResult
	if len(req.UIDs) > 0 {
		results = svc.Verify(r.Context(), req.UIDs)
	} else {
		status := req.Status
		if status == "" {
			status = credhunt.StatusUnverified
		}
		f := credhunt.Filter{Provider: req.Provider, Status: status}
		if req.Kind != "" {
			f.Kinds = []string{req.Kind}
		}
		results = svc.VerifyFilter(r.Context(), f, req.Limit)
	}
	writeJSON(w, 200, map[string]any{"count": len(results), "results": results})
}

// POST /api/credentials/{uid}/archive
func (s *Server) credArchive(w http.ResponseWriter, r *http.Request) {
	svc := s.credSvc(w)
	if svc == nil {
		return
	}
	if err := svc.Store.Archive(r.Context(), r.PathValue("uid")); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/credentials/export?format=csv
func (s *Server) credExport(w http.ResponseWriter, r *http.Request) {
	svc := s.credSvc(w)
	if svc == nil {
		return
	}
	f := credFilterFrom(r)
	f.Limit = 1000
	f.Offset = 0
	items, _, err := svc.Store.List(r.Context(), f)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="credentials-masked.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM for Excel
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"uid", "kind", "provider", "service", "masked", "status", "status_detail", "risk", "source_url", "found_at"})
	for _, e := range items {
		_ = cw.Write([]string{e.UID, e.Kind, e.Provider, e.Service, e.Masked, e.Status,
			e.StatusDetail, e.Risk, e.SourceURL, e.FoundAt.Format("2006-01-02 15:04:05")})
	}
	cw.Flush()
}

// GET /api/proxy-nodes
func (s *Server) credProxyList(w http.ResponseWriter, r *http.Request) {
	svc := s.credSvc(w)
	if svc == nil {
		return
	}
	f := credFilterFrom(r)
	f.Kinds = []string{credhunt.KindProxyNode}
	items, total, err := svc.Store.List(r.Context(), f)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"total": total, "items": items})
}

// POST /api/proxy-nodes/{uid}/probe
func (s *Server) credProxyProbe(w http.ResponseWriter, r *http.Request) {
	svc := s.credSvc(w)
	if svc == nil {
		return
	}
	uid := r.PathValue("uid")
	results := svc.Probe(r.Context(), []string{uid}, 1)
	if len(results) == 0 {
		writeErr(w, 404, "节点不存在或没有可探测配置")
		return
	}
	writeJSON(w, 200, results[0])
}

// POST /api/proxy-nodes/probe
func (s *Server) credProxyProbeBatch(w http.ResponseWriter, r *http.Request) {
	svc := s.credSvc(w)
	if svc == nil {
		return
	}
	var req struct {
		UIDs  []string `json:"uids"`
		Limit int      `json:"limit"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(req.UIDs) > 50 {
		req.UIDs = req.UIDs[:50]
	}
	results := svc.Probe(r.Context(), req.UIDs, req.Limit)
	writeJSON(w, 200, map[string]any{"count": len(results), "results": results})
}
