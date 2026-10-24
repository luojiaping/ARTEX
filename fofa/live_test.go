package fofa

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLive is a network smoke test. It only runs when FOFA_LIVE_KEY is set, so
// the normal `go test ./...` stays offline.
//
//	FOFA_LIVE_KEY=xxxx go test ./fofa -run TestLive -v
func TestLive(t *testing.T) {
	key := os.Getenv("FOFA_LIVE_KEY")
	if key == "" {
		t.Skip("FOFA_LIVE_KEY not set")
	}
	c := New(key, os.Getenv("FOFA_BASE_URL"))
	c.SetRatePerSecond(1)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	acc, err := c.Account(ctx)
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	t.Logf("account=%s vip=%v level=%d remain_query=%d remain_data=%d",
		acc.Username, acc.IsVIP, acc.VIPLevel, acc.RemainAPIQuery, acc.RemainAPIData)

	st, err := c.Stats(ctx, `app="Apache"`, []string{"port"}, 3)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	t.Logf("stats distinct=%v aggs=%v", st.Distinct, st.Aggs)

	res, err := c.Search(ctx, `app="Apache"`, []string{"host", "ip", "port"}, 1, 1)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	t.Logf("search total=%d rows=%d", res.Size, len(res.Results))
}
