FROM golang:1.26.6-trixie AS bukupay-builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o /out/search-engine-b2b ./cmd/collector \
    && CGO_ENABLED=0 go build -ldflags="-w -s" -o /out/bukupay-dashboard ./cmd/dashboard

# Reuse the upstream scraper image so Chromium + Playwright dependencies
# do not need to be maintained separately in this repository.
FROM gosom/google-maps-scraper:latest

WORKDIR /app

COPY --from=bukupay-builder /out/search-engine-b2b /app/bin/search-engine-b2b
COPY --from=bukupay-builder /out/bukupay-dashboard /app/bin/bukupay-dashboard
COPY config /app/config

RUN mkdir -p /app/data && chmod -R a+rwX /app/data

EXPOSE 8082

ENTRYPOINT ["/app/bin/bukupay-dashboard"]
CMD ["-addr", ":8082", "-db", "/app/data/bukupay.db", "-geo-cache", "/app/data/geo-cache", "-config-dir", "/app/config", "-collector", "/app/bin/search-engine-b2b", "-engine", "/usr/bin/google-maps-scraper"]
