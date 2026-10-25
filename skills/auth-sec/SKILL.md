---
name: auth-sec
description: >-
  Entry P1 category router for authentication and authorization. Use when
  testing login flows, sessions, object authorization, JWT, OAuth, CORS, CSRF,
  and enterprise SSO weaknesses before any deeper auth topic skill.
  认证与授权分类路由：登录、会话、IDOR、JWT/OAuth、CSRF、CORS、SAML。
---

> **ARTEX 使用边界（优先于下文）**
> 仅在已授权、范围明确的目标上使用。主动探测前先读 `scan-opsec`，流量走
> `127.0.0.1:7891` 代理；默认只做只读、单点、低频、可回滚的最小验证。
> 禁止批量爆破/撞库、破坏性写入、真实支付或删除、写 webshell、反弹 shell、
> 污染共享连接池；发现异常立即停止。下文的完整攻击链只用于理解与构造最小证明。

# Authentication and Authorization Router

This is the routing entry point for authentication, sessions, and authorization boundaries.

Use it to decide whether the issue is mainly login mechanics, object-level authorization, browser trust boundaries, or identity protocols such as OAuth/JWT/SAML before going deeper.

## When to Use

- The target includes login, registration, password reset, 2FA, sessions, JWT, OAuth, or SSO
- You suspect object authorization flaws, cross-tenant access, cross-origin reads, CSRF, or protocol misconfiguration
- You need to decide whether to test authentication or authorization first

## Skill Map

- [Authentication Bypass](../authbypass-authentication-flaws/SKILL.md): login bypass, password reset, 2FA, enumeration, brute-force protections
- [IDOR Broken Object Authorization](../idor-broken-object-authorization/SKILL.md): IDOR, BOLA, BFLA, missing object permissions
- [JWT OAuth Token Attacks](../jwt-oauth-token-attacks/SKILL.md): algorithm confusion, key trust issues, claim abuse, token forgery
- [OAuth OIDC Misconfiguration](../oauth-oidc-misconfiguration/SKILL.md): redirect URI, state, nonce, PKCE, account binding
- [CSRF Cross Site Request Forgery](../csrf-cross-site-request-forgery/SKILL.md): CSRF tokens, SameSite, JSON CSRF, login CSRF
- [CORS Cross Origin Misconfiguration](../cors-cross-origin-misconfiguration/SKILL.md): reflected Origin, credentialed cross-origin reads, allowlist bypass
- [SAML SSO Assertion Attacks](../saml-sso-assertion-attacks/SKILL.md): assertion wrapping, signature validation, audience, ACS boundaries

## Recommended Flow

1. First confirm the authentication model and session boundaries
2. Then confirm object-level and function-level authorization
3. Then move to token, cross-origin, and protocol details
4. If enterprise federation exists, continue with OAuth, OIDC, or SAML topics

## Related Categories

- api-sec（未安装）
- Default credentials, username variants, wordlist sizing, and port focus are consolidated in [authbypass-authentication-flaws](../authbypass-authentication-flaws/SKILL.md)