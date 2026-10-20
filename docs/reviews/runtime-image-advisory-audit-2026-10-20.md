# Runtime-image advisory audit — 2026-10-20

**#354 follow-up:** the [bundled native SQLite
review](native-sqlite-advisory-audit-2026-10-20.md) is complete, with application
search defect #355 fixed and two reproduced upstream correctness defects
retained in #356 pending a released driver. No affected financial app consumer
or native memory exploit was reproduced. #337's owner-disposition blocker
below remains; the original image evidence is unchanged.

**Engineering review complete; #337 remains BLOCKED on the owner's release
disposition.** The fresh inventory is classified below. No application exploit
was reproduced; no owner acceptance of the residual inventory is inferred.
**Release disposition: HOLD**, pending the owner's explicit acceptance or
remediation requirement. This is an audit result, not release authorization.

Tracked by [#337](https://github.com/sergeyfarin/rekenraam/issues/337) under
[#260](https://github.com/sergeyfarin/rekenraam/issues/260). This supersedes
the residual-review gap in the [2026-10-18 deployment
audit](deployment-audit-2026-10-18.md); its original update and drill evidence
remain historical. The available-package fix shipped in `3471a65d`.

## Candidate and evidence

Clean current-main baseline: `244db3b48b79ee09d229ef1a8e951fb2692f3dd0`.
No application, migration, Dockerfile or Compose change is made by this review.
The production Dockerfile was rebuilt with `--pull --no-cache`, including
fresh apt indexes and inherited-package upgrades, on Linux/amd64. Docker
29.9.1 and Compose 5.6.0 ran the drill. ADR 0005's Debian trixie runtime remains.

- Image manifest: `sha256:787f5bcce2a3fa85fdbff145a4eacae99772a9081bba4e5497a13c97034a89c2`.
- Image config: `sha256:1753eec1cdd5dc95dc7fcd72fbee401721bcc49007c30c64d052e6d34a283628`.
- Scanner: Trivy 0.69.3, image
  `aquasec/trivy@sha256:bcc376de8d77cfe086a917230e818dc9f8528e3c852f7b1aff648949b6258d1c`.
- Advisory database updated **2026-10-20 01:16:59 UTC**, downloaded at
  06:13:31 UTC; scan completed at 06:15:11 UTC. The actual scan had no network,
  Docker socket or host networking, a read-only root/database/image tar,
  UID 1000, dropped capabilities and no-new-privileges. Only its result
  directory and temporary analysis memory were writable.
- [Redacted structured evidence](runtime-image-evidence-2026-10-20.json)
  records hashes, database timestamp, versions, group counts, loaded libraries,
  imported symbols and effective security flags. The
  [complete package/version/advisory matrix](runtime-image-advisory-matrix-2026-10-20.csv)
  retains **all 282 rows**, including scanner status, absent advertised fixes,
  classification, evidence keys, rationale and authoritative advisory links.

The CSV preserves every affected package rather than deduplicating away
repeated advisories. There are **132 distinct Debian advisory IDs** across
281 rows: **47 high, 91 medium, 142 low, 1 unknown**. The Go-module result is
one additional unknown row, `GO-2026-5932`. There are **zero added or removed
class/package/version/advisory tuples** against #279's patched image scan.
`liblzma5=5.8.1-1+deb13u2` remains installed. No scanned row advertises a fixed
version. None of these counts measures reachable exploits.

## Observed consumer boundaries

These evidence keys are referenced by every matrix row:

- **E1 — delivered ELF and live maps.** `readelf -d`, `readelf --dyn-syms
  --wide` and `ldd` on the extracted image executable; `/proc/1/maps` from a
  disposable, network-isolated production instance. Needed/loaded objects are
  **libm, libc and the dynamic loader only**. Neither system libsqlite3 nor
  curl's libraries are loaded by the app. Positive controls find `open64`,
  `malloc`, `pthread_create` and `dlopen`; absent vulnerable interfaces are
  listed explicitly in the JSON. Thus the negative symbol checks actually
  examined an intact dynamic symbol table. No RPATH/RUNPATH exists; the app
  file is mode 0755, without setuid/setgid bits.
- **E2 — exact health-check consumer.** `ldd /usr/bin/curl` shows its separate
  library closure. Compose executes `curl --fail --silent
  http://127.0.0.1:16888/healthz`, with Docker's five-second timeout. It sets
  no authentication, redirects, compressed-transfer option, TLS, SSH, LDAP,
  cookie jar, upload, remote URL or multi-transfer pool. `/healthz` is the
  app's fixed response; the same command succeeds in the isolated container.
  These are boundaries of the supplied clean-image configuration, not of
  arbitrary operator curl commands, proxy/config overrides or a compromised
  process. Those changes require a new assessment.
- **E3 — source/build/import review.** `go version -m` confirms Go 1.27.2,
  CGO and `fts5,netgo,osusergo`. The supported `go list -deps ./cmd/rekenraam`
  closure contains no OpenPGP package or libc DNS/user-lookup package.
  `rg 'os/exec' backend -g '*.go'` finds only the two-process test helper;
  application code does not launch a shell, tar, Perl, mount or package tools.
  Backend text decoding, HTTP, TLS, regex and compression use Go libraries.
- **E4 — actual security posture.** UID/GID 10001, effective/permitted/bounding
  capabilities zero, `NoNewPrivs=1`, read-only root and private writable data.
  The exact Compose drill verifies the supplied configuration, persistence
  and data permissions. A separate isolated instance confirms actual loaded
  libraries and process flags. The app is the service process; no nscd or
  systemd-homed executable was found. These controls constrain privilege
  transitions; they do **not** protect the owner's writable ledger from a
  hypothetical successful same-user application compromise.
- **E5 — previous reachability result, freshly checked import boundary.**
  #279's Go 1.27.2/govulncheck result found zero affected imported packages or
  called symbols; its sole module-level advisory was OpenPGP. This audit
  reuses that recorded result as requested by #337, and verifies OpenPGP is
  still absent from the current production import closure. This is not a
  newly run govulncheck over every possible Go advisory.

An absent direct ELF import alone is not proof that libc cannot call an
interface internally. The assessment combines artifact imports, actual maps,
build tags, source/consumer inputs and advisory preconditions. It is a bounded
reachability review, not native-code fuzzing or a proof against every call path.

## Matrix interpretation and engineering disposition

| Classification | Rows | Evidence and disposition |
|---|---:|---|
| `utility` | 90 | apt, bash/dash, util-linux, coreutils, diffutils, ACL/attr, bzip2, ncurses, shadow, Perl, sysvinit and tar. Their affected commands/operations are not executed by app or health check. **Reviewed, no app path identified**; retain packages under ADR 0005, keep patching, reassess operator execution or changed privileges |
| `health-only` | 121 | curl and its GnuTLS/Kerberos/OpenLDAP/nghttp2/p11-kit/SASL/zlib dependencies. They are absent from app maps. The separate fixed cleartext loopback health check lacks the affected protocol/authentication/transfer inputs. **Reviewed for this exact command**, not a general libcurl safety claim |
| `unloaded-library` | 24 | gcc runtime/C++ findings, PAM and systemd libraries. Not loaded by either audited consumer; affected C++/OS-auth/daemon functions are not app operations. **Reviewed, no app path identified** |
| `unused-interface` | 26 | Thirteen glibc advisories repeated on libc6/libc-bin: monetary formatting, libc regex/glob, DNS diagnostics/resolver, word expansion, iconv, tdelete, attacker-controlled fopen mode. Required interfaces/inputs are absent in the artifact and supplied consumers. **Reviewed, affected operation not identified** |
| `privilege-precondition-absent` | 4 | Two glibc loader advisories repeated on libc6/libc-bin. Required secure-execution/ORIGIN path and privilege transition absent; no app RPATH/RUNPATH, no setuid/setgid app and no-new-privileges. **Reviewed for supplied runtime**, not proof the loader is patched |
| `service-absent` | 2 | glibc nscd advisory repeated on libc6/libc-bin. No nscd service, Go resolver selected. **Affected service absent** |
| `architecture-precondition-absent` | 2 | glibc Power8 strncasecmp advisory repeated on libc6/libc-bin. Image is amd64, no strncasecmp import. **Excluded for this architecture only** |
| `conditional-hardening-risk` | 8 | Four generic glibc stack/ASLR/address-disclosure claims repeated on libc6/libc-bin. libc/pthreads are actually used. No independent application corruption/leak path reproduced; **residual conditional risk retained for owner decision**, not marked unused or fixed |
| `different-engine` | 4 | Debian libsqlite3 rows do not identify the bundled runtime engine. **System-library path absent; bundled-engine coverage separately tracked in #354** |
| `package-not-imported` | 1 | OpenPGP advisory is module inventory; the affected package is absent. **Reviewed, no imported/called affected code**; do not remove required Argon2/crypto functionality to silence a module-level row |

The CSV splits **27 Debian source-package groups plus the Go module**, with
each affected binary package and exact installed version. In particular, 36
of the high rows repeat four util-linux advisories across nine binary
packages. The remaining highs concern curl, ACL, systemd, ncurses and Perl;
all retain their individual boundaries in the CSV. None was promoted to an
application defect solely because its scanner severity was high.

The libc-specific assessment checks advisory preconditions against
[Debian's glibc inventory](https://security-tracker.debian.org/tracker/source-package/glibc),
including [attacker-controlled fopen mode](https://security-tracker.debian.org/tracker/CVE-2026-18374)
and [secure-execution loader paths](https://security-tracker.debian.org/tracker/CVE-2026-95818).
The latter requires a privileged loader context absent from the supplied app
launch. Debian's no-DSA status is recorded upstream; it is not owner risk
acceptance. The [OpenPGP advisory](https://pkg.go.dev/vuln/GO-2026-5932)
identifies a package that remains absent from the app's dependency closure.

**Engineering recommendation:** retain the patched Debian runtime and
continue regular rebuild/rescan; do not replace ADR 0005, suppress advisory
IDs, or introduce speculative application changes to obtain a zero count.
No confirmed application defect or additional small fix arose in this bounded
review. The owner must record whether this exact residual inventory and the
stated limits are acceptable for a candidate or require more remediation.
Until then, #337's owner-disposition acceptance remains unmet and release
stays on hold. Other release gates (#333–#336, #102) remain independent.

## Validation and retained limits

| Scenario / independent expectation | Actual result / disposition |
|---|---|
| Fresh production multi-stage build with current package indexes | Pass; frontend/catalog build (66 catalog cases) and native Go artifact produced. `liblzma5` has the admitted fix |
| Fresh DB, all vulnerability severities, OS and Go binary inventory | Pass; 281 Debian + 1 Go rows, no available advertised fix, complete tuple-set equality to patched #279 inventory |
| Artifact linkage and live isolated process | Pass; libc/libm/loader only; positive/negative symbol controls and non-root/capability flags match JSON |
| Exact supplied Compose fresh install, 14→18 upgrade and verified-backup rollback | Pass; [redacted transcript](runtime-image-drill-2026-10-20.txt). Exactly two independent USD rows **-25.00 / +25.00**, every prior CSV field preserved across restart/upgrade/rollback, self-check passes, newer backup refused by old binary, integrity/FK checks pass |
| `TestNativeSQLiteEngineFeaturesAndFingerprint` | Pass; bundled SQLite **3.53.4**, source ID `2026-07-24 19:02:57 bf7c7f30031888f4e796e429ab3978879485813aaca6f641c7b33e4e09459bcc`, FTS3/FTS5/RTREE enabled, session extension not enabled |
| Published evidence completeness | Every raw scan tuple appears once in CSV with classification/rationale/evidence; source, severity, classification and unique-advisory counts reconcile to JSON; JSON parses; local Markdown links resolve; `git diff --check` passes |

The initial DB download exceeded a 1 GiB temporary-memory limit; a private
disk cache resolved it. An initial read-only analysis-cache error was resolved
with Trivy's memory cache. Neither failed attempt is counted as a scan.
The completed offline scan uses the dated downloaded DB; no severity/fix/status
filter or ignore file is applied. No Docker socket is mounted into the scanner.

This is Linux/amd64, the supplied clean-image/Compose configuration and one
synthetic two-account book. It does not certify alternate architectures,
arbitrary operator tools, host/kernel/reverse-proxy vulnerabilities, builder
dependency advisories, owner deployments or unreported flaws. No full browser
or backend gate is repeated for documentation/evidence-only changes; #280's
recorded shared gates are not relabelled as this candidate's fresh result.

## Additional audit

[#354](https://github.com/sergeyfarin/rekenraam/issues/354) covers **bundled
native SQLite advisory coverage and parser/restore boundaries**. The image
scanner's four system-libsqlite3 findings and govulncheck do not establish the
bundled C engine's status. The actual fingerprint is pinned by ADR 0014 and
the test above. [Debian's SQLite inventory](https://security-tracker.debian.org/tracker/source-package/sqlite3)
and [SQLite's input-security guidance](https://sqlite.org/security.html)
distinguish engine version/features and SQL/database-file consumers. #354
must map current native advisories to that exact source, verify extension and
real search/restore boundaries with synthetic controls, and publish its own
disposition. It is an independent missing audit, **not a confirmed exploit**
or a duplicate replacement for #337's still-missing owner decision.

## Reproduction

Use a private temporary directory for the image tar, scan, cache and output;
the cache download and scan need separate phases. Substitute the candidate tag
and absolute paths. The Trivy container must run as the directory's owner.

```sh
docker build --pull --no-cache -t rekenraam-audit337:2026-10-20 \
  -f deploy/docker/Dockerfile .
docker save -o /tmp/rekenraam-337/image.tar rekenraam-audit337:2026-10-20
docker run --rm --read-only --cap-drop ALL \
  --security-opt no-new-privileges --user 1000:1000 \
  -v /tmp/rekenraam-337/cache:/cache \
  --tmpfs /tmp:rw,noexec,nosuid,size=1g \
  aquasec/trivy:0.69.3 --cache-dir /cache image --download-db-only --no-progress
docker run --rm --read-only --cap-drop ALL --network none \
  --security-opt no-new-privileges --user 1000:1000 \
  --tmpfs /tmp:rw,noexec,nosuid,size=512m \
  -v /tmp/rekenraam-337/image.tar:/image.tar:ro \
  -v /tmp/rekenraam-337/cache:/cache:ro \
  -v /tmp/rekenraam-337/output:/output \
  aquasec/trivy:0.69.3 --cache-dir /cache image --input /image.tar \
  --skip-db-update --offline-scan --cache-backend memory --scanners vuln \
  --format json --output /output/scan.json --no-progress
python3 scripts/audit-deployment.py \
  --old-image rekenraam-audit279-old:00fdbd8f \
  --new-image rekenraam-audit337:2026-10-20 \
  --old-schema 14 --new-schema 18 --port 16892
./scripts/go-backend.sh test ./internal/db \
  -run '^TestNativeSQLiteEngineFeaturesAndFingerprint$' -count=1 -v
```

As in #279, the old candidate is built from `git archive 00fdbd8f`; the drill
removes only its own disposable project/volume. Raw scan/image/process files
stay local; published artifacts exclude financial records, environment values,
credentials, memory addresses, host paths and synthetic backup filenames.
