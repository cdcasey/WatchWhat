# syntax=docker/dockerfile:1

# The build stage runs natively on the build host and cross-compiles Go for
# TARGETARCH, so multi-arch builds need no emulation. Tailwind therefore runs on
# (and is selected by) BUILDARCH.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG BUILDARCH
ARG TARGETOS
ARG TARGETARCH
ARG TAILWIND_VERSION=v4.3.5
ARG TAILWIND_SHA256_X64=bb050edd13f15006d872a9bb1772b908618a8df6806d30bf095cba26a09e640b
ARG TAILWIND_SHA256_ARM64=564dad3028c029b0c69042c66a7a79873ed7289b73fcb7b37973fbf79cb9a32f
WORKDIR /src

# The musl Tailwind build links libstdc++/libgcc dynamically.
RUN apk add --no-cache curl libstdc++ libgcc
RUN set -eu; \
    case "$BUILDARCH" in \
      amd64) arch=x64;   sum="$TAILWIND_SHA256_X64" ;; \
      arm64) arch=arm64; sum="$TAILWIND_SHA256_ARM64" ;; \
      *) echo "unsupported build arch: $BUILDARCH" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o /usr/local/bin/tailwindcss \
      "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-linux-${arch}-musl"; \
    echo "${sum}  /usr/local/bin/tailwindcss" | sha256sum -c -; \
    chmod +x /usr/local/bin/tailwindcss

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go tool templ generate
RUN tailwindcss -i web/tailwind/input.css -o web/static/css/app.css --minify
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/watchwhat ./cmd/server
RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/watchwhat /watchwhat
COPY --from=build --chown=65532:65532 /out/data /data
ENV WATCHWHAT_ADDR=:8080 \
    WATCHWHAT_DB_PATH=/data/watchwhat.db
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/watchwhat", "healthcheck"]
ENTRYPOINT ["/watchwhat"]
