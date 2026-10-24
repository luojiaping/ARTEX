package credhunt

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParsedProxy is one node extracted from a share link, Clash YAML or subscription.
type ParsedProxy struct {
	Config   map[string]any
	Protocol string
	Server   string
	Port     int
	Error    string
	Remark   string
}

// ParseProxyContent detects Clash YAML, base64 subscriptions, SS CSV, proxy
// lists and share links. It never fetches URLs; the service does that first.
func ParseProxyContent(content string) []ParsedProxy {
	text := strings.TrimSpace(content)
	if text == "" {
		return nil
	}
	// base64 subscription body
	if len(text) > 48 && !strings.Contains(text, "://") && !strings.Contains(text, "\n") && isBase64ish(text) {
		if decoded, err := decodeB64(text); err == nil {
			decodedText := strings.TrimSpace(string(decoded))
			if decodedText != text && (strings.Contains(decodedText, "://") || strings.Contains(decodedText, "proxies:")) {
				return ParseProxyContent(decodedText)
			}
		}
	}
	// Clash / mihomo YAML
	if strings.Contains(text[:min(len(text), 4096)], "proxies:") {
		var doc map[string]any
		if err := yaml.Unmarshal([]byte(text), &doc); err == nil {
			if list, ok := doc["proxies"].([]any); ok {
				var out []ParsedProxy
				for _, item := range list {
					if m, ok := item.(map[string]any); ok {
						if p, err := normalizeClashProxy(m); err == nil {
							out = append(out, p)
						}
					}
				}
				if len(out) > 0 {
					return out
				}
			}
		}
	}
	lines := splitLines(text)
	// SS CSV
	if strings.HasPrefix(strings.ToLower(lines[0]), "server,") || strings.HasPrefix(strings.ToLower(lines[0]), "server\t") {
		return parseSSCsv(lines)
	}
	// user:pass:host:port proxy lists (socks5/http dual form)
	if allProxyLines(lines) {
		var out []ParsedProxy
		for _, line := range lines {
			parts := strings.Split(line, ":")
			if len(parts) != 4 {
				continue
			}
			port, _ := strconv.Atoi(parts[3])
			if port <= 0 {
				continue
			}
			cfg := map[string]any{"type": "socks5", "server": parts[2], "port": port,
				"username": parts[0], "password": parts[1], "udp": true}
			out = append(out, ParsedProxy{Config: cfg, Protocol: "socks5", Server: parts[2], Port: port,
				Remark: parts[0] + "@" + parts[2] + ":" + parts[3]})
		}
		if len(out) > 0 {
			return out
		}
	}
	var out []ParsedProxy
	for _, line := range lines {
		if p, err := parseShareLink(line); err == nil {
			out = append(out, p)
		} else if strings.Contains(line, "[Interface]") {
			// WireGuard blocks are handled by ParseWireGuardConf below.
			continue
		}
	}
	if len(out) == 0 && strings.Contains(text, "[Interface]") {
		if p, err := parseWireGuardConf(text); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func splitLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return out
}

func isBase64ish(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '+', r == '/', r == '=', r == '-', r == '_', r == '\n', r == '\r', r == ' ':
		default:
			return false
		}
	}
	return true
}

func decodeB64(s string) ([]byte, error) {
	clean := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	clean += strings.Repeat("=", (4-len(clean)%4)%4)
	if b, err := base64.StdEncoding.DecodeString(clean); err == nil {
		return b, nil
	}
	return base64.URLEncoding.DecodeString(clean)
}

func allProxyLines(lines []string) bool {
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if strings.Contains(line, "://") {
			return false
		}
		if len(strings.Split(line, ":")) != 4 {
			return false
		}
	}
	return true
}

func parseSSCsv(lines []string) []ParsedProxy {
	if len(lines) < 2 {
		return nil
	}
	head := strings.Split(strings.ToLower(lines[0]), ",")
	idx := map[string]int{}
	for i, h := range head {
		idx[strings.TrimSpace(h)] = i
	}
	var out []ParsedProxy
	for _, line := range lines[1:] {
		cols := strings.Split(line, ",")
		get := func(k string) string {
			i, ok := idx[k]
			if !ok || i >= len(cols) {
				return ""
			}
			return strings.TrimSpace(cols[i])
		}
		port, _ := strconv.Atoi(get("server_port"))
		server, password, method := get("server"), get("password"), get("method")
		if server == "" || port == 0 || password == "" || method == "" {
			continue
		}
		cfg := map[string]any{"type": "ss", "server": server, "port": port, "cipher": method, "password": password, "udp": true}
		out = append(out, ParsedProxy{Config: cfg, Protocol: "ss", Server: server, Port: port,
			Remark: get("remarks")})
	}
	return out
}

