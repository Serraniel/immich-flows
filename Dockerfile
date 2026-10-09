# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/immich-flows ./cmd/immich-flows

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/immich-flows /immich-flows
VOLUME /data
EXPOSE 2283
HEALTHCHECK --interval=30s --timeout=5s CMD ["/immich-flows", "healthcheck"]
ENTRYPOINT ["/immich-flows"]
