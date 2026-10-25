package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// TestFofaLiveTools is the online smoke test for the tool layer. It only runs
// when FOFA_LIVE_KEY is set:
//
//	FOFA_LIVE_KEY=xxxx go test ./agent -run TestFofaLiveTools -v
func TestFofaLiveTools(t *testing.T) {
	key := os.Getenv("FOFA_LIVE_KEY")
	if key == "" {
		t.Skip("FOFA_LIVE_KEY not set")
	}
	FofaKeyProvider = func() (string, string, float64) { return key, os.Getenv("FOFA_BASE_URL"), 1 }
	resetFofaCacheForTest()
	defer func() { FofaKeyProvider = nil; resetFofaCacheForTest() }()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	ts := NewToolSet(nil, "live")

	res, err := ts.fofaAccount().Call(ctx, json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Content[0].Text
	if !strings.Contains(out, "remain_api_query") {
		t.Fatalf("account output unexpected: %s", out)
	}
	t.Logf("account: %s", out)

	res, err = ts.fofaSearch().Call(ctx, json.RawMessage(`{"query":"app=\"Apache\"","fields":"host,ip,port","size":1}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	out = res.Content[0].Text
	if !strings.Contains(out, "| host | ip | port |") {
		t.Fatalf("search output unexpected: %s", out)
	}
	t.Logf("search head: %s", strings.SplitN(out, "\n", 3)[0])
}
