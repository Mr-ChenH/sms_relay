FROM node:20-bookworm-slim AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM debian:bookworm-slim AS lpac-build
ARG LPAC_VERSION=v2.2.1
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential ca-certificates cmake git libcurl4-openssl-dev \
    && rm -rf /var/lib/apt/lists/*
RUN git clone --depth 1 --branch "${LPAC_VERSION}" https://github.com/estkme-group/lpac.git /src/lpac \
    && cmake -S /src/lpac -B /src/lpac/build \
        -DCMAKE_BUILD_TYPE=MinSizeRel \
        -DLPAC_WITH_APDU_PCSC=OFF \
        -DLPAC_WITH_APDU_AT=OFF \
        -DLPAC_WITH_HTTP_CURL=ON \
    && cmake --build /src/lpac/build --parallel --target lpac \
    && strip /src/lpac/build/output/lpac

FROM golang:1.25-bookworm AS api-build
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /src/api
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api/ ./
RUN go test ./... \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/smshub ./cmd/smshub

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates libcurl4 \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 smshub \
    && useradd --uid 10001 --gid smshub --create-home --home-dir /app smshub \
    && mkdir -p /app/web /data \
    && chown -R smshub:smshub /app /data
COPY --from=lpac-build /src/lpac/build/output/lpac /usr/local/bin/lpac
COPY --from=api-build /out/smshub /usr/local/bin/smshub
COPY api/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
COPY --from=web-build --chown=smshub:smshub /src/web/dist/ /app/web/
RUN chmod 0755 /usr/local/bin/docker-entrypoint.sh

WORKDIR /app
ENV SMS_HUB_API_ADDR=:8080 \
    SMS_HUB_MQTT_ADDR=:1883 \
    SMS_HUB_DB_PATH=/data/smshub.db \
    SMS_HUB_FIRMWARE_DIR=/data/firmware \
    SMS_HUB_WEB_DIR=/app/web \
    LPAC_PATH=/usr/local/bin/lpac
VOLUME ["/data"]
EXPOSE 8080 1883
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/usr/local/bin/smshub"]
