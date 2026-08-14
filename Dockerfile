# ---- Build stage ----
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Cache module downloads in their own layer (invalidated only when go.mod/sum change)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a static, stripped binary
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-w -s" -o /out/aegis ./cmd/aegis

# ---- Runtime stage ----
# Minimal, non-root static image: no shell, no package manager, ~1.8 MB base.
FROM gcr.io/distroless/static-debian12:nonroot

# Metadata
LABEL org.opencontainers.image.title="aegis" \
      org.opencontainers.image.description="Protected infrastructure cleaning utility" \
      org.opencontainers.image.source="https://github.com/Orinameh/aegis"

# Non-root runtime user (uid/gid 65532) so the container can't write outside
# its mounted paths.
USER nonroot:nonroot

COPY --from=builder --chown=nonroot:nonroot /out/aegis /usr/local/bin/aegis

WORKDIR /config
ENV HOME=/config

ENTRYPOINT ["/usr/local/bin/aegis"]
CMD ["--config", "/config/config.yaml"]