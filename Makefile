.PHONY: tidy test build collector dashboard run

tidy:
	go mod tidy

test:
	go test ./...

build: collector dashboard

collector:
	go build -o bin/search-engine-b2b ./cmd/collector

dashboard:
	go build -o bin/b2b-dashboard ./cmd/dashboard

run: build
	./bin/b2b-dashboard
