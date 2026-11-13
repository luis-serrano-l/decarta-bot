# Build stage. Base images are pinned by digest; Dependabot proposes updates.
# It runs on the build machine's own architecture and cross-compiles, so a
# multi-architecture build needs no emulation.
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
# VERSION is what "decarta-bot --version" prints; the release workflow sets it.
ARG VERSION=dev
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /out/decarta-bot . \
    && mkdir /out/data

# Runtime stage: no shell, no package manager, CA certificates included.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931

COPY --from=build /out/decarta-bot /usr/local/bin/decarta-bot
# Owned by the runtime user so that a named volume mounted here is writable.
COPY --from=build --chown=65532:65532 /out/data /data

ENV STORAGE_PATH=/data \
    PORT=8080

USER 65532:65532
VOLUME ["/data"]
EXPOSE 8080

# The image has no curl or wget, so the binary probes its own /health.
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/decarta-bot", "-healthcheck"]

ENTRYPOINT ["/usr/local/bin/decarta-bot"]
