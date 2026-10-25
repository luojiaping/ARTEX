---
name: asset-acquisition
description: 需求驱动的资产获取闭环：把用户的资产需求翻译成 FOFA 查询 → 调用 API 返回资产列表 → 逐条验证在线 → 去重后写入 ARTEX 资产图 → 输出可复核报告。适用于“找某类资产”（开放目录、LLM 服务、面板、数据库、VPN、摄像头等）或“按范围扩线”（域名/IP/组织/ICP/证书）。
license: MIT
compatibility: 需要 fofa_account / fofa_stats / fofa_search / fofa_host 与 insert_assets / list_assets / list_companies 工具；Bash 需可执行 curl。
---

# 需求驱动的资产获取闭环（asset-acquisition）

用户要的不是“查得多”，而是“按需求找到**特定的一类资产**，确认在线，再入库”。
FOFA 只负责给候选；在线验证和去重入库必须自己做。

## 输入类型

| 类型 | 例子 | 处理 |
|---|---|---|
| 直接查询语句 | `title="Index of /" && country="CN"` | 原样使用，先做语法检查 |
| 自然语言需求 | “找国内开放目录”“找暴露的 Ollama”“找 8080 端口的 x-ui” | 解析成查询 |
| 范围 + 类型 | “example.com 下的 HTTP 服务” | 组合 `domain=` + `port/protocol` |

## 需求 → 查询

从需求里提取维度，**有就加，没有就不臆造**：

| 维度 | 字段 | 示例 |
|---|---|---|
| 资产特征 | `title` / `body` / `header` / `banner` / `app` / `server` / `icon_hash` / `fid` / `cert` | `title="Index of /"` |
| 协议/端口 | `protocol` / `port` | `port="8080"` |
| 地域 | `country` / `region` / `city` | `country="CN"` |
| 归属 | `asn` / `org` / `icp` | `org="Example Inc"` |
| 范围 | `domain` / `ip` / `cert` | `domain="example.com"` |
| 时间 | `after` | `after="2026-01-01"` |

**选择特征的原则**：

1. 高选择性优先：`icon_hash` / `fid` / `cert` / 专有 `header` / `title` > 通用 `body` / `banner`。
2. 单特征结果过大时加第二特征降噪（如 `title="Index of /" && header="Server: nginx"`）。
3. 禁止百万级裸查：`body="admin"`、`title="login"`、`body="<!DOCTYPE html>"`。
4. 需求太泛（“找服务器”“找网站”）→ 先问一个澄清问题（类型/端口/国家/指纹）。
5. 用户说的是某类系统 → 先用 `fofa-recon` 的 `assets/queries/` 配方：
   - 开放目录：`title="Index of /"`、`body="Directory listing for"`、`header="SimpleHTTP"`
   - LLM：`body="Ollama is running"`、`title="Open WebUI"`、`app="vLLM"`、`title="one-api"`
   - 面板/代理：`title="x-ui"`、`title="3x-ui"`、`title="V2Board"`、`title="Marzban"`
   - 证书/图标：`cert="..."`、`icon_hash="..."`

常见需求模板见 `fofa-recon` 的 `assets/queries/open-directory.txt`、`ai-services.txt`、`panels-proxies.txt`。

## 执行流程

### 1) 复述需求 + 给出查询

先把“你要找什么”复述成一句人话，再给出**一条** FOFA 查询语句。
如果需求里有歧义（比如“找摄像头”但没给国家/品牌/端口），先问一句再查。

### 2) 查额度

```
fofa_account()
```

`remain_api_query < 200` 时先告知用户，询问是否继续。

### 3) 估规模

```
fofa_stats(query=query, fields="port,title,server,domain", size=5)
```

- `-403`：账号没开通 stats，**跳过，不重试**，改用 `size=50` 试拉。
- 估算 > 5000 条：先确认是否继续，或收窄查询。
- 记住：`search` 不返回命中总数，别把返回的 `size` 当总量。

### 4) 返回资产列表

```
fofa_search(
  query=query,
  fields="host,ip,port,domain,title,server,protocol,icp,country,province,city,asn,org,link",
  size=50,
  mode="page"
)
```

