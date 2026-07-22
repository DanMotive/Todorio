.PHONY: build run web dist clean

VERSION ?= 1.0.0-dev

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/todorio ./cmd/todorio

run:
	go run ./cmd/todorio serve --dev

web:
	cd web && npm install && npm run build

dist: web build
	@echo "OK: bin/todorio + web/dist"

clean:
	rm -rf bin web/dist
