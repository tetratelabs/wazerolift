
# Make functions strip spaces and use commas to separate parameters. The below variables escape these characters.
comma := ,
space :=
space +=

gofumpt := mvdan.cc/gofumpt@v0.4.0
gosimports := github.com/rinchsan/gosimports/cmd/gosimports@v0.3.5
golangci_lint := github.com/golangci/golangci-lint/cmd/golangci-lint@v1.51.1
asmfmt := github.com/klauspost/asmfmt/cmd/asmfmt@v1.3.2

.PHONY: test
test:
	@go test ./...

golangci_lint_path := $(shell go env GOPATH)/bin/golangci-lint

$(golangci_lint_path):
	@go install $(golangci_lint)

golangci_lint_goarch ?= $(shell go env GOARCH)

.PHONY: lint
lint: $(golangci_lint_path)
	@GOARCH=$(golangci_lint_goarch) CGO_ENABLED=0 $(golangci_lint_path) run --timeout 5m

.PHONY: format
format:
	@go run $(gofumpt) -l -w .
	@go run $(gosimports) -local github.com/tetratelabs/ -w $(shell find . -name '*.go' -type f)
	@go run $(asmfmt) -w $(shell find . -name '*.s' -type f)

.PHONY: check  # Pre-flight check for pull requests
check:
	@$(MAKE) lint golangci_lint_goarch=arm64
	@$(MAKE) lint golangci_lint_goarch=amd64
	@$(MAKE) format
	@go mod tidy
	@if [ ! -z "`git status -s`" ]; then \
		echo "The following differences will fail CI until committed:"; \
		git diff --exit-code; \
	fi


cranelift_compiler_dir := internal/compiler
cranelift_binary_name := cranelift_backend.wasm
cranelift_target_binary_path := $(cranelift_compiler_dir)/target/wasm32-wasi/release/$(cranelift_binary_name)
cranelift_checked_in_binary_path := $(cranelift_compiler_dir)/$(cranelift_binary_name)

.PHONY: build.cranelift
build.cranelift:
	@cd $(cranelift_compiler_dir) && cargo wasi build --release
	@cp $(cranelift_target_binary_path) $(cranelift_checked_in_binary_path)

test.cranelift: build.cranelift
	@cd $(cranelift_compiler_dir) && cargo wasi test
