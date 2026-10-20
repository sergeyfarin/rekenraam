# Deployment, upgrades and release-readiness audit — 2026-10-18

**2026-10-20 residual-review update:** [#337's dated
follow-up](runtime-image-advisory-audit-2026-10-20.md) rescans the current image
and classifies all residual rows. Engineering review is complete; #337 remains
blocked on owner release disposition. Release stays on hold; additional #354
owns bundled native SQLite advisory coverage. The original evidence below is
unchanged.

**Complete:** bounded audit [#279](https://github.com/sergeyfarin/rekenraam/issues/279)
under [#260](https://github.com/sergeyfarin/rekenraam/issues/260). This report
records candidate deployment evidence, not permission to release or announce.

## Scope and inputs

The clean baseline was `1c63b7bc9b03484c354a0b26339d7e4afa545bc9`.
The earlier candidate was `00fdbd8f` (migration 14), built from `git archive`,
with its own frontend and backend. Current source contains migrations 1–18.
The final image includes this audit's `.dockerignore` and Debian-update fixes;
application source and migrations are unchanged from the baseline. Native
build/platform evidence is Linux/amd64 only, with CGO/bundled SQLite and libc.

Docker Engine 29.9.1 and Compose 5.6.0 ran the supplied production Compose
configuration, with only the image tag and loopback test port overridden.
Each drill uses its own named volume and project; it removes both afterwards.
No real owner data, provider credentials or provider requests are needed.
Python's synthetic HTTP client explicitly sends its own Secure-cookie jar to
loopback, where browsers permit these cookies. HTTPS/browser cookie behavior
is established separately by [#268](proxy-browser-privacy-audit-2026-10-17.md),
at that report's recorded SHA; it is not claimed as a fresh TLS run here.

The independent oracle is an ordinary USD transfer on 2026-01-02: checking
**-25.00**, savings **+25.00**, exactly two posted CSV rows and a zero sum.
The baseline export's entire set of fields must survive restart, upgrade and
restored rollback unchanged. Head adds `posting_version_id` (#322); that
additive field must be 1/2 for this freshly seeded book. Comparing raw files
across that additive header change would incorrectly reject a sound upgrade.
The wider frozen candidate fixture is checked by
`TestMigrateSeededBaselineMatchesFreshSchema`; it is not an earlier installed
release fixture. No installed release exists yet.

## Findings and disposition

| Finding | Severity | Disposition |
|---|---|---|
| Docker admitted local runtime data and environment files to the builder | P2, [#332](https://github.com/sergeyfarin/rekenraam/issues/332) | **Confirmed defect, fixed.** Real Docker COPY admitted the synthetic `backend/var/backups/private.sqlite` sentinel before the fix. The same consumer now excludes every private/generated sentinel and retains required sources; the production image builds with the filtered context |
| Inherited runtime packages could miss available Debian updates | P2 review, [#337](https://github.com/sergeyfarin/rekenraam/issues/337) | **Confirmed update gap, fixed; residual audit open.** Runtime construction now updates inherited packages. `liblzma5` advances from `5.8.1-1+deb13u1` to `5.8.1-1+deb13u2`; the two fixable advisory rows disappear. No remaining scanned Debian row has an available fix. Reachability/release disposition of the other inventory rows still needs #337 |
| Release-seed generator named a removed upgrade test and implied a released-version fixture | P3, [#338](https://github.com/sergeyfarin/rekenraam/issues/338) | **Confirmed documentation defect, fixed.** Active generator/header template and upgrade guidance name the actual candidate regression. Frozen fixture bytes remain unchanged |

## Acceptance evidence

Expected results derive from the upgrade guide, ADRs 0002/0005/0014/0015 and
the two explicitly entered amounts, rather than from code-generated totals.

| Scenario and command/test | Independent expected result | Actual result | Limit / disposition |
|---|---|---|---|
| `./scripts/build-single-binary.sh` | Frontend and API in one runnable native artifact; embedded static consumer passes | Frontend production build, 66 catalog checks and `TestHandlerServesEmbeddedBuildOutput` pass; `dist/rekenraam` built | **Verified**, Linux/amd64; compatible libc required |
| `docker compose … build --no-cache`, followed by fixed-source builds | Clean multi-stage build uses Debian trixie, CGO/FTS5 and embedded frontend | Original clean build and final patched image build pass; current manifest `sha256:d4f1e09e3d5588037e97442090a6cc6d8d6732f50e337199d1c2c481d3715426` | **Verified**; source fixes rebuilt and exercised |
| `bash scripts/test-docker-context.sh` | Private files unavailable to Docker COPY; required sources retained | Fails before #332, passes after it | **Verified** synthetic regression; does not read private files |
| `scripts/audit-deployment.py`: fresh current volume | Healthy production service, fresh setup state, embedded shell at `/` and extensionless app route | Pass | **Verified**; head onboarding through a browser is covered by preflight below |
| Earlier candidate fresh install/setup/seed | All setup commands succeed; exact -25.00/+25.00 rows; self-check passes | Pass on migration 14 | **Verified**, synthetic two-account book |
| Persistence and permissions | Non-root, read-only root, all capabilities dropped, no-new-privileges; data 0700, database/WAL/SHM 0600; restart retains every CSV field | All observed; restart CSV matches | **Verified** for named volume; arbitrary host bind-mount ownership is not tested |
| Running old-version backup, CLI verification | Manual backup reaches completed/verified before shutdown; integrity/FK checks pass; schema 14 | Completed, verified, 1,634,304 bytes; schema 14 | **Verified**; copied by the running app, not a raw live-file copy |
| Stopped old → current image | Migrations advance 14→18; prior ledger fields unchanged; self-check passes | Schema 18; all prior CSV fields preserved; self-check passed; integrity/FK checks pass | **Verified**, synthetic candidate upgrade |
| Current backup inspected by old restore | Newer schema refused before replacement | Current backup completed/verified, 1,662,976 bytes; old restore refuses it as newer | **Verified**; no down migration used |
| Restore pre-upgrade backup, restart old image | Restore succeeds; exact old ledger and self-check retained | Restore succeeds; all prior fields match, -25.00/+25.00; self-check passed | **Verified**, old-image rollback by backup |
| `E2E_SKIP_BUILD=1 ./scripts/test-release-preflight.sh` | Fresh-binary browser journeys for balanced split, reconciliation, QIF, buy/sell and mobile entry | **5 passed** | **Verified**; focused preflight, not the full browser suite |
| Config tests (`TestLoad*`) and `TestRuntime*` | Invalid env/key/proxy inputs and production provider-stub overrides refused; runtime failures release resources | Pass | **Verified** in named consumers; final settings of a real deployment remain owner evidence |
| Focused DB/migration/CLI tests | Seeded migration preservation, pinned checksums, restore/recovery/verification and two-process lock guards hold | Pass; initial sandbox socket refusals disappear on authorized socket-enabled rerun | **Verified**; no full backend/race gate repeated here |

The rerunnable drill and parameters are documented in
[developer workflow](../developer-workflow.md#deployment-audit). The earlier
image is built by extracting `git archive 00fdbd8f` to a temporary directory
and running `docker build -t rekenraam-audit279-old:00fdbd8f -f
<old>/deploy/docker/Dockerfile <old>`. The current image uses Compose project
`rekenraam-audit279`; the drill accepts both tags and highest migration
versions as explicit arguments. The final invocation uses loopback port 16891.
The retained [drill transcript](deployment-drill-2026-10-18.txt) records the
image digests, permission observations, backup inspections and final success.

## Supply chain and outbound configuration

- **Go:** `govulncheck ./...`, with the native SQLite flags, reports zero
  reachable vulnerabilities and zero vulnerabilities in imported packages.
  Verbose output names module-only `GO-2026-5932` in unimported
  `golang.org/x/crypto/openpgp`; no fixed version exists. **Accepted scan
  limit:** an unused package's module advisory is not an application exploit.
- **JavaScript:** `pnpm audit --json` reports zero advisories across 308
  dependencies. This is a dated registry result, not a future guarantee.
- **Image:** Trivy 0.69.3 scans an exported tar without Docker-socket access
  or host networking, as the tar owner, with a read-only root and all
  capabilities dropped. Before/after counts and advisory IDs are in
  [the scan summary](deployment-image-scan-2026-10-18.json). Debian rows fall
  from 283 to 281: **47 high, 91 medium, 142 low, 1 unknown** remain, with
  no advertised fix; the separate unused Go-module row remains. These are
  package/advisory rows, including repeated advisories across packages, not
  281 confirmed exploitable flaws. **Not verified:** reachability and release
  acceptance of the residual inventory; additional audit #337 owns it.
- **Secret history:** Gitleaks 8.30.1 scans all local refs (`git --log-opts=--all
  --redact`), 1,057 commits / about 30.92 MB, in this non-shallow checkout.
  Five detections were individually inspected: the fake Trading 212 stub key
  in `dae255c3`, the deterministic encryption-test key in `5aa1027c`, and
  three fixed setup-token literals in `8398e99a`'s setup/config tests. All
  are synthetic test constants. **Verified bounded result:** no actual secret
  identified by this scan; remote scanning/protection is unverified.
- **Outbound boundaries:** production refuses `TRADING212_BASE_URL` and
  `MARKET_DATA_BASE_URL` in both config and runtime consumers. The supplied
  Compose file passes no provider keys by default. Quote assignments and
  import connections are explicit owner actions; online-import auto-refresh
  defaults off. The pricing scheduler itself defaults on, but FX targets need
  configured base/source relationships. This is configuration/code evidence,
  **not a packet-capture proof of zero outbound traffic** or a global egress
  firewall. Provider-independent core operation remains the governing rule.

## Release-readiness gaps

| Gap | Issue / disposition |
|---|---|
| No release packaging/signing workflow or selected signing identity; only baseline migrations 0001/0002 checksum-pinned so far | [#333](https://github.com/sergeyfarin/rekenraam/issues/333), P2, **blocked on owner signing/target decisions**. Before v0.1.0, pin every released migration and include highest migration, checksums, signatures and libc/reproducibility notes. v0.1.0 also activates installed-release immutability and branches/PRs |
| No scoped, hashed, expiring/revocable PAT workflow | [#334](https://github.com/sergeyfarin/rekenraam/issues/334), P2, **open**; cookie/CSRF APIs are not this credential mechanism |
| No public seeded-demo recipe, README screenshots or completed migration walkthrough | [#335](https://github.com/sergeyfarin/rekenraam/issues/335), P3, **open**. Test seeds/reference books are not adoption assets |
| Real GitHub security settings, owner HTTPS/MFA/key custody/off-device restore and human household dry run | Additional audit [#336](https://github.com/sergeyfarin/rekenraam/issues/336), P2, **blocked on actual owner access/evidence and human use**. Explicitly **not verified** here or by #268 |
| Remaining image-advisory reachability and release disposition | Additional audit [#337](https://github.com/sergeyfarin/rekenraam/issues/337), P2, **open**; the available package patch is applied here |
| Native-language terminology sign-off | Existing [#102](https://github.com/sergeyfarin/rekenraam/issues/102), **not verified**; catalog parity is not native review |
| Combined architecture/API/quality/performance final gates | Existing [#280](https://github.com/sergeyfarin/rekenraam/issues/280), **pending**; this area deliberately runs focused suites only |

No release, hosted deployment, announcement or marketplace publication was
performed. The absence of an installed predecessor and Linux-only validation
are explicit current limits. The 14→18 candidate drill must not be generalized
to every rewritten pre-release schema or other libc/CPU/platform combination.

## Documentation and structural validation

Deployment guidance now describes the build-context boundary and Debian
updates. Upgrade guidance names the actual candidate migration regression and
links this actual image drill. README and developer workflow agree on the new
commands. `git diff --check` and shell syntax checks pass.

`./scripts/check-post-merge.sh HEAD` reports one diagnostic: the identical
Chromium-install step occurs in `ci.yml` and `release-preflight.yml`. Inspection
shows two separate validation workflows with different scopes/triggers, both
requiring a browser. **Accepted diagnostic**, not duplicate jobs or a reason
to delete validation. All other structural sections pass. #280 owns the final
combined gates; this report does not call this command an unqualified pass.

## Maintainability notes

- Production provider-override refusal is enforced in both
  `backend/internal/config/config.go` (`loadDevelopmentBaseURL`) and
  `backend/internal/appruntime/runtime.go` (`openWith`), with separate named
  tests. The runtime deliberately protects hand-built configs, but adding a
  new override requires keeping both guards and their messages aligned.
  Evidence: both paths were read and `TestLoad*`/`TestRuntime*` were exercised.
  Recorded for #284; no refactor is proposed or implemented here.
