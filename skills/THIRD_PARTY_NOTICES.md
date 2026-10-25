# Third-party skill notices

This directory contains skills imported from third-party MIT-licensed
collections. The upstream copyright and permission notices are reproduced
below. Modifications are limited to Chinese trigger text, ARTEX safety
boundaries, router trimming, removal of machine-local files, and small fixes
to unresolved placeholders or destructive examples.

## yaklang/hack-skills

- Source: https://github.com/yaklang/hack-skills
- Snapshot: `main` at `6fbf0bc8d5c71830d62543a308d8744606c43d7c`
- License: MIT
- Copyright (c) 2026 VillanCh
- Imported subset: `hack`, `auth-sec`, `injection-checking`,
  `file-access-vuln`, `business-logic-vuln`, `sqli-sql-injection`,
  `xss-cross-site-scripting`, `ssrf-server-side-request-forgery`,
  `xxe-xml-external-entity`, `ssti-server-side-template-injection`,
  `cmdi-command-injection`, `nosql-injection`, `path-traversal-lfi`,
  `upload-insecure-files`, `idor-broken-object-authorization`,
  `jwt-oauth-token-attacks`, `oauth-oidc-misconfiguration`,
  `csrf-cross-site-request-forgery`, `cors-cross-origin-misconfiguration`,
  `saml-sso-assertion-attacks`, `request-smuggling`, `websocket-security`,
  `race-condition`, `business-logic-vulnerabilities`,
  `authbypass-authentication-flaws`.
- Two optional companion files (`ssti-server-side-template-injection/
  SCENARIOS.md` and `upload-insecure-files/SCENARIOS.md`) could not be imported:
  the workstation antivirus blocked them as offensive payload corpora. The main
  skills state that those extended scenarios are not shipped.

## Pa55w0rd/secknowledge-skill

- Source: https://github.com/Pa55w0rd/secknowledge-skill
- Snapshot: commit `078de3baa6b325f5acc5338c62169532e8f922b4`
- License: MIT (declared in the upstream README)
- Copyright (c) Pa55w0rd
- Imported as `secknowledge-skill` (one entry + 48 references).
- The upstream `.git` directory was removed. The `flushall` Redis example in
  `references/web-ssrf-misc.md` was replaced with a non-destructive `PING`
  proof, and routes to skills that are not installed here were marked as such.

## MIT License

Copyright (c) 2026 VillanCh
Copyright (c) Pa55w0rd

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## Import review: rejected material

The following archives were inspected but not imported.

| Archive | Reason |
|---|---|
| `web-pentest` | No license/provenance. Its JS audit duplicates the existing `api-recon`, its credential detection overlaps `credential-hunting`, and its multi-agent roles do not map cleanly to a single ARTEX skill. |
| `api-security` | No license, no YAML frontmatter (so ARTEX cannot expose a description), and thinner than the existing `api-recon` plus the auth/injection skills. |
| `src-6k-skill` (`clown-src-6k`) | No license. It is a Grok machine-identity pack, not a portable skill: it depends on `~/.grok/rules`, carries a machine-local `config.toml`, and tells the agent to assume authorization instead of confirming it. |
| `教学文档` | Not a skill; it is a target-specific xTun reverse-engineering course with no license. |
| `hack-skills` API/recon topics | `api-sec`, `api-recon-and-docs`, `api-authorization-and-bola`, `api-auth-and-jwt-abuse`, and `graphql-and-hidden-parameters` are 48–76 line checklists that duplicate the much deeper existing `api-recon`. `recon-for-sec` and `recon-and-methodology` recommend masscan/nmap/ffuf active scanning without the `scan-opsec` egress rules, duplicating `asset-acquisition` and `fofa-recon`. |
| `hack-skills` AI topics | `llm-prompt-injection` and `ai-ml-security` are covered more broadly by `secknowledge-skill` (GAARM 173 + OWASP LLM/ASI). Importing both would duplicate AI-security triggers and push the skill listing over its discovery budget. |
