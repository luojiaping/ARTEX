package credhunt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// Fetcher performs read-only HTTP(S) GETs with SSRF protection and writes the
// raw response to an evidence file.
type Fetcher struct {
	DataDir      string
	Timeout      time.Duration
	MaxBytes     int64
	AllowPrivate bool
	Client       *http.Client
}

// NewFetcher builds a fetcher rooted at dataDir/hunts.
func NewFetcher(dataDir string) *Fetcher {
	f := &Fetcher{DataDir: filepath.Join(dataDir, "hunts"), Timeout: 15 * time.Second, MaxBytes: 2 << 20}
	f.Client = &http.Client{Timeout: f.Timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("too many redirects")
		}
		if !f.AllowPrivate {
			if err := f.checkURL(req.Context(), req.URL); err != nil {
				return err
			}
		}
		return nil
	}}
	f.Client.Transport = &http.Transport{Proxy: nil, DialContext: f.dialContext, TLSHandshakeTimeout: 10 * time.Second}
	return f
}

func (f *Fetcher) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if f.AllowPrivate {
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, address)
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ip := range ips {
		if !publicIP(ip.IP) {
			return nil, fmt.Errorf("blocked non-public address %s", ip.IP)
		}
		d := &net.Dialer{Timeout: 10 * time.Second}
		conn, err := d.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no address for %s", host)
	}
	return nil, lastErr
}

func (f *Fetcher) checkURL(ctx context.Context, u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http/https allowed")
	}
	if u.User != nil {
		return fmt.Errorf("userinfo in URL is not allowed")
	}
	if f.AllowPrivate {
		return nil
	}
	host := u.Hostname()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if !publicIP(ip.IP) {
			return fmt.Errorf("blocked non-public address %s", ip.IP)
		}
	}
	return nil
}

func publicIP(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	// IPv4 broadcast / reserved 0.0.0.0/8, 100.64.0.0/10 (CGNAT) and 240.0.0.0/4.
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 0 || v4[0] >= 240 || (v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127) {
			return false
		}
	}
	return true
}

// Fetch retrieves one URL and persists the raw body plus a metadata sidecar.
func (f *Fetcher) Fetch(ctx context.Context, rawURL, runID string) FetchResult {
	res := FetchResult{URL: rawURL}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		res.Error = "invalid URL: " + err.Error()
		return res
	}
	if err := f.checkURL(ctx, u); err != nil {
		res.Error = err.Error()
		return res
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	req.Header.Set("User-Agent", "ARTEX-CredHunter/1.0 (read-only; authorized assessment)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,text/plain,*/*;q=0.5")
	resp, err := f.Client.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()
	res.Status = resp.StatusCode
	res.FinalURL = resp.Request.URL.String()
	res.ContentType = resp.Header.Get("Content-Type")
	body, err := io.ReadAll(io.LimitReader(resp.Body, f.MaxBytes+1))
	if err != nil {
		res.Error = "read: " + err.Error()
		return res
	}
	if int64(len(body)) > f.MaxBytes {
		body = body[:f.MaxBytes]
		res.Truncated = true
	}
	res.Size = int64(len(body))
	res.Body = body
	sum := sha256.Sum256(body)
	res.SHA256 = hex.EncodeToString(sum[:])

	name := filepath.Base(resp.Request.URL.Path)
	if name == "" || name == "." || name == "/" {
		name = "index"
	}
	path, err := f.saveEvidence(runID, res.FinalURL, name, body, map[string]any{
		"url": rawURL, "final_url": res.FinalURL, "status": res.Status,
		"content_type": res.ContentType, "sha256": res.SHA256, "bytes": len(body),
		"fetched_at": time.Now().Format(time.RFC3339),
	})
	if err != nil {
		res.Error = "evidence: " + err.Error()
		return res
	}
	res.EvidencePath = path

	text := string(body)
	if isText(res.ContentType, name) {
		if len(text) > 8192 {
			text = text[:8192]
		}
		res.Text = text
		res.Title = htmlTitle(string(body))
		if listing, dirs, files := ParseListing(res.FinalURL, string(body)); listing {
			res.IsListing = true
			res.Dirs, res.Files = dirs, files
			res.Candidates = RankCandidates(files, 60)
		}
	}
	return res
}

func isText(contentType, name string) bool {
	ct := strings.ToLower(contentType)
	if strings.HasPrefix(ct, "text/") || strings.Contains(ct, "json") || strings.Contains(ct, "xml") ||
		strings.Contains(ct, "yaml") || strings.Contains(ct, "javascript") || strings.Contains(ct, "x-www-form-urlencoded") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".env", ".txt", ".md", ".json", ".yaml", ".yml", ".toml", ".ini", ".conf", ".cfg", ".properties",
		".xml", ".csv", ".sql", ".key", ".pem", ".crt", ".ovpn", ".js", ".ts", ".php", ".py", ".sh", ".log", ".bak":
		return true
	}
	if strings.HasPrefix(filepath.Base(name), ".env") || filepath.Base(name) == ".git-credentials" {
		return true
	}
	return false
}

