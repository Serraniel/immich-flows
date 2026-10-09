# Contributing

## Setup

Requirements: Go (version in `go.mod`), Docker.

```sh
go test -race ./...
docker run --rm -v "$PWD:/app" -w /app golangci/golangci-lint:latest golangci-lint run
docker build -t immich-flows:dev .
```

E2E against a real Immich (Docker, ~2 min first run):

```sh
docker compose -f e2e/compose.yaml up -d --build --wait
go test -tags e2e -count=1 ./e2e/
docker compose -f e2e/compose.yaml down -v
```

## Workflow

1. Open or pick an issue. Larger changes: discuss first.
2. Branch from `main`, one topic per PR.
3. PR title follows [Conventional Commits](https://www.conventionalcommits.org) (`feat:`, `fix:`, `docs:`, `chore:`, `ci:`, `refactor:`, `test:`, `perf:`). It becomes the squash commit and drives releases and the changelog. Breaking: `feat!:`.
4. CI must pass.

## Style

- Code and docs: short and precise. Comment the *why*, not the *what*.
- New behaviour needs tests.
- No new dependency without a reason in the PR.

## AI

See [AI_POLICY.md](AI_POLICY.md).

## License

Contributions are licensed under [AGPL-3.0-or-later](LICENSE).
