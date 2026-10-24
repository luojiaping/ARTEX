---
name: scan-opsec
description: 渗透探测扫描的出口防护与IP轮换规程。对任何外部目标做端口扫描/目录爆破/漏洞扫描/子域枚举/爬虫等主动探测前，必须先读本技能：所有扫描流量经 mihomo 强制代理通道 127.0.0.1:7891 出口并动态轮换 IP，永不暴露本机真实 IP。
---
# 扫描出口防护与 IP 轮换（scan-opsec）

## 背景（为什么要这样做）
本机 mihomo（TUN 模式）接管全局流量，但规则链尾部存在：
- `GEOIP,CN,DIRECT` —— 发往**中国大陆目标**的流量会走真实出口（即本机真实 IP）
- `MATCH,PROXY` —— 其余走代理

因此：**不带代理的扫描流量，扫国内目标会直接暴露本机真实 IP**。绝对禁止。

## 强制通道（唯一允许的扫描出口）
mihomo 已配置专用监听口，固定绑定选择器 `SCAN-POOL`，**绕过所有规则强制走代理**：

- `127.0.0.1:7891`（HTTP + SOCKS5 mixed）→ SCAN-POOL 节点池（27 个节点）

## 标准操作规程
1. **扫描前轮换**：`scanctl rotate`
   - 自动测速选新节点（避开最近两个），输出 `{"node","egress_ip","real_ip_leak":false}`
   - 若 `real_ip_leak:true` 或失败，禁止扫描，先修复。
2. **执行扫描**（任选其一）：
   - `scanctl run -- <完整命令>`：自动轮换 + 注入代理环境变量执行（最推荐）
   - `scanctl env` 打印代理变量，自行 `export`：
     `http_proxy/https_proxy/all_proxy` 及大写版本，值均为 `http://127.0.0.1:7891`
   - 支持 proxy 参数的工具：`--proxy http://127.0.0.1:7891` 或 `socks5://127.0.0.1:7891`
   - proxychains4 已指向 7891：`proxychains4 -q <cmd>`
3. **长任务中轮换**：批量扫描/爆破每过一段时间（或目标切换）再次 `scanctl rotate` 换 IP。
4. **收尾检查**：`scanctl guard` 输出 `{"guard":"OK"}` 才算无泄露。

## 命令速查
```bash
scanctl rotate      # 轮换出口 IP（扫描前必做）
scanctl egress      # 查当前出口 IP / 是否泄露
scanctl guard       # 全面守卫检查（端口/节点/泄露）
scanctl status      # 状态总览
scanctl env         # 输出代理环境变量
scanctl run [--no-rotate] -- CMD...   # 轮换+带代理执行
```

## 硬性规则（不得违反）
- ❌ 禁止经由 7890（常规入口）出扫描流量——受规则控制，CN 目标会直连泄露。
- ❌ 禁止省略代理直接扫描/爆破/枚举任何外部目标。
- ❌ 禁止在任何输出、报告、流量捕获中打印真实 IP 与 `.real_ip_baseline_cn` 内容。
- ✅ `scanctl guard` 报 FAIL 时，先修复再继续任何扫描。
- ✅ 工具示例：
  ```bash
  scanctl run -- nmap -sT -Pn -p 80,443 target.example.com
  scanctl run -- gobuster dir -u https://target -w /usr/share/seclists/Discovery/Web-Content/common.txt
  scanctl run -- ffuf -u https://target/FUZZ -w /usr/share/seclists/Discovery/Web-Content/raft-medium-words.txt
  scanctl run -- nuclei -u https://target -severity critical,high
  ```
  （nmap 注意：-sS 需要 raw socket，代理不生效；经代理扫描请用 `-sT` 全连接模式。）

## 故障处理
- 7891 不通：`systemctl is-active mihomo`，必要时 `curl -X PUT http://127.0.0.1:9090/configs?force=true -d '{"path":"/etc/mihomo/config.yaml"}'` 重载。
- 节点全部超时：`scanctl rotate --refresh`（强制重新测速）。
- 需要给某节点固定 IP 时：mihomo API `PUT /proxies/SCAN-POOL {"name":"节点名"}`（不推荐绕过 scanctl）。