func parseShareLink(link string) (ParsedProxy, error) {
	link = strings.TrimSpace(link)
	idx := strings.Index(link, "://")
	if idx <= 0 {
		return ParsedProxy{}, fmt.Errorf("not a share link")
	}
	scheme := strings.ToLower(link[:idx])
	rest := link[idx+3:]
	if hash := strings.Index(rest, "#"); hash >= 0 {
		rest = rest[:hash]
	}
	switch scheme {
	case "vmess":
		return parseVMess(rest)
	case "vless":
		return parseVLESS(rest)
	case "trojan":
		return parseTrojan(rest)
	case "ss":
		return parseSS(rest)
	case "hysteria2", "hy2":
		return parseHysteria2(rest, scheme)
	case "tuic":
		return parseTUIC(rest)
	case "anytls":
		return parseAnyTLS(rest)
	case "socks5", "socks", "http", "https":
		return parseHTTPProxy(rest, scheme)
	default:
		return ParsedProxy{}, fmt.Errorf("unsupported scheme %s", scheme)
	}
}

func parseVMess(rest string) (ParsedProxy, error) {
	blob, err := decodeB64(rest)
	if err != nil {
		return ParsedProxy{}, err
	}
	var m map[string]any
	if err := json.Unmarshal(blob, &m); err != nil {
		return ParsedProxy{}, err
	}
	server := asString(m["add"])
	port := intValue(m["port"])
	if server == "" || port == 0 {
		return ParsedProxy{}, fmt.Errorf("vmess: missing add/port")
	}
	cfg := map[string]any{"type": "vmess", "server": server, "port": port,
		"uuid": asString(m["id"]), "alterId": intValue(m["aid"]), "cipher": firstNonEmpty(asString(m["scy"]), "auto"), "udp": true}
	tlsMode := strings.ToLower(asString(m["tls"]))
	if tlsMode == "tls" || tlsMode == "reality" || tlsMode == "true" || tlsMode == "1" || tlsMode == "xtls" {
		cfg["tls"] = true
		cfg["servername"] = firstNonEmpty(asString(m["sni"]), asString(m["host"]))
	}
	net := strings.ToLower(firstNonEmpty(asString(m["net"]), "tcp"))
	switch net {
	case "ws":
		cfg["network"] = "ws"
		cfg["ws-opts"] = map[string]any{"path": firstNonEmpty(asString(m["path"]), "/"),
			"headers": map[string]any{"Host": asString(m["host"])}}
	case "grpc":
		cfg["network"] = "grpc"
		cfg["grpc-opts"] = map[string]any{"grpc-service-name": asString(m["path"])}
	case "h2", "http":
		cfg["network"] = "h2"
	}
	return proxyFromConfig(cfg)
}

func parseVLESS(rest string) (ParsedProxy, error) {
	userinfo, netloc := splitUserHost(rest)
	if userinfo == "" {
		return ParsedProxy{}, fmt.Errorf("vless: missing uuid")
	}
	host, port, q := splitHostPortQuery(netloc)
	if host == "" || port == 0 {
		return ParsedProxy{}, fmt.Errorf("vless: missing host/port")
	}
	cfg := map[string]any{"type": "vless", "server": host, "port": port, "uuid": userinfo, "udp": true}
	security := strings.ToLower(firstNonEmpty(q.Get("security"), q.Get("tls")))
	if security == "tls" || security == "reality" || security == "xtls" {
		cfg["tls"] = true
	}
	if security == "reality" {
		opts := map[string]any{}
		if v := q.Get("pbk"); v != "" {
			opts["public-key"] = v
		}
		if v := q.Get("sid"); v != "" {
			opts["short-id"] = v
		}
		if len(opts) > 0 {
			cfg["reality-opts"] = opts
		}
	}
	if v := q.Get("sni"); v != "" {
		cfg["servername"] = v
	}
	if v := q.Get("flow"); v != "" {
		cfg["flow"] = v
	}
	if v := q.Get("fp"); v != "" {
		cfg["client-fingerprint"] = v
	}
	switch strings.ToLower(firstNonEmpty(q.Get("type"), q.Get("net"), "tcp")) {
	case "ws":
		cfg["network"] = "ws"
		cfg["ws-opts"] = map[string]any{"path": firstNonEmpty(q.Get("path"), "/"),
			"headers": map[string]any{"Host": q.Get("host")}}
	case "grpc":
		cfg["network"] = "grpc"
		cfg["grpc-opts"] = map[string]any{"grpc-service-name": firstNonEmpty(q.Get("servicename"), q.Get("serviceName"))}
	case "h2", "http":
		cfg["network"] = "h2"
	}
	if strings.EqualFold(q.Get("allowinsecure"), "1") || strings.EqualFold(q.Get("allowinsecure"), "true") {
		cfg["skip-cert-verify"] = true
	}
	return proxyFromConfig(cfg)
}