- 需要更多：`mode="next"`，首次 `next` 留空，之后填返回的游标。
- 单次 `size ≤ 500`；不要 10000。
- 按 `host:port` 去重；同一 IP 的多个端口保留多条候选。
- **在对话里输出列表**（host:port / title / server / 国家），最多 50 行；更多就只给统计。
- 用户说“先看列表/先别验证”时，**停在这里等确认**。

### 5) 验证在线（必须）

对每个唯一 `host:port`，用 Bash + curl 做只读验证：

```bash
curl -k -sS -L --max-time 8 -o /dev/null \
  -w '%{http_code}\t%{content_type}\t%{size_download}\t%{url_effective}\n' \
  "https://HOST:PORT/"
```

批量规则：

- 并发 ≤ 8，分批执行，批间 `sleep 1`。
- https 失败再试 http；都失败记 `OFFLINE`。
- 只请求根路径 `/`；不要访问 `/admin`、`/api` 等路径。
- 不要用 nmap 扫端口；端口只信 FOFA 给出的结果。
- `200/301/302/401/403/404` 都算在线；只有连不上才是离线。
- 结果写到 `verify-<slug>.tsv`，列：`host:port, code, content_type, size, final_url`。

### 6) 去重与导入

只导入**在线**资产。先查重：

```
list_assets(dsl="ip=\"1.2.3.4\"")
list_assets(dsl="domain=\"app.example.com\"")
```

字段映射：

| 验证结果 | 资产类型 | 字段 |
|---|---|---|
| 纯 IP | `ip` | `ip`、`open_ports[{port,service}]`、`bound_domains` |
| 域名/子域 | `subdomain` | `domain`、`record_value=[ip]`、`record_type="A"` |
| 主域 | `root_domain` | `domain`、`icp` |
| HTTP/HTTPS 服务 | `service` | `url`（优先 `link` 或验证后的 final_url）、`service_ip`、`page_title`、`technologies=[server]`、`status_code`、`content_length` |
| 非 HTTP 服务 | `service` | `service_name`（ssh/mysql/rdp…）、`port`、`ip` 或 `domain` |

`insert_assets` 每批 ≤ 50 条。`status_code` / `content_length` / `technologies` 必须来自本次验证，不要抄 FOFA。

### 7) 交付

```markdown
## 资产获取报告 · <需求>

- 查询：<FOFA query>
- 命中：<n 或 未知>
- 返回列表：<n>
- 在线：<n> / 离线：<n>
- 入库：<n>（ip <a> / subdomain <b> / service <c>）
- 跳过：<n>（重复 <x> / 未验证 <y> / 范围外 <z>）

| 类型 | 标识 | 端口 | 标题 | 状态码 |
|---|---|---|---|---|
| service | https://... | 443 | ... | 200 |

证据：`verify-<slug>.tsv`
```

同时写到当前工作目录的 `fofa-run-<YYYYmmdd-HHMM>.md`。

## 停止条件

- 用户说“只查 / 先别验证 / 先别入库” → 停在该阶段
- 用户说“查完给我看 / 先返回列表” → 返回列表后停止，等确认
- 额度不足 → 停止，报告已完成部分
- 全部离线 → 如实报告，不硬凑

## 硬规则

1. **授权范围内**：只处理用户明确给出的目标；发现范围外的资产只报告，不探测。
2. **只读**：GET/HEAD；禁止利用、爆破、上传、写入目标、全端口扫描。
3. **先验证后入库**：未验证的候选只出现在报告里，不进资产图。
4. **不重复**：入库前查重；同一 `ip:port` 不重复建服务。
5. **不烧额度**：先 stats 后 search；`size` 从小到大；同一 query 不反复拉。
6. **不泄密**：FOFA Key、Cookie、Token 不出现在输出、命令、报告里。
7. **不造假**：拿不到就写「未知/离线」；不要用 FOFA 的历史状态冒充现状。

## 失败处理

| 情况 | 处理 |
|---|---|
| `stats -403` | 跳过 stats，改用小 size search |
| `search 820000` | 改 `mode="page"` |
| search 空结果 | 最多换一次同源 query，仍空则报告 |
| 额度不足 | 停止，报告已完成部分 |
| 全部离线 | 如实报告，不入库 |
| curl 被 WAF 拦（403/406） | 记为在线、标题留空，标注「需人工确认」 |
