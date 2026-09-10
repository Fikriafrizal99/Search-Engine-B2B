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

# Bukupay runs as a non-root UID from docker-compose. The upstream scraper
# invokes Playwright's browser installer at runtime, which creates a lock under
# PLAYWRIGHT_BROWSERS_PATH (/opt/browsers). Keep the bundled browser directory
# writable so a collect does not fail with EACCES on /opt/browsers/__dirlock.
RUN mkdir -p /app/data /opt/browsers \
    && chmod -R a+rwX /app/data /opt/browsers

EXPOSE 8082

ENTRYPOINT ["/app/bin/bukupay-dashboard"]
CMD ["-addr", ":8082", "-db", "/app/data/bukupay.db", "-geo-cache", "/app/data/geo-cache", "-config-dir", "/app/config", "-collector", "/app/bin/search-engine-b2b", "-engine", "/usr/bin/google-maps-scraper"]
