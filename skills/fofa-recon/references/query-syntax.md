# FOFA 查询语法

> 未标「推断」者均有原文示例，出处见文末。

## 1. 基本形式

- `field="value"`：包含匹配。`title="chatgpt"` 即标题含该词的站点（[入门]）。
- `field=="value"`：精确匹配。`title=="myLinx"`、`domain=="fofa.info"`；语义原文未写，**推断**为全等。
- `field*="value"`：启用模糊/通配。`host*="test*.baidu.com"`。
- `field!="value"`：排除。`banner="ZXR10" && banner="Router" && banner!="Switch"`（[白皮书]）。
- `field=""` 判空，`field!=""` 判非空：`icon_hash=""`、`domain!=""`、`cname!=""`。
- 同字段可重复，表示同时满足：`banner="ZXR10" && banner="Switch"`。
- `&&` 与、`||` 或；原文混用时均显式加括号：`(host="deepseek" || title="deepseek") && type="subdomain"`（[DeepSeek]）。优先级原文未写，**推断** `&&` > `||`，建议一律显式括号。
- 引号：默认双引号；值内含 `"` 时用 `\"`：`body="href=\"/themes/Gargoyle/images/favicon.png\""`；也可外层单引号：`header='Alt-Svc: h3=":443"'`；二进制用 `\x15\x03`（[白皮书][APT-C-23]）。
- 大小写：字段名恒小写；值的大小写原文未说明，**推断**按页面原文书写，敏感性待验证。

## 2. 操作符语义表

| 写法 | 含义 | 示例 | 注意 |
|---|---|---|---|
| `=` | 包含（分词后匹配） | `title="chatgpt"` | 受分词影响，最常用 |
| `==` | 精确全等（推断） | `title=="myLinx"` | 原文仅用例 |
| `*=` | 开启模糊匹配 | `host*="test*.baidu.com"` | `*=` 本身不是通配符 |
| `!=` | 排除/不等 | `banner!="Switch"` | 可配 `""` 判空 |
| `=""` | 字段为空 | `icon_hash=""` | 常用于排除有图标资产 |

## 3. 模糊搜索

- `*` 匹配 0 到多个字符；`?` 只匹配 1 个字符（[模糊]）。
- `*=` 只是"启用模糊"的标志，真正通配的是引号内的 `*` / `?`（[模糊]）。

| 示例 | 效果 |
|---|---|
| `host*="test*.baidu.com"` | test 后可有任意后缀 |
| `host*="*test*.baidu.com"` | test 前后均可是任意字符 |
| `host*="test?.baidu.com"` | test 后**恰好一个**字符 |
| `host*="?????.baidu.com"` | `.` 前**恰好五个**字符 |
| `banner="mysql version" && banner*="5.5.4*"` | 版本以 5.5.4 开头（含 5.5.4x，需复核） |
| `protocol*="snmp*"` | 一次覆盖 snmp 各版本 |
| `domain="fofa.info" && port*="????"` | 只留四位端口 |

- 适用：分词不完整（banner 中 `0ubuntu0` 导致搜不到 `ubuntu`）、版本号、协议族、云厂商名。
- 不适用：`host*="*test*"` 这类前置通配原文自述"范围非常大"，仅适合拓线灵感；生产查询先用精确条件收敛。模糊搜索需会员或 F 点（[模糊]）。

## 4. 类型与范围

- `type="subdomain"` = 网站（HTTP/HTTPS，含 HTML 源码）；`type="service"` = 服务（所有协议，HTTP 不含源码）。`port="80" && type="service"` 与 `port="80" && type="subdomain"` 可直接分开两者（[白皮书]）。
- 规则提取可用字段：服务仅 `banner`/`cert`/`protocol`；网站为 `server`/`title`/`body`/`header`/`cert`（[白皮书]，仅指可录规则）。
- 地理与归属：`country`、`region`、`city`、`org`（ASN 组织名，如 `CLOUDFLARENET`）、`cloud_name`。
- 时间：`after="2023-08-01"`（[Bitter]）。
- 证书：`cert`（可命中 CN、组织、有效期字符串）、`cert.is_valid`、`cert.is_match`、`cert.is_expired`、`cert.subject.*`、`cert.issuer.*`。
- 指纹：`icon_hash`、`fid`、`app`、`js_name`、`js_md5`、`sdk_hash`、`jarm`、`header_hash`、`body_hash`。
- `asn`、`icp` 本批资料无原文用例，列入 field-reference「待验证」。

## 5. 组合查询范式（10 条）

1. **品牌面收敛**：`(host="deepseek" || title="deepseek") && type="subdomain"`
   目的：找品牌相关站点。OR 括起避免与 type 抢优先级，再限定网站类，排除服务噪声。（[DeepSeek]）
