FROM golang:1.25-bookworm AS lpac-build

ARG LPAC_VERSION=v2.3.0
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential ca-certificates cmake git libcurl4-openssl-dev libpcsclite-dev pkg-config \
    && rm -rf /var/lib/apt/lists/*
RUN git clone --depth 1 --branch "${LPAC_VERSION}" https://github.com/estkme-group/lpac.git /src/lpac \
    && cmake -S /src/lpac -B /src/lpac/build -DLPAC_WITH_APDU_PCSC=OFF -DLPAC_WITH_HTTP_CURL=ON \
    && cmake --build /src/lpac/build --parallel \
    && cmake --install /src/lpac/build

FROM golang:1.25-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates libcurl4 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=lpac-build /usr/local/ /usr/local/
WORKDIR /app/server/api
ENV LPAC_PATH=/usr/local/bin/lpac
CMD ["go", "run", "./cmd/smshub"]
