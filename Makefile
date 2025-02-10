GOOS = $(shell go env GOOS)
GOARCH = $(shell go env GOARCH)
BUILD_DIR = dist/${GOOS}_${GOARCH}
GENERATED_CONF := pkg/config/conf.gen.go

ifeq ($(GOOS),windows)
OUTPUT_PATH = ${BUILD_DIR}/baton-bitbucket.exe
else
OUTPUT_PATH = ${BUILD_DIR}/baton-bitbucket
endif

.PHONY: build
build: $(GENERATED_CONF)
	go build -o ${OUTPUT_PATH} ./cmd/baton-bitbucket

$(GENERATED_CONF): pkg/config/config.go go.mod
	@echo "Generating $(GENERATED_CONF)..."
	go generate ./pkg/config

generate: $(GENERATED_CONF)

.PHONY: update-deps
update-deps:
	go get -d -u ./...
	go mod tidy -v
	go mod vendor

.PHONY: add-dep
add-dep:
	go mod tidy -v
	go mod vendor

.PHONY: lint
lint:
	golangci-lint run