2. **同源框架站点**：`body="loading-wrap" && body="balls" && is_domain=true && cert.is_valid=true`
   目的：两个 body 特征同时命中，排除单词偶合；再要求有域名+可信证书，滤掉临时/自签站。（[入门]）
3. **证书归属确认**：`cert="fofa.info" && cert.is_valid=true && cert.is_match=true`
   目的：证书受信且与域名匹配，剔除蹭证书、错绑 DNS 的干扰资产。（[证书]）
4. **自有旧证书回收**：`cert="abchina.com.cn" && cert.is_valid=true && cert.is_match=false`
   目的：内部域名未写进证书但私钥在手，仍属自身资产；只看 match=true 会漏。（[证书]）
5. **JARM + 响应头聚类**：`jarm="3fd3fd0003fd3fd21c3fd3fd3fd3fd703dc1bf20eb9604decefea997eabff7" && header="HTTP/1.1 404 Not Found" && header="Server: nginx" && header="Content-Type: text/html" && (header="Content-Length: 183" || header="Content-Length: 535")`
   目的：TLS 指纹定家族，header 组合定默认页模板；长度用 OR 容纳两个版本。（[响尾蛇]）
6. **ASN 段 + 跳转特征**：`org="BLNWX" && header="Location: http://www.google.com" && header="Server: nginx/1.18.0 (Ubuntu)" && domain!=""`
   目的：先圈 ASN，再用跳转目标与精确版本锁定同一部署。（[威胁情报]）
7. **版本收敛**：`banner="mysql version" && banner*="5.5.4*"`
   目的：先限定 MySQL banner，再用 `*=` 取版本前缀；比 `=` 更能覆盖分词不完整的版本串。（[模糊]）
8. **非 HTTP 协议指纹**：`banner="\x15\x03\x03\x00\x02\x022" && port="3000"`
   目的：二进制 banner 定位异常 TLS 响应，端口收窄误报。（[COLDRIVER]）
9. **证书字段 + 空值排除**：`cert.subject.cn*="*.*" && cert.is_valid=true && cert.is_expired=false && icon_hash="" && cname!=""`
   目的：主体必须是域名、证书有效未过期、无 favicon、有 CNAME——典型空默认页基础设施。（[Bitter]）
10. **多区域 OR + 排除官方域**：`title="deepseek" && (region="HK" || country="US") && domain!="deepseek.com"`
    目的：区域用括号 OR，最后排除官方域名，避免把自家资产当仿冒。（[DeepSeek]）

## 6. 常见错误与降噪

- **模糊导致爆炸**：`banner="404 Not Found"` 单独查有 316,493,580 条（[威胁情报]）；必须叠加 header/port/jarm 等稳定特征。
- **前置通配**：`host*="*test*"` 原文自述结果很宽（[模糊]）；只用于发现线索，不作最终清单。
- **证书噪声**：`cert="x"` 只说明字符串出现在证书里；按 `is_valid` → `is_match` → `is_expired` 逐层判断。
- **标题/正文误报**：`body="corcloud"` 会命中其官网与非本产品资产，且 `title` 可被仿冒（[白皮书]）；用多特征+证书交叉。
- **分词坑**：`.` `:` `-` `/` 是分词符；用 `_` 等分隔的词可能整体成词而搜不到（[白皮书] FAQ）；换关键词或改 `*=`。
- **返回 -1（推断，原文无记载）**：常见于引号/括号未闭合、全角标点（原文示例中即出现过全角 `）`）、运算符误写（`=*`、`===`）、字段名不存在、布尔字段给了非 `true/false` 值。用最短条件逐步追加定位。

## 7. 来源

- [白皮书] `Cheatsheet/FOFA规则白皮书.md`：L48-56、L82-86、L153-239、L283-379、L490-496
- [模糊] `Basic scenario/The correct way to use Fuzzy Search in FOFA.md`：L12-111
- [证书] `Basic scenario/How to effectively eliminate interference through certificates in the asset discovery.md`：L6-60
- [入门] `Get Started with FOFA/A Beginner's Guide.md`：L23-46
- [Bitter] `Basic scenario/Conducting APT Bitter Tracking Operation Using FOFA.md`：L74-117
- [响尾蛇] `Basic scenario/Practical FOFA Asset Expansion- Sidewinder APT.md`：L56-114
- [威胁情报] `Basic scenario/A Comprehensive Guide to Threat Intelligence Discovery Using FOFA.md`：L186-204、L328-433
- [DeepSeek] `Basic scenario/Practical Guide to Discovering DeepSeek Impersonation Assets Using FOFA.md`：L11-14、L296
- [COLDRIVER] `Basic scenario/Practical FOFA Asset Discovery- COLDRIVER.md`：L35-71
- [APT-C-23] `Basic scenario/Practical FOFA Asset Expansion- APT-C-23 Android Malware.md`：L47-60（转义）
