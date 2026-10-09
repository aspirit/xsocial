# syntax=docker/dockerfile:1.7
# Multi-stage build for the worker service.
# Versions come from versions.env (passed as --build-arg by Taskfile / CI), so
# local builds, CI and production images use identical toolchains.
ARG GO_VERSION=1.26.0
ARG ALPINE_VERSION=3.23.3

# ---- build ---------------------------------------------------------------------
FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /src

# Dependencies first: this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Only the trees the binary needs, to keep the build cache stable.
COPY internal ./internal
COPY pkg ./pkg
COPY services/worker ./services/worker

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags="-w -s \
        -X 'github.com/aspirit/xsocial/internal/version.Version=${VERSION}' \
        -X 'github.com/aspirit/xsocial/internal/version.Commit=${COMMIT}' \
        -X 'github.com/aspirit/xsocial/internal/version.BuildTime=${BUILD_TIME}'" \
      -o /out/worker ./services/worker

# ---- runtime -------------------------------------------------------------------
FROM alpine:${ALPINE_VERSION}
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
WORKDIR /app
COPY --from=builder /out/worker ./worker
# Non-secret defaults only; secrets are injected as environment variables.
COPY configs ./configs
USER app
ENTRYPOINT ["./worker"]
