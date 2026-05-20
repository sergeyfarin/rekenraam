# Deployment

Deployment assets live here.

## Single Binary

The preferred release artifact is a Go binary that embeds the compiled SvelteKit static frontend.

Expected future output:

```text
dist/rekenraam
```

## Docker

Docker should package the same production binary rather than rebuilding the architecture differently.

Suggested future layout:

```text
deploy/docker/Dockerfile
deploy/docker/compose.yaml
```

Keep runtime SQLite data mounted as a volume in Docker.
