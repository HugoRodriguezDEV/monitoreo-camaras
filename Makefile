.PHONY: build
build:
	cd web && npm ci && npm run build
	touch web/dist/.gitkeep
	mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -o dist/monitor ./cmd/monitor
