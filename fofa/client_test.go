package fofa

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := New("test-key", srv.URL)
	c.retryDelay = time.Millisecond
	t.Cleanup(srv.Close)
	return c, srv
}

func TestAccount(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/info/my" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "test-key" {
			t.Errorf("key not sent")
		}
		_, _ = w.Write([]byte(`{"error":false,"username":"bob_allen","isvip":true,"vip_level":101,"remain_api_query":99432,"remain_api_data":790305}`))
	})
	acc, err := c.Account(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acc.Username != "bob_allen" || !acc.IsVIP || acc.VIPLevel != 101 {
		t.Fatalf("unexpected account: %+v", acc)
	}
	if acc.RemainAPIQuery != 99432 {
		t.Fatalf("quota = %d", acc.RemainAPIQuery)
	}
}

func TestSearchEncodesQueryAndFields(t *testing.T) {
	var gotQ, gotFields, gotPage, gotSize string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		gotQ = q.Get("qbase64")
		gotFields = q.Get("fields")
		gotPage = q.Get("page")
		gotSize = q.Get("size")
		_, _ = w.Write([]byte(`{"error":false,"mode":"extended","page":1,"size":2,"query":"app=\"Apache\"","results":[["1.1.1.1:80","1.1.1.1","80"]]}`))
	})
	res, err := c.Search(context.Background(), `app="Apache"`, []string{"host", "ip", "port"}, 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	wantQ := base64.StdEncoding.EncodeToString([]byte(`app="Apache"`))
	if gotQ != wantQ {
		t.Fatalf("qbase64 = %q want %q", gotQ, wantQ)
	}
	if gotFields != "host,ip,port" {
		t.Fatalf("fields = %q", gotFields)
	}
	if gotPage != "1" || gotSize != "50" {
		t.Fatalf("page/size = %s/%s", gotPage, gotSize)
	}
	if len(res.Results) != 1 || res.Results[0][0] != "1.1.1.1:80" {
		t.Fatalf("results = %#v", res.Results)
	}
}

func TestAPIErrorCodeFromMessage(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":true,"errmsg":"[-700] 账号无效"}`))
	})
	_, err := c.Account(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("err type = %T", err)
	}
	if apiErr.Code != -700 || !apiErr.IsKeyInvalid() {
		t.Fatalf("code = %d, keyInvalid = %v", apiErr.Code, apiErr.IsKeyInvalid())
	}
}

func TestRetryOnRateLimitThenSuccess(t *testing.T) {
	calls := 0
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"error":true,"errmsg":"[-5] 请求频率超限"}`))
			return
		}
		_, _ = w.Write([]byte(`{"error":false,"username":"ok"}`))
	})
	acc, err := c.Account(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
	if acc.Username != "ok" {
		t.Fatalf("username = %q", acc.Username)
	}
}

func TestSearchNextCursor(t *testing.T) {
	var gotNext, gotFull string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		gotNext = q.Get("next")
		gotFull = q.Get("full")
		_, _ = w.Write([]byte(`{"error":false,"results":[],"next":123456}`))
	})
	res, err := c.SearchNext(context.Background(), `port="80"`, nil, 100, "abc", true)
	if err != nil {
		t.Fatal(err)
	}
	if gotNext != "abc" || gotFull != "true" {
		t.Fatalf("next/full = %q/%q", gotNext, gotFull)
	}
	if res.NextID() != "123456" {
		t.Fatalf("next id = %q", res.NextID())
	}
}

func TestNoKey(t *testing.T) {
	c := New("", "")
	if _, err := c.Account(context.Background()); err != ErrNoKey {
		t.Fatalf("err = %v", err)
	}
}

func TestStatsAndHost(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/search/stats"):
			_, _ = w.Write([]byte(`{"error":false,"distinct":{"ip":1717},"aggs":{"title":[{"count":35,"name":"x"}]}}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/host/"):
			_, _ = w.Write([]byte(`{"error":false,"host":"1.1.1.1","ip":"1.1.1.1","port":[80,443]}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	st, err := c.Stats(context.Background(), `domain="x.com"`, []string{"title"}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if st.Distinct["ip"] != 1717 || len(st.Aggs["title"]) != 1 {
		t.Fatalf("stats = %+v", st)
	}
	h, err := c.Host(context.Background(), "1.1.1.1", true)
	if err != nil {
		t.Fatal(err)
	}
	if h.IP != "1.1.1.1" || len(h.Port) != 2 {
		t.Fatalf("host = %+v", h)
	}
}

func TestFieldsSanitized(t *testing.T) {
	got := normalizeFields([]string{"host", "bad;DROP", "ip", "", "port"})
	if got != "host,ip,port" {
		t.Fatalf("fields = %q", got)
	}
	if normalizeFields(nil) != "host,ip,port" {
		t.Fatalf("default fields wrong")
	}
	var _ = json.Marshal
}
