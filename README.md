# Garimpo

A local semantic search and RAG engine written in Go: it reads a corpus, cuts it into chunks,
turns them into vectors with a local model, indexes them and answers questions with citations
you can check. Everything runs on your machine.

## Requirements

- Docker with Compose. The Go toolchain runs inside the `dev` container; nothing else is installed on your machine.
- Git on the host.

## Getting started

```bash
docker compose up -d --build dev
docker compose exec dev go test ./...
docker compose exec dev go build -o bin/garimpo ./cmd/garimpo
docker compose exec dev ./bin/garimpo version
```

The container runs as a user with UID and GID 1000, so the files it creates in the project
belong to you. If your user has other IDs (`id -u`, `id -g`), build with them:

```bash
UID=$(id -u) GID=$(id -g) docker compose build dev
```

## Layout

| Path | Contents |
|---|---|
| `cmd/garimpo` | The binary: argument handling and wiring |
| `docker/dev.Dockerfile` | The development container (Go 1.27.4) |
| `compose.yaml` | Local services |

## License

[MIT](LICENSE)
