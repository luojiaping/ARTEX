# FOFA 字段参考

> 示例均取自 Awesome-FOFA 原文；「待验证」表示本批资料未出现该字段的查询示例。

## 字段总表

| 字段 | 含义 | 适用协议/类型 | 示例查询 | 备注 |
|---|---|---|---|---|
| `host` | 主机名（域名/子域） | 网站·服务 | `host*="*test*.baidu.com"` | 可用模糊匹配子域；`host="deepseek"` 命中含词域名 |
| `ip` | IP 地址 | 全部 | `ip="211.99.98.76"` | 单 IP 精确定位 |
| `port` | 开放端口 | 全部 | `port="80" && type="service"` | 可 `port*="????"` 找四位端口；原文亦有 `port=22` 不带引号 |
| `domain` | 域名 | 网站 | `domain="fofa.info" && port*="????"` | `domain!="deepseek.com"` 排除官方域 |
| `protocol` | 协议名 | 服务 | `protocol="telnets"` | `protocol*="snmp*"` 覆盖 snmp 各版本 |
| `title` | 网页标题 | 网站 | `title="chatgpt"` | `title=="myLinx"` 为精确写法 |
| `server` | Server 响应头 | 网站 | `server="LiteSpeed"` | 检索效率高于 header（白皮书） |
| `header` | HTTP 响应头全文 | 网站 | `header="Content-Length: 1229"` | 可多条 AND；含双引号时外层用单引号 |
| `banner` | 协议 banner/版本 | 服务（HTTP 亦有） | `banner="MikroTik"` | 支持 `\x15\x03` 字节转义 |
| `body` | HTML 正文 | 网站 | `body="loading-wrap"` | 多关键词须同时 AND，单关键词易误报 |
| `cert` | 证书全文 | 网站·TLS | `cert="AirControl"` | 可命中 CN、组织、有效期字符串 |
| `cert.is_valid` | 证书是否受信 | TLS | `cert.is_valid=true` | false=不受信/自签 |
| `cert.is_match` | 证书是否匹配域名 | TLS | `cert.is_match=true` | false 可能是自有旧证书 |
| `cert.is_expired` | 证书是否过期 | TLS | `cert.is_expired=false` | true=已过期 |
| `cert.subject` | 证书主体 | TLS | `cert.subject.cn*="*.*"` | 子字段 `cn`、`org` |
| `cert.issuer` | 证书颁发者 | TLS | `cert.issuer.cn="R3"` | 子字段 `cn`、`org`；`!=""` 判非空 |
| `icon_hash` | favicon 哈希 | 网站 | `icon_hash="-118645384"` | 唯一性强；`icon_hash=""` 排除有图标资产 |
| `fid` | FOFA 聚合特征 ID | 网站 | `fid="qObSo5JrR9FIkIqprX7XKg=="` | 同模板站点聚类 |
| `app` | 应用/产品名 | 网站·服务 | `app="COBALTSTRIKE-Team Server"` | 示例值形如 FOFA 规则名（推断） |
| `os` | 操作系统 | 服务 | `os="xxxxxxxxxx"` | 原文仅占位示例 |
| `country` | 国家 | 全部 | `country="US"` | `country!="CN"` 可排除 |
| `region` | 省/地区 | 全部 | `region="HK"` | 可与 country 组合 |
| `city` | 城市 | 全部 | `city="xxxxxxx"` | 原文仅占位示例 |
| `org` | 组织（ASN 组织名） | 全部 | `org="CLOUDFLARENET"` | 示例另有 `AMAZON-02` |
| `cloud_name` | 云厂商 | 全部 | `cloud_name*="Cloud*"` | 模糊匹配 Cloudflare |
| `type` | 资产类型 | 全部 | `port="80" && type="subdomain"` | `service`=服务，`subdomain`=网站 |
| `js_name` | JS 文件名 | 网站 | `js_name="static/js/quanzhan.js"` | 同团伙模板特征 |
| `js_md5` | JS 内容 MD5 | 网站 | `js_md5="a13f7f208ba534681deadb1ec7a2e54a"` | 供应链定位常用 |
| `sdk_hash` | SDK 特征哈希 | 网站 | `sdk_hash="5IcXyBJ8QrxlDLQFTl2DCHG0Z42JHfk6"` | 原文仅给用例（推断为 SDK 指纹） |
| `header_hash` | 响应头哈希 | 网站 | `header_hash="-324809210"` | 与 header 组合降噪 |
| `body_hash` | 正文哈希 | 网站 | `body_hash="-1840324437"` | 整页正文指纹 |
| `jarm` | TLS 服务端指纹 | 服务·TLS | `jarm="3fd3fd0003fd3fd21c3fd3fd3fd3fd703dc1bf20eb9604decefea997eabff7"` | 换 IP/域名仍可追踪 |
| `cname` | CNAME 记录 | 网站·DNS | `cname!=""` | 判断 CDN/别名 |
| `is_domain` | 是否绑定域名 | 网站 | `is_domain=true` | 过滤纯 IP 站点 |
| `status_code` | HTTP 状态码 | 网站 | `status_code="200"` | 也可用 404 定位默认页（推断） |
| `after` | 更新时间下界 | 全部 | `after="2023-08-01"` | 取近期数据 |
| `lastupdatetime` | 最后更新时间 | 全部 | —（无查询示例） | 仅字段清单 |
| `tls_ja3s` | JA3S 指纹 | 服务·TLS | —（无查询示例） | 仅字段清单 |
| `tls_version` | TLS 版本 | 服务·TLS | —（无查询示例） | 仅字段清单 |

