# Agent instructions

- Go service, single binary, Docker only. Entry: `cmd/immich-flows`. Packages in `internal/`.
- Target the latest Immich major only. Verify API details against Immich source or OpenAPI spec, not memory.
- Immich client is generated (`internal/immich`). Add operation IDs to `oapi-codegen.yaml`, run `go generate ./...`; never edit `*.gen.go`.
- Before finishing: `gofmt`, `go vet ./...`, `go test -race ./...`, golangci-lint (`.golangci.yml`).
- Keep comments and docs minimal and precise. No filler prose.
- Commit messages and PR titles: Conventional Commits.
- Do not add dependencies without need. Pure Go, `CGO_ENABLED=0`.
- Proxy changes must not buffer request or response bodies.
- Update `docs/` when behaviour or configuration changes.
