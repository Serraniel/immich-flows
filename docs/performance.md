# Performance

Overhead of immich-flows compared to direct Immich access. Measured with [e2e/perf_test.go](../e2e/perf_test.go) on the e2e stack (Immich v3.3.1, no ML, flows from `e2e/flows` enabled, user connected).

Host: AMD Ryzen 7 5700X (8 cores), 32 GB, Docker on Linux, client on the same host. Median of 3 rounds, direct and proxy alternating.

| Test | Direct | Through immich-flows |
| --- | --- | --- |
| `GET /api/server/ping`, 16 parallel, p50 / p95 | 3.6 / 7.5 ms, 3906 req/s | 4.0 / 8.6 ms, 3480 req/s |
| `GET /api/users/me`, 16 parallel, p50 / p95 | 9.8 / 15.7 ms, 1553 req/s | 11.1 / 16.5 ms, 1369 req/s |
| Upload 16 × 50 MiB, 8 parallel | 418 MiB/s | 383 MiB/s |

immich-flows memory: 13 MiB idle, 31 MiB peak during the uploads.

- Bodies are streamed, never buffered: memory does not grow with file size.
- Every upload is hashed with MD5, SHA-1, SHA-256 and SHA-512 while streaming; this is most of the upload overhead (~8 % at 400 MiB/s; a typical network is far slower).
- Flow runs are queued and run after the response; they add load on Immich (reads, downloads for `hash` with `source: stored`), not latency.

Run it:

```sh
cd e2e && docker compose up -d --build --wait
go test -tags e2e,perf -run TestPerf -v -timeout 30m ./e2e/
```
