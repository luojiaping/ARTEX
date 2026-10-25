---
name: hack
description: >-
  授权 Web / API 渗透测试总路由。已经拿到目标、需要判断先测什么，或在 SQLi、
  XSS、SSRF、XXE、SSTI、命令注入、NoSQL、越权、认证、业务逻辑、文件上传、
  请求走私、WebSocket 之间选择下一步时使用。中文触发：渗透测试、挖洞、漏洞挖掘、
  Web 安全评估、API 安全测试、SRC、CTF、漏洞验证、安全审计。
---

# 授权 Web / API 安全测试总路由

> 本文件的路由结构和部分实战直觉来自 `yaklang/hack-skills`（MIT）。
> 这里只保留已安装的专题，并按 ARTEX 的授权与最小验证边界改写。

## 先决条件

- 目标、范围、授权人明确；范围不清先问，不猜。
- 主动探测前先读 `scan-opsec`，出口走 `127.0.0.1:7891` 代理。
- 默认只做只读、单点、低频、可回滚的最小验证；不批量爆破、不破坏写入、
  不真实支付/删除、不落 webshell、不反弹 shell。
- 每条结论区分「已确认（证据）」与「假设（待验证）」。

## 测试顺序

1. **范围与资产**：`asset-acquisition` / `fofa-recon`（已有）→ 确认活资产。
2. **前端与接口面**：`api-recon`（已有）→ JS、路由、API、参数、鉴权头。
3. **认证与对象授权**：`auth-sec` → 登录、会话、IDOR、JWT/OAuth、CSRF、CORS、SAML。
4. **注入**：`injection-checking` → 按最终汇聚点选 SQLi/XSS/SSRF/XXE/SSTI/CMDi/NoSQL。
5. **文件**：`file-access-vuln` → 路径遍历/LFI、上传校验与处理链。
6. **业务逻辑**：`business-logic-vuln` → 价格、优惠券、库存、状态机、竞态。
7. **协议层**：`request-smuggling`、`websocket-security`。
8. **凭据与 AI**：`credential-hunting`、`llm-key-verification`、`secknowledge-skill`。

## 路由表

| 观察到的信号 | 下一步 |
|---|---|
| 输入回显到 HTML/JS，或模板表达式被执行 | `xss-cross-site-scripting` / `ssti-server-side-template-injection` |
| 输入进入 SQL 查询、排序、筛选、报表 | `sqli-sql-injection` |
| 服务端主动请求 URL、导入远程内容 | `ssrf-server-side-request-forgery` |
| 接受 XML、SVG、OOXML、SOAP | `xxe-xml-external-entity` |
| 输入进入 shell、转换器、导入/预览管线 | `cmdi-command-injection` |
| JSON/查询对象出现 `$ne`、`$gt`、`$where` | `nosql-injection` |
| 路径、文件名、下载、包含参数可控 | `path-traversal-lfi` |
| 上传、预览、转码、解压、共享链接 | `upload-insecure-files` |
| 请求里出现对象 ID、租户 ID、角色字段 | `idor-broken-object-authorization` |
| 登录、重置、MFA、会话、账号接管 | `authbypass-authentication-flaws` |
| JWT、Bearer、`alg`/`kid`/`jku` | `jwt-oauth-token-attacks` |
| OAuth/OIDC 的 redirect_uri、state、PKCE、nonce | `oauth-oidc-misconfiguration` |
| 状态变更请求、SameSite、CSRF token | `csrf-cross-site-request-forgery` |
| `Access-Control-Allow-Origin` / 凭据跨域读 | `cors-cross-origin-misconfiguration` |
| SAML 断言、ACS、签名校验 | `saml-sso-assertion-attacks` |
| 前后端对 CL/TE 或 HTTP/2 边界理解不一致 | `request-smuggling` |
| WebSocket 升级、Origin 校验、实时消息 | `websocket-security` |
| 优惠券、库存、余额、一次性操作 | `race-condition` / `business-logic-vulnerabilities` |

## 核心直觉

1. 同一套过滤逻辑常在多个页面复用；一处能绕，其他同构页面通常也能绕。
2. 参数名也是攻击面：WAF 经常只看值，不看名。
3. 二阶漏洞很常见：存储时安全，不代表之后被读入危险上下文时安全。
4. BOLA 的本质是「已登录但未授权」；A/B 双账号重放是最稳的证明。
5. 旧 API 版本经常漏补丁；修了 v2 不代表 v1 下线。
6. 业务逻辑漏洞扫描器看不见，往往停留时间最长、影响最大。
7. 竞态优先打一次性动作：领券、积分、重置、邀请、试用、库存扣减。
8. JWT 先看 `alg`、`kid`、JWKS 和密钥来源，不要盲目喷 payload。

## 未安装的专题

原型污染、类型混淆、HTTP 参数污染、XSLT、CSV 公式、反序列化、JNDI、
表达式语言注入、CRLF、依赖混淆、子域接管、AD/移动/Pwn/逆向/密码学/取证
尚未导入。遇到这些场景时，先按通用方法论做最小验证，或另行导入对应专题；
不要把未安装的 skill 名当作可调用工具。
