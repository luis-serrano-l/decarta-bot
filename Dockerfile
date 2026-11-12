# Build stage. Base images are pinned by digest; Dependabot proposes updates.
FROM golang:1.27.4-alpine@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/demenu_bot . \
    && mkdir /out/data

# Runtime stage: no shell, no package manager, CA certificates included.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329

COPY --from=build /out/demenu_bot /usr/local/bin/demenu_bot
# Owned by the runtime user so that a named volume mounted here is writable.
COPY --from=build --chown=65532:65532 /out/data /data

ENV STORAGE_PATH=/data \
    PORT=8080

USER 65532:65532
VOLUME ["/data"]
EXPOSE 8080

# The image has no curl or wget, so the binary probes its own /health.
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/demenu_bot", "-healthcheck"]

ENTRYPOINT ["/usr/local/bin/demenu_bot"]
