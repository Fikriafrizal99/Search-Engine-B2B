.PHONY: test collector

test:
	go test ./...

collector:
	go build -o bin/search-engine-b2b ./cmd/collector

# Example:
# ./bin/search-engine-b2b -engine /path/to/google_maps_scraper \
#   -location "Sukamulya, Cugenang, Kabupaten Cianjur, Jawa Barat, Indonesia"
