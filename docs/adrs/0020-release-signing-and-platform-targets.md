# ADR 0020: Release Signing And Platform Targets

## Status

Accepted 2026-10-28 by the owner for
[#333](https://github.com/sergeyfarin/rekenraam/issues/333).
The owner moved the migration freeze to the first supported household
installation ([#576](https://github.com/sergeyfarin/rekenraam/issues/576)).

## Context

Rekenraam needs verifiable release artifacts without a permanent signing key
that a single maintainer must store, rotate and recover. A possible future
Wails edition needs Windows packaging and native publisher signing, while the
current product remains a Go server with an embedded frontend. Preparing the
release pipeline does not authorize publishing a release or accepting its
separate deployment, household and advisory gates.

## Decision

1. Use Sigstore keyless signing through GitHub Actions OIDC and Cosign. Sign a
   SHA-256 manifest of the finished downloads and release notes, retaining the
   Sigstore bundle. Verification must require the expected repository,
   workflow and ref identity, the GitHub Actions OIDC issuer, and matching file
   hashes. A signature from an unexpected identity must fail verification.
2. Initial server targets are Linux `amd64` and `arm64`, built natively on
   Ubuntu 24.04 runners with CGO and bundled SQLite. The declared runtime
   baseline is Ubuntu 24.04 with glibc 2.39, on the matching architecture;
   inspect ELF dependencies and reject a higher glibc requirement. Other
   distributions require their own compatibility evidence. This does not
   change the Debian trixie production-container baseline (ADR 0005).
3. Record source SHA, version, toolchains, runner image, dynamic dependencies,
   required glibc symbols, highest migration and every migration checksum.
   Compare repeated unsigned builds with identical inputs. Deterministic
   archives use the source commit time, stable file order and permissions;
   timestamped signatures are outside byte-for-byte reproducibility claims.
   Runner labels are not immutable toolchain pins; retain exact build inputs
   and the limits of same-environment evidence.
4. Prepare candidates through a manual, candidate-only workflow on `main` in
   `sergeyfarin/rekenraam`. Its identity is
   `https://github.com/sergeyfarin/rekenraam/.github/workflows/release-candidate.yml@refs/heads/main`,
   issued by `https://token.actions.githubusercontent.com`. It accepts only
   `vMAJOR.MINOR.PATCH-rc.N` labels, uploads Actions artifacts and has no
   contents-write permission. It never creates a Git tag or GitHub release.
   Candidate labels identify builds, not installed product releases.
5. Actual publication requires explicit owner instruction, green candidate
   validation and the retained release gates. Use a draft with all finished
   assets, then immutable publication. A future publication workflow must
   verify the approved source/tag, use an exact release-tag signing identity
   and retain the signature bundle. Do not claim a branch-signed candidate
   was signed under a release tag. The household preview uses the commit that
   freezes migrations on `main` or a later commit. Pin every appended
   migration as it lands and preserve the baseline generation so the household
   database upgrades in place.
6. Keep platform build/package steps separate from common manifest signing.
   A future Windows/Wails target adds native build and WebView2 installer
   validation, Authenticode signing and timestamping of the executable, then
   packaging and signing of its installer. Generate checksums and Sigstore
   signatures only after platform signing finishes. Select the Windows CA or
   managed signing provider then; today's Sigstore decision does not supply
   a Windows publisher certificate. macOS would likewise need its own native
   signing/notarization. No Wails runtime or desktop distribution is approved
   by this release-signing decision.

## Consequences

GitHub repository/workflow access is part of the release trust boundary.
Use minimal job permissions, pin release signing tooling, keep OIDC out of
ordinary build jobs and test real verification, including changed files and
wrong signer identity. Signing establishes origin/integrity, not financial
correctness or deployment acceptance. Hosted OIDC and native ARM evidence must
be recorded before #333's candidate acceptance is complete; local key-pair
tests cannot substitute for them.

## References

- [Sigstore CI signing and verification](https://docs.sigstore.dev/quickstart/quickstart-ci/)
- [Sigstore blob bundles](https://docs.sigstore.dev/cosign/signing/signing_with_blobs/)
- [GitHub runner architectures](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)
- [GitHub immutable releases](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases)
- [Wails platform signing](https://wails.io/guides/build/signing/)
- [Windows publisher signing and reputation](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation)
