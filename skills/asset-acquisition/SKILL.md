---
name: asset-acquisition
description: FOFA 驱动的资产获取闭环：确定授权范围 → FOFA 检索 → 逐主机存活/指纹验证 → 去重并写入 ARTEX 资产图 → 输出可复核的交付报告。适用于把域名/IP/组织/ICP/证书扩展成可扫描资产，或定期刷新资产面。
license: MIT
compatibility: 需要 fofa_account / fofa_stats / fofa_search / fofa_host 与 insert_assets / list_assets / list_companies 工具；Bash 需可执行 curl。
---

# 资产获取闭环（asset-acquisition）

目标不是「查到最多」，而是「把**验证过、可复核**的资产写进资产图」。FOFA 只负责给候选，存活与指纹必须自己验。

## 输入

用户给出的授权目标，通常是以下之一：

- 域名：`example.com`
- URL：`https://app.example.com/login`
- IP / CIDR：`1.2.3.4`、`1.2.3.0/24`
- 组织 / ASN / ICP：`org="Example Inc"`、`asn="12345"`、`icp="京ICP备..."`

范围不清（例如只说公司名、但不确定主域）时，**先问一句**再动手；不要用宽泛关键词裸查。

## 固定流程

### 0) 读规程

本技能负责流程；查询语法、字段、错误码、成本控制见 `fofa-recon` 技能。两者配合使用。

### 1) 固化 scope

把目标翻译成一条 FOFA 查询锚点，写进本轮记录：

```
domain="example.com"
ip="1.2.3.4"
org="Example Inc"
cert="example.com"
icon_hash="-247388890"
```

优先高选择性锚点：`domain` / `ip` / `cert` / `icon_hash` > `org` / `asn` / `icp` > `title` / `body`。
一个 scope 查不干净时，最多再补一个**同源**锚点（如 `domain` + `cert`），不要扩散到无关组织。

### 2) 查额度

```
fofa_account()
```

`remain_api_query < 200` 时先告知用户，询问是否继续。

### 3) 估规模

```
fofa_stats(query=scope, fields="port,title,server,domain", size=5)
```

- `-403`：账号没开通 stats，**跳过，不重试**，改用 `size=50` 的 search 试拉。
- 估算 > 5000 条：先确认是否继续，或缩小 scope。
- 记住：`search` 不返回命中总数，别把返回的 `size` 当总量。

### 4) 取明细

```
fofa_search(
  query=scope,
  fields="host,ip,port,domain,title,server,protocol,icp,country,province,city,asn,org,link",
  size=100,
  mode="page"
)
```

- 需要更多：`mode="next"`，首次 `next` 留空，之后填返回的游标。
- 单次 `size ≤ 500`，不要 10000。
- 按 `host:port` 去重；同一 IP 多个端口保留多条服务记录。

### 5) 存活验证（必须，不可跳过）

对每个唯一 `host:port`，用 Bash + curl 做只读验证：

```bash
# 单条示例：只 GET 根路径，8 秒超时，跟随跳转
curl -k -sS -L --max-time 8 -o /dev/null \
  -w '%{http_code}\t%{content_type}\t%{size_download}\t%{url_effective}\n' \
  "https://HOST:PORT/"
```

批量时写成脚本，**并发 ≤ 8**，批间 `sleep 1`：

```bash
# candidates.txt: 每行 host:port
cat candidates.txt | xargs -P 8 -I{} sh -c '
  hp="{}"
  for scheme in https http; do
    out=$(curl -k -sS -L --max-time 8 -o /dev/null \
      -w "%{http_code}\t%{content_type}\t%{size_download}\t%{url_effective}" \
      "$scheme://$hp/" 2>/dev/null) && { printf "%s\t%s\n" "$hp" "$out"; exit 0; }
  done
  printf "%s\tDEAD\t-\t-\t-\n" "$hp"
'
```

规则：

- **只 GET/HEAD 根路径**；不要访问 `/admin`、`/api` 等路径做爆破。
- **不要 nmap 扫端口**；端口只信 FOFA 给出的结果。
- 超时 8s；https 失败再试 http；都失败记 `DEAD`。
- `200/301/302/401/403/404` 都算「服务存活」；`DEAD` 才算不可达。
- 把结果写到 `verify-<scope>.tsv`，列：`host:port, code, content_type, size, final_url`。

### 6) 去重与入库

只入**验证存活**的资产。先查重：

```
list_assets(dsl="ip=\"1.2.3.4\"")
list_assets(dsl="domain=\"app.example.com\"")
```

字段映射：

| FOFA/验证结果 | 资产类型 | 字段 |
|---|---|---|
| 纯 IP | `ip` | `ip`、`open_ports[{port,service}]`、`bound_domains` |
| 域名/子域 | `subdomain` | `domain`、`record_value=[ip]`、`record_type="A"` |
| 主域 | `root_domain` | `domain`、`icp` |
| HTTP/HTTPS 服务 | `service` | `url`（优先 `link` 或验证后的 final_url）、`service_ip`、`page_title`、`technologies=[server]`、`status_code`、`content_length` |
| 非 HTTP 服务 | `service` | `service_name`（ssh/mysql/rdp…）、`port`、`ip` 或 `domain` |

`insert_assets` 每批 ≤ 50 条，分批提交。`status_code`/`content_length`/`technologies` 必须来自本次验证，不要抄 FOFA。

### 7) 交付

输出一段可复核的报告：

```markdown
## 资产获取报告 · <scope>

- 范围：<FOFA query>
- FOFA 命中：<n 或 未知>
- 验证主机：<n>（存活 <alive> / 不可达 <dead>）
- 入库：<n> 条（ip <a> / subdomain <b> / service <c>）
- 跳过：<n> 条（重复 <x> / 未验证 <y> / 范围外 <z>）

| 类型 | 标识 | 端口 | 标题 | 状态码 |
|---|---|---|---|---|
| service | https://... | 443 | ... | 200 |

证据：`verify-<scope>.tsv`
```

同时把同样内容写到当前工作目录的 `fofa-run-<YYYYmmdd-HHMM>.md`。

## 硬规则

1. **授权范围内**：只处理用户明确给出的目标；发现同 IP 的其它业务，先报告，不擅自扩展。
2. **只读**：GET/HEAD；禁止利用、爆破、上传、写入目标。
3. **先验证后入库**：未验证的候选只出现在报告里，不进资产图。
4. **不重复**：入库前查重；同一 `ip:port` 不重复建服务。
5. **不烧额度**：先 stats 后 search；`size` 从小到大；同一 query 不反复拉。
6. **不泄密**：FOFA Key、Cookie、Token 不出现在输出、命令、报告里。
7. **不造假**：拿不到就写「未知/不可达」；不要用 FOFA 的历史状态冒充现状。

## 失败处理

| 情况 | 处理 |
|---|---|
| `stats -403` | 跳过 stats，改用小 size search |
| `search 820000` | 改 `mode="page"` |
| search 空结果 | 最多换一次同源 query（domain→cert/org），仍空则报告 |
| 额度不足 | 停止，报告已完成部分 |
| 全部 dead | 如实报告，不入库 |
| curl 被 WAF 拦 | 记 `403/406` 为存活，标题留空，标注「需人工确认」 |
