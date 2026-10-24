---
name: proxy-node-probe
description: 代理节点/订阅的识别、解析与最小延迟探测：vless/vmess/trojan/ss/hysteria2/tuic/anytls/socks5/http、WireGuard、Clash/mihomo YAML、base64 订阅、SS CSV、user:pass:host:port 列表；独立 mihomo sidecar 探测，不碰生产代理。
license: MIT
compatibility: 需要 proxy_submit / proxy_probe；服务端需有 mihomo 可执行文件。
---

# 代理节点探测（proxy-node-probe）

## 支持的输入

| 形态 | 例子 |
|---|---|
| 分享链接 | `vless://uuid@host:443?...`、`vmess://base64`、`trojan://...`、`ss://...`、`hysteria2://...`、`tuic://...`、`anytls://...` |
| 代理列表 | `user:pass:host:port`（按 socks5 + http 双形态登记） |
| Clash/mihomo | `proxies:` YAML（单个或整份配置） |
| base64 订阅 | 整段 base64，解码后是多行分享链接或 YAML |
| SS CSV | `server,server_port,password,method,remarks` |
| WireGuard | `[Interface]` + `[Peer]` 的 `.conf` |
| 订阅 URL | `https://.../sub?token=...`、`.../clash.yaml`（服务端只读拉取） |

识别变体要灵活：`x-ui ≡ xui ≡ x_ui ≡ 3x-ui`；`sub` / `subscribe` / `subscription` / `sub-store` / `subconverter`；`nodes` / `proxy` / `机场` / `订阅`。

## 登记

```
proxy_submit(entries=[{
  value: "<链接/订阅内容/订阅 URL>",
  source_url: "<来源页>",
  evidence_path: "<证据文件>",
  service: "clash 订阅 / x-ui 节点"
}])
```

- 单条链接 → 一条 `proxy_node`。
- 订阅 → 父 `proxy_subscription` + 展开的 `proxy_node` 子节点。
- 同一节点重复出现由指纹去重。
- **不要把节点和普通凭据混在 cred_submit**。

## 探测

```
proxy_probe(uids=[...])     # 指定节点
proxy_probe(limit=20)       # 最近 20 条
```

服务端行为：

1. 启动**独立 mihomo sidecar**：`data/mihomo-probe/`，`tun: false`，无入站监听，只开 `127.0.0.1` 控制器。
2. 把本批节点写入临时配置，`PUT /configs` 不需要；每次重启 sidecar。
3. 对每个节点请求：
   - `http://cp.cloudflare.com/generate_204`
   - `http://www.gstatic.com/generate_204`
   - 5 秒超时，任一返回 204 即活。
4. 并发 4；空闲 5 分钟自动停 sidecar。
5. 只产生极少量探测流量，不访问目标业务。

## 判定

| 结果 | 含义 |
|---|---|
| `valid` + delay_ms | 节点可用 |
| `invalid` | 连接失败/认证失败/超时 |
| `error` | sidecar 未启动、mihomo 缺失、配置无法解析 |

## 常见问题

| 现象 | 处理 |
|---|---|
| `mihomo binary not found` | 服务器安装 mihomo，或设置 `MIHOMO_BIN` |
| 节点全 invalid | 检查协议参数（uuid/password/sni/reality pbk/ws path）；先手工用客户端验一条 |
| 订阅展开 0 条 | 内容可能是 HTML 登录页/需要 UA/token；保留证据，报告“需人工” |
| 订阅 URL 403 | 不爆破 token；标记需认证 |
| WireGuard 无 endpoint | 解析失败，保留原文，不入库为节点 |
| Reality 缺 pbk | 无法构建，标 error，不猜 |

## 报告

```
- 订阅：2 个（展开 37 条）
- 节点：37（valid 21 / invalid 14 / error 2）
- 最快：< 80ms
- 仅登记未探测：0
```

## 硬规则

1. 只探测节点本身，不通过节点访问任何目标业务。
2. 不爆破订阅 token、不猜面板口令。
3. 节点配置含密钥，只存加密台账；对话/报告不出现完整 uuid/密码。
4. 生产 mihomo（TUN/9090）绝不复用、不重载、不改配置。