func parseTrojan(rest string) (ParsedProxy, error) {
	password, netloc := splitUserHost(rest)
	if password == "" {
		return ParsedProxy{}, fmt.Errorf("trojan: missing password")
	}
	host, port, q := splitHostPortQuery(netloc)
	if host == "" || port == 0 {
		return ParsedProxy{}, fmt.Errorf("trojan: missing host/port")
	}
	cfg := map[string]any{"type": "trojan", "server": host, "port": port, "password": password, "udp": true}
	if v := q.Get("sni"); v != "" {
		cfg["sni"] = v
	}
	switch strings.ToLower(firstNonEmpty(q.Get("type"), q.Get("net"), "tcp")) {
	case "ws":
		cfg["network"] = "ws"
		cfg["ws-opts"] = map[string]any{"path": firstNonEmpty(q.Get("path"), "/"),
			"headers": map[string]any{"Host": q.Get("host")}}
	case "grpc":
		cfg["network"] = "grpc"
		cfg["grpc-opts"] = map[string]any{"grpc-service-name": q.Get("servicename")}
	}
	return proxyFromConfig(cfg)
}

func parseSS(rest string) (ParsedProxy, error) {
	if !strings.Contains(rest, "@") {
		if decoded, err := decodeB64(rest); err == nil && strings.Contains(string(decoded), "@") {
			rest = string(decoded)
		}
	}
	userinfo, netloc := splitUserHost(rest)
	if userinfo == "" {
		return ParsedProxy{}, fmt.Errorf("ss: missing userinfo")
	}
	if decoded, err := decodeB64(userinfo); err == nil && strings.Contains(string(decoded), ":") {
		userinfo = string(decoded)
	}
	host, port, _ := splitHostPortQuery(netloc)
	if host == "" || port == 0 {
		return ParsedProxy{}, fmt.Errorf("ss: missing host/port")
	}
	method, password, ok := strings.Cut(userinfo, ":")
	if !ok || method == "" || password == "" {
		return ParsedProxy{}, fmt.Errorf("ss: missing method/password")
	}
	cfg := map[string]any{"type": "ss", "server": host, "port": port, "cipher": method, "password": password, "udp": true}
	return proxyFromConfig(cfg)
}

func parseHysteria2(rest string, scheme string) (ParsedProxy, error) {
	password, netloc := splitUserHost(rest)
	host, port, q := splitHostPortQuery(netloc)
	if host == "" || port == 0 {
		return ParsedProxy{}, fmt.Errorf("%s: missing host/port", scheme)
	}
	cfg := map[string]any{"type": "hysteria2", "server": host, "port": port, "password": password, "udp": true}
	if v := q.Get("sni"); v != "" {
		cfg["sni"] = v
	}
	if v := q.Get("obfs"); v != "" {
		cfg["obfs"] = v
	}
	if v := firstNonEmpty(q.Get("obfs-password"), q.Get("obfs_password")); v != "" {
		cfg["obfs-password"] = v
	}
	return proxyFromConfig(cfg)
}

func parseTUIC(rest string) (ParsedProxy, error) {
	userinfo, netloc := splitUserHost(rest)
	uuid, password, _ := strings.Cut(userinfo, ":")
	host, port, q := splitHostPortQuery(netloc)
	if host == "" || port == 0 {
		return ParsedProxy{}, fmt.Errorf("tuic: missing host/port")
	}
	cfg := map[string]any{"type": "tuic", "server": host, "port": port, "uuid": uuid, "password": password, "udp": true}
	if v := q.Get("sni"); v != "" {
		cfg["sni"] = v
	}
	return proxyFromConfig(cfg)
}

