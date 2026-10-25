package credhunt

import (
	"net"
	"testing"
)

func TestParseListingApache(t *testing.T) {
	body := `<html><head><title>Index of /backup</title></head><body>
<h1>Index of /backup</h1>
<pre><a href="../">../</a>
<a href=".env">.env</a>                 12K
<a href="config/">config/</a>
<a href="app.zip">app.zip</a>              2.1M
</pre></body></html>`
	ok, dirs, files := ParseListing("https://example.com/backup/", body)
	if !ok {
		t.Fatal("not detected as listing")
	}
	if len(dirs) != 1 || len(files) != 2 {
		t.Fatalf("dirs=%d files=%d", len(dirs), len(files))
	}
	cands := RankCandidates(files, 10)
	if len(cands) < 2 || cands[0].Name != ".env" {
		t.Fatalf("candidates = %+v", cands)
	}
}

func TestParseListingS3(t *testing.T) {
	body := `<ListBucketResult><Contents><Key>config/providers.json</Key><Size>123</Size></Contents>
<Contents><Key>readme.txt</Key><Size>10</Size></Contents></ListBucketResult>`
	ok, _, files := ParseListing("https://bucket.s3.example.com/", body)
	if !ok || len(files) != 2 {
		t.Fatalf("s3 parse ok=%v files=%d", ok, len(files))
	}
	if RankCandidates(files, 5)[0].Name != "config/providers.json" {
		t.Fatalf("s3 candidate ranking = %+v", RankCandidates(files, 5))
	}
}

func TestPublicIP(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "169.254.169.254", "::1", "fc00::1"} {
		if publicIP(net.ParseIP(s)) {
			t.Errorf("%s should be blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !publicIP(net.ParseIP(s)) {
			t.Errorf("%s should be public", s)
		}
	}
}
