# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest  | Yes       |

## Reporting a Vulnerability

If you discover a security vulnerability in gocker, please report it responsibly.

**Do NOT open a public GitHub issue.**

Instead, use [GitHub's private vulnerability reporting](https://github.com/lunguini/gocker/security/advisories/new) and include:

- Description of the vulnerability
- Steps to reproduce
- Impact assessment (what an attacker could do)
- Suggested fix (if you have one)

We will acknowledge receipt within 48 hours and aim to provide a fix or mitigation within 7 days for critical issues.

## Scope

gocker's security surface includes:

- **Sandbox isolation** — the container boundary between the sandbox and the host. A sandbox escape would be critical.
- **Config sync** — host settings are mounted into containers. Sensitive data (API keys, auth tokens) must not leak to unintended containers.
- **Docker API daemon** — the Unix socket at `~/.gocker/gocker.sock` accepts unauthenticated requests. It should only be accessible to the current user.
- **Template images** — pre-built images on Docker Hub (`adyjay/gocker`) could be a supply chain vector if compromised.

## Security Model

gocker relies on Apple's `Virtualization.framework` for isolation — each container is a separate Linux VM, not a namespaced process. This provides stronger isolation than traditional containers, but:

- **The workspace is mounted read-write** into the sandbox. A malicious agent can modify your project files.
- **API keys are passed via environment variables.** They are visible inside the container.
- **Host Claude settings are synced** (read-only) into sandboxes. This includes plugin and marketplace references but not secrets.
- **The Docker API socket has no authentication.** Protect it with filesystem permissions (default: user-only).

## Hardening Recommendations

- Use `--network-policy deny --allow-host api.anthropic.com` to restrict sandbox network access (when available)
- Do not run `gocker daemon start` on multi-user systems without restricting socket permissions
- Review mounted settings before running untrusted agents (`--no-sync-config`)
- Pin template image tags rather than using `:claude-latest` in production