## 待验证字段

| 字段 | 推测含义 | 状态 |
|---|---|---|
| `base_protocol` | 基础协议（TCP/UDP 等） | 模板提及，本批资料无出处 |
| `banner_hash` | banner 哈希 | 同上 |
| `product` | 产品名 | 同上 |
| `category` | 产品分类 | 同上 |
| `icp` | ICP 备案号 | 同上 |
| `asn` | AS 编号 | 同上；已证的 `org` 可查 ASN 组织名 |
| `link` | 外链/跳转链接 | 同上 |
| `before` | 更新时间上界 | 仅见 `after`，未见 `before` |
| `certs_subject_cn`、`certs_subject_org`、`certs_issuer_cn`、`certs_issuer_org` | 证书主体/颁发者字段 | 字段清单如此写，但全部实际示例用 `cert.subject.cn`、`cert.issuer.cn` 等，**以点号写法为准** |

## 易混字段

- `domain` vs `host`：原文未下定义。示例中 `domain="fofa.info"` 用于域名筛查，`host` 可带子域通配（`host*="*test*.baidu.com"`）；**推断** `domain` 为域名、`host` 为主机名。
- `server` vs `header`：`server` 是 header 中的一个字段，检索效率更高；白皮书又建议录规则时用 `header` 更精准。快筛用 `server`，定指纹用 `header`。
- `cert` vs `cert.*`：`cert="..."` 是整段证书全文（可命中 CN、组织、有效期）；同一 `cert` 可重复 AND，如 `cert="Internet Widgits Pty Ltd" && cert="2023-06-23 15:59 UTC"` 表示同一张证书同时含组织与有效期（原文示例）。子字段 `cert.subject.cn` 等适合精确化；`cert.subject.cn*="*.*"` 表示要求 CN 含点、像域名。
- `banner` vs `body`：`banner` 是协议原始返回（以服务为主，HTTP 响应行也可能出现），`body` 是解析后的 HTML 正文（网站）。
- `type`：`service` 与 `subdomain` 是同一批数据的两种视角（协议服务 / 网站），过滤前先确定要哪类。

## 来源

- 字段定义与可用范围：`Cheatsheet/FOFA规则白皮书.md` L74-86、L153-379、L490-496
- 字段清单：`Basic scenario/A Comprehensive Guide to Threat Intelligence Discovery Using FOFA.md` L186-204
- 模糊字段用法：`Basic scenario/The correct way to use Fuzzy Search in FOFA.md` L12-111
- 证书字段：`Basic scenario/How to effectively eliminate interference through certificates in the asset discovery.md` L6-60
- 类型区分：`Cheatsheet/FOFA规则白皮书.md` L48-72
- 空值/哈希/JARM：`Basic scenario/Practical FOFA Asset Expansion：Ducktail.md` L84-146
- 证书 CN/ASN 组合：`Basic scenario/Conducting APT Bitter Tracking Operation Using FOFA.md` L74-117
- 同一证书多条件：`Basic scenario/Practical FOFA Asset Discovery- COLDRIVER.md` L61-71
- 银狐字段组合：`Basic scenario/Practical FOFA Asset Expansion- Unmasking the Trail of the 'Silver Fox' APT.md` L37-199
- 大字段导出上限：`Basic scenario/Best Approach for Fetching Large Amounts of Asset in FOFA.md` L14

## 字段选择建议

- **资产盘点**：先用 `domain`/`host`/`ip`/`port`/`type`/`country`/`org` 圈范围，再用 `cert.*` 与 `icon_hash` 下钻；不要一上来就查 `body`。
- **指纹识别**：优先 `icon_hash`、`fid`、`app`、`js_md5`、`js_name`、`jarm`、`header_hash`、`body_hash`、`banner`。原文强调按唯一性、稳定性、可探测性、相关性选特征。
- **证书拓线**：`cert` 先捞字符串，再用 `cert.subject.*`/`cert.issuer.*` 精确化，最后用 `cert.is_valid`、`cert.is_match`、`cert.is_expired` 判定归属；证书相对稳定，适合作长期线索。
- **成本敏感**：`banner`/`header`/`cert`/`body`/`structinfo`/`icon` 属大字段，原文注明含这些字段导出时单次最多 10 万条；先用 `title`/`server`/`port`/`ip`/`domain` 等小字段收敛，再取大字段（推断：可减少传输与配额消耗）。
