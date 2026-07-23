# syntax=docker/dockerfile:1

ARG VERSION=0.2.0-rc.9
ARG COMMIT=source
ARG BUILD_DATE=unknown

FROM node:22-alpine AS web-builder
WORKDIR /src/web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS go-builder
WORKDIR /src

ARG VERSION
ARG COMMIT
ARG BUILD_DATE

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-builder /src/internal/webui/dist ./internal/webui/dist

RUN CGO_ENABLED=0 GOOS=linux go build -tags=nodynamic \
    -trimpath \
    -ldflags="-s -w -X samrai/internal/version.Version=${VERSION} -X samrai/internal/version.Commit=${COMMIT} -X samrai/internal/version.Date=${BUILD_DATE}" \
    -o /out/samrai ./cmd/server \
    && mkdir -p /out/data

# Tesseract and the Portuguese/English language data are included so OCR works
# without an additional sidecar. The samrai binary itself remains static.
FROM debian:bookworm-slim
WORKDIR /app

ARG VERSION
ARG COMMIT
ARG BUILD_DATE

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        tesseract-ocr \
        tesseract-ocr-eng \
        tesseract-ocr-por \
        7zip \
        unar \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 65532 samrai \
    && useradd --system --uid 65532 --gid 65532 --home-dir /nonexistent --shell /usr/sbin/nologin samrai

LABEL org.opencontainers.image.title="samrai" \
      org.opencontainers.image.description="Self-hosted reader for manga, comics, books, and PDFs" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"

COPY --from=go-builder /out/samrai /app/samrai
COPY --from=go-builder --chown=65532:65532 /out/data /data

ENV SAMRAI_ADDRESS=:8080 \
    SAMRAI_DATA_DIR=/data \
    SAMRAI_LOG_LEVEL=info \
    SAMRAI_SHUTDOWN_TIMEOUT=10s \
    SAMRAI_MAX_DB_CONNECTIONS=4 \
    SAMRAI_SESSION_DURATION=720h \
    SAMRAI_COOKIE_SECURE=false \
    SAMRAI_ARGON2_MEMORY_MIB=64 \
    SAMRAI_ARGON2_ITERATIONS=3 \
    SAMRAI_ARGON2_PARALLELISM=2 \
    SAMRAI_MAX_UPLOAD_MIB=2048 \
    SAMRAI_MAX_ARCHIVE_MIB=8192 \
    SAMRAI_MAX_PAGE_MIB=128 \
    SAMRAI_MAX_ARCHIVE_ENTRIES=10000 \
    SAMRAI_MAX_PAGES=5000 \
    SAMRAI_IMAGE_WORKERS=2 \
    SAMRAI_IMAGE_CACHE_MIB=5120 \
    SAMRAI_IMAGE_QUALITY=82 \
    SAMRAI_IMAGE_MAX_WIDTH=3840 \
    SAMRAI_CACHE_CLEANUP_INTERVAL=1h \
    SAMRAI_TESSERACT_PATH=/usr/bin/tesseract \
    SAMRAI_OCR_WORKERS=1 \
    SAMRAI_OCR_TIMEOUT=90s \
    SAMRAI_7ZIP_PATH=/usr/bin/7zz \
    SAMRAI_LSAR_PATH=/usr/bin/lsar \
    SAMRAI_UNAR_PATH=/usr/bin/unar

EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD ["/app/samrai", "healthcheck"]
USER 65532:65532
ENTRYPOINT ["/app/samrai"]