func parseAnyTLS(rest string) (ParsedProxy, error) {
	password, netloc := splitUserHost(rest)
	host, port, q := splitHostPortQuery(netloc)
	if host == "" || port == 0 {
		return ParsedProxy{}, fmt.Errorf("anytls: missing host/port")
	}
	cfg := map[string]any{"type": "anytls", "server": host, "port": port, "password": password, "udp": true}
	if v := q.Get("sni"); v != "" {
		cfg["sni"] = v
	}
	return proxyFromConfig(cfg)
}

func parseHTTPProxy(rest string, scheme string) (ParsedProxy, error) {
	userinfo, netloc := splitUserHost(rest)
	host, port, _ := splitHostPortQuery(netloc)
	if host == "" || port == 0 {
		return ParsedProxy{}, fmt.Errorf("%s: missing host/port", scheme)
	}
	typ := "http"
	if scheme == "socks5" || scheme == "socks" {
		typ = "socks5"
	}
	cfg := map[string]any{"type": typ, "server": host, "port": port, "udp": true}
	if userinfo != "" {
		user, pass, _ := strings.Cut(userinfo, ":")
		cfg["username"] = user
		cfg["password"] = pass
	}
	if scheme == "https" {
		cfg["tls"] = true
	}
	return proxyFromConfig(cfg)
}

func parseWireGuardConf(text string) (ParsedProxy, error) {
	iface := map[string]string{}
	peer := map[string]string{}
	current := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch strings.ToLower(line) {
		case "[interface]":
			current = "i"
			continue
		case "[peer]":
			current = "p"
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		if current == "i" {
			iface[k] = v
		} else if current == "p" {
			peer[k] = v
		}
	}
	server, portStr, ok := strings.Cut(peer["endpoint"], ":")
	if !ok {
		return ParsedProxy{}, fmt.Errorf("wireguard: missing endpoint")
	}
	port, _ := strconv.Atoi(portStr)
	cfg := map[string]any{"type": "wireguard", "server": strings.Trim(server, "[]"), "port": port,
		"private-key": iface["privatekey"], "public-key": peer["publickey"],
		"allowed-ips": []string{"0.0.0.0/0"}, "udp": true}
	if v := iface["address"]; v != "" {
		cfg["ip"] = strings.Split(v, ",")[0]
	}
	if v := peer["presharedkey"]; v != "" {
		cfg["pre-shared-key"] = v
	}
	return proxyFromConfig(cfg)
}

func normalizeClashProxy(m map[string]any) (ParsedProxy, error) {
	typ := asString(m["type"])
	server := asString(m["server"])
	port := intValue(m["port"])
	if typ == "" || server == "" || port == 0 {
		return ParsedProxy{}, fmt.Errorf("clash entry missing type/server/port")
	}
	cfg := map[string]any{}
	for k, v := range m {
		if k == "name" || k == "dialer-proxy" {
			continue
		}
		cfg[k] = v
	}
	cfg["type"], cfg["server"], cfg["port"] = typ, server, port
	return proxyFromConfig(cfg)
}

func proxyFromConfig(cfg map[string]any) (ParsedProxy, error) {
	typ := asString(cfg["type"])
	server := asString(cfg["server"])
	port := intValue(cfg["port"])
	if typ == "" || server == "" || port == 0 {
		return ParsedProxy{}, fmt.Errorf("incomplete proxy config")
	}
	return ParsedProxy{Config: cfg, Protocol: typ, Server: server, Port: port}, nil
}

func splitUserHost(rest string) (string, string) {
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		return rest[:at], rest[at+1:]
	}
	return "", rest
}

func splitHostPortQuery(netloc string) (string, int, url.Values) {
	path := ""
	if slash := strings.Index(netloc, "/"); slash >= 0 {
		path, netloc = netloc[slash:], netloc[:slash]
	}
	query := ""
	if q := strings.Index(netloc, "?"); q >= 0 {
		query, netloc = netloc[q+1:], netloc[:q]
	}
	host, portStr := netloc, ""
	if h, p, ok := strings.Cut(netloc, ":"); ok && !strings.HasPrefix(netloc, "[") {
		host, portStr = h, p
	} else if strings.HasPrefix(netloc, "[") {
		if end := strings.Index(netloc, "]"); end > 0 {
			host = netloc[1:end]
			if rest := netloc[end+1:]; strings.HasPrefix(rest, ":") {
				portStr = rest[1:]
			}
		}
	}
	port, _ := strconv.Atoi(strings.Split(portStr, "-")[0])
	values, _ := url.ParseQuery(query)
	_ = path
	return host, port, values
}

func intValue(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(strings.Split(n, "-")[0])
		return i
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
