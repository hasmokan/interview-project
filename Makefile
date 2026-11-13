export GOPROXY ?= https://mirrors.aliyun.com/goproxy/
export GOSUMDB ?= sum.golang.google.cn

.PHONY: build test web acceptance docker-acceptance
build: web
	go build -o bin/server ./cmd/server
	go build -o bin/fileshare ./cmd/fileshare
web:
	cd web && npm ci --no-audit --no-fund && npm run build && npm run typecheck
test:
	go test -race ./...
acceptance: build
	python3 scripts/acceptance.py --browser
docker-acceptance: build
	docker build -t interview-project:local .
	python3 scripts/acceptance.py --compose --browser
