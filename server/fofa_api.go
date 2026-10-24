package server

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Autumn-27/artex/fofa"
)

// GET /api/settings/fofa/account
//
// Proxies FOFA's /api/v1/info/my so the settings page can show live quota
// without ever exposing the API key to the browser. The key is read from the
// settings table on each request, so a newly saved key works immediately.
func (s *Server) fofaAccountInfo(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	key, _, _ := pg.GetSetting(settingFOFAKey)
	key = strings.TrimSpace(key)
	if key == "" {
		writeErr(w, 400, "FOFA API Key 未配置")
		return
	}
	base, _, _ := pg.GetSetting(settingFOFABaseURL)

	c := fofa.New(key, strings.TrimSpace(base))
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	acc, err := c.Account(ctx)
	if err != nil {
		writeErr(w, 502, "获取 FOFA 额度失败："+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"username":          acc.Username,
		"email":             acc.Email,
		"isvip":             acc.IsVIP,
		"vip_level":         acc.VIPLevel,
		"remain_api_query":  acc.RemainAPIQuery,
		"remain_api_data":   acc.RemainAPIData,
		"remain_free_point": acc.RemainFreePoint,
		"fofa_point":        acc.FofaPoint,
		"fcoin":             acc.FCoin,
		"checked_at":        time.Now().Format(time.RFC3339),
	})
}
