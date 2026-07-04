# Security Policy

## Supported Versions

Rekenraam is in early development. Security fixes are applied to the active
main branch unless a release process documents supported versions separately.

## Reporting a Vulnerability

Please do not report suspected vulnerabilities in public issues or discussions.

Use GitHub's private vulnerability reporting for this repository:

https://github.com/sergeyfarin/rekenraam/security/advisories/new

Include a clear description of the issue, reproduction steps or proof of
concept if available, affected versions or commits, and any impact you have
observed. Please avoid accessing, modifying, or deleting data that is not yours.

We will acknowledge reports as soon as practical, investigate privately, and
coordinate disclosure after a fix or mitigation is available.

## Repository Security Controls

This repository keeps Dependabot version updates in `.github/dependabot.yml` and
runs `govulncheck` for the Go backend in `.github/workflows/govulncheck.yml`.

Repository administrators should also enable the strongest GitHub secret
scanning and push-protection controls available for the repository before making
the repository public. If the repository is user-owned and private, GitHub may
not show a repository-level Secret Protection enablement button unless the
account is covered by GitHub Advanced Security/Enterprise Managed Users; verify
the controls again after the repository becomes public or move the repository
under an organization with Secret Protection available.

Admin setup checklist:

1. Open the repository on GitHub.
2. Go to Settings -> Advanced Security.
3. If "Secret Protection" is available, enable it for the repository.
4. If repository-level push protection is available, enable it under the secret
   scanning or Secret Protection settings.
5. If those controls are not available while the repo is private and
   user-owned, note that GitHub runs secret scanning automatically for public
   repositories and user push protection is enabled by default for pushes to
   public repositories; re-check these controls immediately after the repo is
   public.
6. Confirm Dependabot alerts and security updates are enabled for the
   repository.
