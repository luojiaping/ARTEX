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
- Phase 2 subset (advanced web + host/AD): `insecure-source-code-management`,
  `subdomain-takeover`, `open-redirect`, `http-host-header-attacks`,
  `web-cache-deception`, `http-parameter-pollution`, `type-juggling`,
  `crlf-injection`, `csp-bypass-advanced`, `prototype-pollution`,
  `prototype-pollution-advanced`, `401-403-bypass-techniques`,
  `waf-bypass-techniques`, `unauthorized-access-common-services`,
  `jndi-injection`, `expression-language-injection`,
  `linux-privilege-escalation`, `windows-privilege-escalation`,
  `active-directory-kerberos-attacks`, `ntlm-relay-coercion`.
- The optional `unauthorized-access-common-services/PORT_SERVICE_MATRIX.md`
  companion was blocked by the workstation antivirus; the main skill notes that
  it is not shipped.

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
| `hack-skills` binary/RE/crypto/mobile/forensics/blockchain topics | No ARTEX agent consumes them today. They are high quality but would be dead weight; defer to a dedicated CTF/reversing agent pack. |
| `deserialization-insecure` | The 725-line document repeats its Ruby/.NET/Node sections twice (sections 8–10 and 11–13). It needs deduplication before import; `jndi-injection`, `expression-language-injection`, and `secknowledge` cover parts of the same surface. |
| `http2-specific-attacks` | Overlaps the already imported `request-smuggling/H2_SMUGGLING_VARIANTS.md` (H2.CL/H2.TE/downgrade). |
| `dns-rebinding-attacks` | Overlaps `ssrf-server-side-request-forgery/URL_PARSER_TRICKS.md` (DNS rebinding, TTL, IMDSv2). |
| `ghost-bits-cast-attack` | 800-line, highly specialized Java char-narrowing corpus with 10 references to uninstalled skills; defer until a Java-focused agent exists. |
| `xslt-injection`, `csv-formula-injection`, `email-header-injection`, `clickjacking`, `dangling-markup-injection`, `dependency-confusion` | Niche or low-impact for the current web/asset/credential agents; not worth the skill-listing budget. |