func (f *Fetcher) saveEvidence(runID, finalURL, name string, body []byte, meta map[string]any) (string, error) {
	run := sanitizePart(runID)
	if run == "" {
		run = time.Now().Format("20060102")
	}
	u, _ := url.Parse(finalURL)
	host := sanitizePart(u.Hostname())
	if host == "" {
		host = "unknown"
	}
	dir := filepath.Join(f.DataDir, run, host)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(finalURL))
	base := sanitizePart(name)
	if base == "" {
		base = "index"
	}
	if len(base) > 80 {
		base = base[:80]
	}
	path := filepath.Join(dir, hex.EncodeToString(sum[:6])+"-"+base)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return "", err
	}
	if meta != nil {
		meta["evidence_path"] = path
		if b, err := json.MarshalIndent(meta, "", "  "); err == nil {
			_ = os.WriteFile(path+".meta.json", b, 0o600)
		}
	}
	return path, nil
}

func sanitizePart(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return ""
	}
	return out
}

func htmlTitle(body string) string {
	z := html.NewTokenizer(strings.NewReader(body))
	inTitle := false
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return ""
		}
		if tt == html.StartTagToken {
			name, _ := z.TagName()
			inTitle = string(name) == "title"
			continue
		}
		if tt == html.TextToken && inTitle {
			return strings.TrimSpace(html.UnescapeString(string(z.Text())))
		}
	}
}

// ParseListing recognizes Apache/nginx/Python/IIS directory indexes and S3 XML
// listings, returning same-host links.
func ParseListing(baseURL, body string) (bool, []Link, []Link) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return false, nil, nil
	}
	lower := strings.ToLower(body)
	listing := strings.Contains(lower, "index of /") || strings.Contains(lower, "directory listing for") ||
		strings.Contains(lower, "<listbucketresult") || strings.Contains(lower, "autoindex")
	if !listing {
		return false, nil, nil
	}
	if strings.Contains(lower, "<listbucketresult") {
		var s3 struct {
			Contents []struct {
				Key  string `xml:"Key"`
				Size int64  `xml:"Size"`
			} `xml:"Contents"`
		}
		if xml.Unmarshal([]byte(body), &s3) == nil && len(s3.Contents) > 0 {
			var files []Link
			for _, c := range s3.Contents {
				files = append(files, Link{URL: strings.TrimRight(base.String(), "/") + "/" + c.Key, Name: c.Key, Size: c.Size})
			}
			return true, nil, files
		}
	}
	z := html.NewTokenizer(strings.NewReader(body))
	var dirs, files []Link
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		name, hasAttr := z.TagName()
		if string(name) != "a" || !hasAttr {
			continue
		}
		var href string
		for {
			key, val, more := z.TagAttr()
			if string(key) == "href" {
				href = string(val)
			}
			if !more {
				break
			}
		}
		href = strings.TrimSpace(href)
		if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "?") ||
			strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") {
			continue
		}
		ref, err := url.Parse(href)
		if err != nil {
			continue
		}
		abs := base.ResolveReference(ref)
		if abs.Host != base.Host || abs.Scheme != base.Scheme {
			continue
		}
		path := abs.Path
		if path == "" || path == base.Path || strings.HasSuffix(path, "/../") {
			continue
		}
		nameText := filepath.Base(strings.TrimRight(path, "/"))
		if nameText == "" || nameText == "." || nameText == ".." {
			continue
		}
		isDir := strings.HasSuffix(abs.Path, "/") || strings.HasSuffix(href, "/")
		link := Link{URL: abs.String(), Name: nameText, IsDir: isDir}
		if isDir {
			dirs = append(dirs, link)
		} else {
			files = append(files, link)
		}
	}
	return true, dirs, files
}

var candidatePatterns = []struct {
	re    *regexp.Regexp
	score int
}{
	{regexp.MustCompile(`(?i)(^|/)\.env(\.|$)`), 100},
	{regexp.MustCompile(`(?i)(^|/)(id_rsa|id_ed25519|\.git-credentials)$`), 100},
	{regexp.MustCompile(`(?i)\.(key|pem|p12|pfx)$`), 95},
	{regexp.MustCompile(`(?i)(config|settings|auth|credentials|providers|secrets|application)\.(json|ya?ml|toml|ini|conf|txt)$`), 92},
	{regexp.MustCompile(`(?i)(clash|mihomo|sing-box|xray|v2ray|sub|subscribe|nodes|proxy).*(ya?ml|json|txt|conf)$`), 88},
	{regexp.MustCompile(`(?i)(docker-compose|wp-config\.php|database\.php|\.npmrc|\.pypirc|\.bash_history)$`), 82},
	{regexp.MustCompile(`(?i)\.(ovpn|conf|cfg)$`), 72},
	{regexp.MustCompile(`(?i)\.(sql|db|sqlite|bak|backup|zip|tar|gz)$`), 55},
}

// RankCandidates scores listing files by how likely they hold credentials.
func RankCandidates(files []Link, limit int) []Link {
	out := make([]Link, 0, len(files))
	for _, f := range files {
		best := 0
		for _, p := range candidatePatterns {
			if p.re.MatchString(f.Name) && p.score > best {
				best = p.score
			}
		}
		if best == 0 {
			continue
		}
		f.Score = best
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Name < out[j].Name
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
