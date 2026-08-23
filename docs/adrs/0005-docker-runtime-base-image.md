# ADR 0005: Docker Runtime Base Image

## Status

Accepted

## Date

2026-05-30

## Context

Rekenraam ships as one Go binary serving the statically built frontend. Docker Compose must preserve that app shape. Earlier docs mentioned a distroless Debian 12 runtime image, while the actual Dockerfile used Alpine. The project needs one explicit container base-image policy.

Debian 13, codenamed trixie, is the current stable Debian release. Docker publishes official `debian:trixie-slim` images.

## Decision

The production Docker runtime image uses the official Debian 13 slim image:

```text
debian:trixie-slim
```

Rules:

1. Use the official Debian image rather than distroless for the runtime image.
2. Keep the runtime image on Debian 13/trixie until an ADR intentionally moves it.
3. Keep the final image as one app process running the compiled Go binary.
4. Run the app as a non-root numeric user.
5. Keep SQLite data in a mounted persistent volume outside the image.
6. Build stages use official language images pinned to the project toolchain and Debian generation; the Go build stage uses `golang:1.27-trixie` while the runtime base remains `debian:trixie-slim`.

## Rationale

`debian:trixie-slim` is a better default for this self-hosted app than distroless:

- It is an official, familiar Debian base.
- It makes operator inspection and emergency debugging easier.
- It aligns with Debian 13 explicitly instead of relying on a distroless Debian variant.
- It is still small enough for a single-binary Go app.
- It avoids Alpine/musl differences in the production runtime.

Distroless remains a reasonable future optimization, but it is not the default production base image.

## Consequences

### Positive

- Docker runtime behavior matches the documented Debian 13 target.
- Self-hosted operators get a familiar base image for scanning and incident response.
- The final container remains simple: one non-root Go process plus a persistent data volume.

### Negative

- The runtime image is larger than distroless or scratch.
- A Debian slim base contains more packages than a minimal distroless image, so image scanning may show more OS package findings.

## References

- Debian 13 trixie release information: https://www.debian.org/releases/trixie/
- Debian official Docker image: https://hub.docker.com/_/debian
