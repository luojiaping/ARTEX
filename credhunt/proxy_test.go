package credhunt

import (
	"encoding/base64"
	"fmt"
	"testing"
)

func TestParseShareLinks(t *testing.T) {
	vmessJSON := `{"v":"2","ps":"n","add":"vm.example.com","port":"443","id":"11111111-1111-1111-1111-111111111111","aid":"0","net":"ws","host":"vm.example.com","path":"/ws","tls":"tls"}`
	vmess := "vmess://" + base64.StdEncoding.EncodeToString([]byte(vmessJSON))
	cases := []struct {
		name, value, protocol, server string
		port                          int
	}{
		{"vless", "vless://11111111-1111-1111-1111-111111111111@v.example.com:443?security=tls&type=ws&path=%2Fws&host=v.example.com#n", "vless", "v.example.com", 443},
		{"vmess", vmess, "vmess", "vm.example.com", 443},
		{"trojan", "trojan://secret@t.example.com:8443?sni=t.example.com#n", "trojan", "t.example.com", 8443},
		{"ss", "ss://YWVzLTI1Ni1nY206c2VjcmV0@ss.example.com:8388#n", "ss", "ss.example.com", 8388},
		{"hysteria2", "hysteria2://secret@h.example.com:443?sni=h.example.com#n", "hysteria2", "h.example.com", 443},
		{"socks5", "socks5://user:pass@s.example.com:1080", "socks5", "s.example.com", 1080},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := ParseProxyContent(tc.value)
			if len(out) != 1 {
				t.Fatalf("got %d nodes, want 1: %+v", len(out), out)
			}
			if out[0].Protocol != tc.protocol || out[0].Server != tc.server || out[0].Port != tc.port {
				t.Fatalf("got %s %s:%d, want %s %s:%d", out[0].Protocol, out[0].Server, out[0].Port,
					tc.protocol, tc.server, tc.port)
			}
		})
	}
}

func TestParseClashAndSubscription(t *testing.T) {
	clash := `proxies:
  - name: a
    type: trojan
    server: a.example.com
    port: 443
    password: secret
  - name: b
    type: ss
    server: b.example.com
    port: 8388
    cipher: aes-256-gcm
    password: secret
`
	out := ParseProxyContent(clash)
	if len(out) != 2 || out[0].Protocol != "trojan" || out[1].Protocol != "ss" {
		t.Fatalf("clash parse = %+v", out)
	}

	sub := "vless://11111111-1111-1111-1111-111111111111@a.example.com:443#a\n" +
		"trojan://secret@b.example.com:443#b\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(sub))
	out = ParseProxyContent(encoded)
	if len(out) != 2 {
		t.Fatalf("base64 subscription parsed %d nodes, want 2", len(out))
	}
}

func TestParseWireGuard(t *testing.T) {
	conf := `[Interface]
PrivateKey = aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa=
Address = 10.0.0.2/32

[Peer]
PublicKey = bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb=
Endpoint = wg.example.com:51820
AllowedIPs = 0.0.0.0/0
`
	out := ParseProxyContent(conf)
	if len(out) != 1 || out[0].Protocol != "wireguard" || out[0].Port != 51820 {
		t.Fatalf("wireguard parse = %+v", out)
	}
}

func TestParseSSOldStyle(t *testing.T) {
	inner := "aes-256-gcm:secret@ss.example.com:8388"
	link := "ss://" + base64.StdEncoding.EncodeToString([]byte(inner))
	out := ParseProxyContent(link)
	if len(out) != 1 || out[0].Server != "ss.example.com" || out[0].Port != 8388 {
		t.Fatalf("old ss parse = %+v", out)
	}
}

func TestParseProxyList(t *testing.T) {
	value := "user1:pass1:1.2.3.4:1080\nuser2:pass2:5.6.7.8:1080"
	out := ParseProxyContent(value)
	if len(out) != 2 || out[0].Protocol != "socks5" {
		t.Fatalf("proxy list parse = %s", fmt.Sprint(out))
	}
}
