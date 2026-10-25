package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFofaSearchToolRendersTable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/info/my"):
			_, _ = w.Write([]byte(`{"error":false,"username":"tester","remain_api_query":5000,"remain_api_data":9000}`))
		case strings.HasSuffix(r.URL.Path, "/search/all"):
			_, _ = w.Write([]byte(`{"error":false,"mode":"extended","page":1,"size":2,"results":[["1.2.3.4:80","1.2.3.4","80","Example"],["5.6.7.8:443","5.6.7.8","443","Other"]]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	FofaKeyProvider = func() (string, string, float64) { return "test-key", srv.URL, 50 }
	resetFofaCacheForTest()
	defer func() { FofaKeyProvider = nil; resetFofaCacheForTest() }()

	ts := NewToolSet(nil, "")
	tool := ts.fofaSearch()
	res, err := tool.Call(context.Background(), json.RawMessage(`{"query":"app=\"Apache\"","fields":"host,ip,port,title","size":2}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, b := range res.Content {
		sb.WriteString(b.Text)
	}
	out := sb.String()
	for _, want := range []string{"mode=page", "quota_left=5000/9000", "| host | ip | port | title |", "1.2.3.4:80"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestFofaToolsReportMissingKey(t *testing.T) {
	FofaKeyProvider = nil
	resetFofaCacheForTest()
	ts := NewToolSet(nil, "")
	res, err := ts.fofaSearch().Call(context.Background(), json.RawMessage(`{"query":"app=\"Apache\""}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Content) == 0 || !strings.Contains(res.Content[0].Text, "FOFA 未配置") {
		t.Fatalf("unexpected: %#v", res.Content)
	}
}

func TestFofaStatsTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":false,"distinct":{"ip":17},"aggs":{"port":[{"count":9,"name":"443"}]},"lastupdatetime":"2026-10-20 10:00:00"}`))
	}))
	defer srv.Close()

	FofaKeyProvider = func() (string, string, float64) { return "k", srv.URL, 50 }
	resetFofaCacheForTest()
	defer func() { FofaKeyProvider = nil; resetFofaCacheForTest() }()

	res, err := NewToolSet(nil, "").fofaStats().Call(context.Background(),
		json.RawMessage(`{"query":"domain=\"x.com\"","fields":"port"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Content[0].Text
	if !strings.Contains(out, "ip: 17") || !strings.Contains(out, "443: 9") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}
