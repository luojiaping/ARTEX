package credhunt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ProbeItem is one node to test.
type ProbeItem struct {
	UID    string
	Config map[string]any
}

// Prober runs a dedicated mihomo sidecar (no TUN, no inbound listener) and asks
// its controller for per-node delay. It never touches the production proxy.
type Prober struct {
	Bin  string
	Dir  string
	URLs []string

	mu     sync.Mutex
	cmd    *exec.Cmd
	port   int
	secret string
	timer  *time.Timer
}

// NewProber locates the mihomo binary and prepares a private data dir.
func NewProber(dataDir string) *Prober {
	bin := strings.TrimSpace(os.Getenv("MIHOMO_BIN"))
	if bin == "" {
		for _, candidate := range []string{"/usr/local/bin/mihomo", "/usr/bin/mihomo"} {
			if _, err := os.Stat(candidate); err == nil {
				bin = candidate
				break
			}
		}
	}
	if bin == "" {
		if p, err := exec.LookPath("mihomo"); err == nil {
			bin = p
		}
	}
	return &Prober{Bin: bin, Dir: filepath.Join(dataDir, "mihomo-probe"),
		URLs: []string{"http://cp.cloudflare.com/generate_204", "http://www.gstatic.com/generate_204"}}
}

// Probe starts the sidecar (if needed), tests every item and returns one result
// per item. A sidecar failure is reported per item instead of aborting the run.
func (p *Prober) Probe(ctx context.Context, items []ProbeItem) []ProbeResult {
	out := make([]ProbeResult, len(items))
	for i, it := range items {
		out[i] = ProbeResult{UID: it.UID, Protocol: asString(it.Config["type"]),
			Server: asString(it.Config["server"]), Port: intValue(it.Config["port"])}
	}
	if len(items) == 0 {
		return out
	}
	if p.Bin == "" {
		for i := range out {
			out[i].Status, out[i].Message = "error", "mihomo binary not found"
		}
		return out
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	proxies := make([]map[string]any, 0, len(items))
	for _, it := range items {
		cfg := make(map[string]any, len(it.Config)+1)
		for k, v := range it.Config {
			cfg[k] = v
		}
		cfg["name"] = it.UID
		proxies = append(proxies, cfg)
	}
	if err := p.restart(proxies); err != nil {
		for i := range out {
			out[i].Status, out[i].Message = "error", err.Error()
		}
		return out
	}
	p.scheduleStop()
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range items {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			status, delay, msg, target := p.delay(ctx, items[idx].UID)
			out[idx].Status, out[idx].DelayMS, out[idx].Message, out[idx].URL = status, delay, msg, target
		}(i)
	}
	wg.Wait()
	return out
}

func (p *Prober) restart(proxies []map[string]any) error {
	p.stopLocked()
	if err := os.MkdirAll(p.Dir, 0o700); err != nil {
		return err
	}
	p.port = freePort(19090)
	p.secret = fmt.Sprintf("artex-%d", time.Now().UnixNano())
	cfg := map[string]any{
		"log-level":           "warning",
		"ipv6":                true,
		"mode":                "rule",
		"external-controller": fmt.Sprintf("127.0.0.1:%d", p.port),
		"secret":              p.secret,
		"proxies":             proxies,
		"rules":               []string{"MATCH,DIRECT"},
		"tun":                 map[string]any{"enable": false},
		"dns": map[string]any{"enable": true,
			"nameserver": []string{"1.1.1.1", "8.8.8.8"}},
	}
	yamlBytes, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(p.Dir, "config.yaml")
	if err := os.WriteFile(cfgPath, yamlBytes, 0o600); err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd := exec.Command(p.Bin, "-d", p.Dir, "-f", cfgPath)
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mihomo start: %w", err)
	}
	p.cmd = cmd
	client := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/version", p.port)
	for i := 0; i < 40; i++ {
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			break
		}
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer "+p.secret)
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	msg := strings.TrimSpace(stderr.String())
	if len(msg) > 500 {
		msg = msg[len(msg)-500:]
	}
	p.stopLocked()
	return fmt.Errorf("mihomo controller not ready: %s", msg)
}

func (p *Prober) delay(ctx context.Context, name string) (status string, delayMS int, message, target string) {
	client := &http.Client{Timeout: 9 * time.Second}
	for _, target := range p.URLs {
		path := fmt.Sprintf("http://127.0.0.1:%d/proxies/%s/delay?url=%s&timeout=5000",
			p.port, url.PathEscape(name), url.QueryEscape(target))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
		if err != nil {
			return "error", 0, err.Error(), target
		}
		req.Header.Set("Authorization", "Bearer "+p.secret)
		resp, err := client.Do(req)
		if err != nil {
			message = err.Error()
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		var parsed struct {
			Delay   int    `json:"delay"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &parsed)
		if resp.StatusCode == 200 && parsed.Delay > 0 {
			return "valid", parsed.Delay, "", target
		}
		if parsed.Message != "" {
			message = parsed.Message
		} else {
			message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
	}
	if message == "" {
		message = "no response"
	}
	return "invalid", 0, message, p.URLs[len(p.URLs)-1]
}

func (p *Prober) scheduleStop() {
	if p.timer != nil {
		p.timer.Stop()
	}
	p.timer = time.AfterFunc(5*time.Minute, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.stopLocked()
	})
}

func (p *Prober) stopLocked() {
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_, _ = p.cmd.Process.Wait()
	}
	p.cmd = nil
}

func freePort(preferred int) int {
	for _, port := range []int{preferred, 0} {
		if port == 0 {
			break
		}
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			ln.Close()
			return port
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return preferred
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}
