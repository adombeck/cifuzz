current_os :=
label_os :=
bin_ext :=

# Force PowerShell on Windows. sh.exe on Windows GH Actions runners is using a different PATH with incompatible tools.
ifeq ($(OS),Windows_NT)
	SHELL := pwsh.exe
	.SHELLFLAGS := -NoProfile -Command
endif

ifeq ($(OS),Windows_NT)
	current_os = windows
	label_os = windows
	bin_ext = .exe
	RM = rm -r -force
else
	UNAME_S := $(shell uname -s)
	ifeq ($(UNAME_S),Linux)
		current_os = linux
		label_os = linux
	endif
	ifeq ($(UNAME_S),Darwin)
		current_os = darwin
		label_os = macOS
		UNAME_P := $(shell uname -p)
	endif
	RM = rm -r -f
endif

bin_dir = build/bin
binary_base_path = $(bin_dir)/cifuzz
installer_base_path = $(bin_dir)/cifuzz_installer

project := "code-intelligence.com/cifuzz"

# default version can be overriden by
# make version=1.0.0-dev [target]
version = dev

# Set IMAGE_ID to ghcr.io/codeintelligencetesting/cifuzz if it's not set
image_id ?= ghcr.io/codeintelligencetesting/cifuzz

# Set IMAGE_TAG to IMAGE_ID:version
image_tag ?= $(image_id):$(version)

# Export environment variables from the .env file if it exists
ifneq ("$(wildcard .env)","")
	include .env
	export
endif


default:
	@echo cifuzz

.PHONY: clean
clean: clean/examples/cmake clean/third-party/minijail
	$(RM) build/

.PHONY: clean/examples/cmake
clean/examples/cmake:
	-$(RM) examples/cmake/.cifuzz-*
	-$(RM) examples/cmake/build/
	-$(RM) examples/cmake/crash-*
	-$(RM) examples/cmake/*_inputs

.PHONY: clean/third-party/minijail
clean/third-party/minijail:
	PWD=${PWD}/third-party/minijail make -C third-party/minijail clean

.PHONY: deps
deps:
	go mod download

.PHONY: deps/integration-tests
deps/integration-tests:
	./tools/test-bucket-generator/check-buckets.sh
	go install github.com/bazelbuild/buildtools/buildozer@latest

.PHONY: deps/dev
deps/dev: deps
	go install github.com/incu6us/goimports-reviser/v2@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.54.2
	yarn install --silent


.PHONY: deps/test
# TODO: use a version of gotestfmt ^2.4.2 when it's released
deps/test:
	go install github.com/gotesttools/gotestfmt/v2/cmd/gotestfmt@b870aff77ad39547466e58f79725ca4d1bd92881

.PHONY: install
install:
	go run tools/builder/builder.go --version $(version)
	go run -tags installer cmd/installer/installer.go --verbose
	$(RM) cmd/installer/build

.PHONY: install/coverage
install/coverage:
	go run tools/builder/builder.go --version $(version) --coverage
	go run -tags installer cmd/installer/installer.go --verbose
	$(RM) cmd/installer/build

.PHONY: installer
installer:
	go run tools/builder/builder.go --version $(version)
	go build -tags installer -o $(installer_base_path)_$(label_os)_amd64$(bin_ext) cmd/installer/installer.go
	$(RM) cmd/installer/build

.PHONY: installer/darwin-arm64
installer/darwin-arm64:
	go run tools/builder/builder.go --version $(version) --goos darwin --goarch arm64
	GOOS=darwin GOARCH=arm64 go build -tags installer -o $(installer_base_path)_macOS_arm64 cmd/installer/installer.go
	$(RM) cmd/installer/build

###
# Building binaries for all platforms
# With a switch for coverage instrumentation
###

# Decide between build and build-coverage
build_target := $(word 1, $(subst /, ,$(MAKECMDGOALS)))
build_flags :=
build-coverage_flags := --cover
# Used with build_target to decide flags to pass to the go build command
define get_go_build_args
	$($(word 1, $(subst /, ,$(1)))_flags)
endef

.PHONY: build
build: build/$(current_os)

build/all build-coverage/all: $(build_target)/linux $(build_target)/windows $(build_target)/darwin

.PHONY: build/linux build-coverage/linux
build/linux build-coverage/linux: deps
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(call get_go_build_args, $@) -o $(binary_base_path)_linux cmd/cifuzz/main.go

.PHONY: build/windows build-coverage/windows
build/windows build-coverage/windows: deps
	env GOOS=windows GOARCH=amd64 go build $(call get_go_build_args, $@) -o $(binary_base_path)_windows.exe cmd/cifuzz/main.go

.PHONY: build/darwin build-coverage/darwin
build/darwin build-coverage/darwin: deps
ifeq ($(UNAME_P), arm)
	env GOOS=darwin GOARCH=arm64 go build $(call get_go_build_args, $@) -o $(binary_base_path)_macOS cmd/cifuzz/main.go
else
	env GOOS=darwin GOARCH=amd64 go build $(call get_go_build_args, $@) -o $(binary_base_path)_macOS cmd/cifuzz/main.go
endif

.PHONY: lint
lint: deps/dev
	golangci-lint run

.PHONY: fmt
fmt: deps/dev
	find . -type f -name "*.go" -not -path "./.git/*" -print0 | xargs -0 -n1 goimports-reviser -project-name $(project) -file-path
	npx prettier --ignore-path .gitignore --ignore-path .prettierignore --plugin=@prettier/plugin-xml --print-width=120 --xml-whitespace-sensitivity=preserve --log-level=warn --write .

.PHONY: fmt/check
fmt/check: deps/dev
	@DIFF=$$(find . -type f -name "*.go" -not -path "./.git/*" -print0 | xargs -0 -n1 goimports-reviser -project-name $(project) -list-diff -file-path); \
	# Exit if the find command failed \
	if [ "$$?" -ne 0 ]; then \
	  exit "$$1"; \
	fi; \
	# Exit after printing unformatted files (if any) \
	if [ -n "$${DIFF}" ]; then \
		echo -e >&2 "Unformatted files:\n$${DIFF}"; \
		exit 1; \
	fi;
	npx prettier --ignore-path .gitignore --ignore-path .prettierignore --plugin=@prettier/plugin-xml --print-width=120 --xml-whitespace-sensitivity=preserve --log-level=warn --check .

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: tidy/check
tidy/check:
	# Replace with `go mod tidy -check` once that's available, see
	# https://github.com/golang/go/issues/27005
	if [ -n "$$(git status --porcelain go.mod go.sum)" ]; then       \
		echo >&2 "Error: The working tree has uncommitted changes."; \
		exit 1;                                                      \
	fi
	go mod tidy
	if [ -n "$$(git status --porcelain go.mod go.sum)" ]; then \
		echo >&2 "Error: Files were modified by go mod tidy";  \
		git checkout go.mod go.sum;                            \
		exit 1;                                                \
	fi

.PHONY: test
test: deps build/$(current_os)
	go test -v ./...

.PHONY: test/unit
test/unit: deps deps/test
	go test -json -v ./... -short 2>&1 | tee gotest.log | gotestfmt -hide all

INTEGRATION_TEST_PLATFORM := $(word 3, $(subst /, ,$(MAKECMDGOALS)))
INTEGRATION_TEST_BUCKET := $(word 4, $(subst /, ,$(MAKECMDGOALS)))
.PHONY: test/integration/%
test/integration/%: deps deps/test deps/integration-tests
	@echo $(INTEGRATION_TEST_PLATFORM)
	go test -json -v -timeout=20m -run 'TestIntegration.*' $(shell cat ./tools/test-bucket-generator/$(INTEGRATION_TEST_PLATFORM)/bucket-$(INTEGRATION_TEST_BUCKET).txt) 2>&1 | tee gotest.log | gotestfmt -hide all

.PHONY: test/integration
test/integration: deps deps/test deps/integration-tests
	go test -json -v -timeout=20m ./... -run 'TestIntegration.*' 2>&1 | tee gotest.log | gotestfmt -hide all

.PHONY: test/e2e
test/e2e: deps deps/test build/linux build/windows build-container-image start-container-registry
test/e2e: export E2E_TESTS_MATRIX = 1
test/e2e:
	go test -json -v ./e2e/... | tee gotest.log | gotestfmt

.PHONY: test/race
test/race: deps build/$(current_os)
	go test -v ./... -race

.PHONY: test/maven
test/maven:
	cd tools/list-fuzz-tests && mvn test

.PHONY: coverage
coverage: export E2E_TESTS_MATRIX = V
coverage: deps install/coverage
coverage:
	-$(RM) coverage
	mkdir -p coverage/e2e coverage/unit coverage/integration
	-go test ./... -cover -args -test.gocoverdir="${PWD}/coverage/unit"
	go tool covdata func -i=./coverage/unit,./coverage/e2e,./coverage/integration

.PHONY: coverage/merge
coverage/merge:
	go tool covdata func -i=./coverage/unit,./coverage/e2e,./coverage/integration
	go tool covdata textfmt -i=./coverage/unit,./coverage/e2e,./coverage/integration -o coverage/profile
	go tool cover -html coverage/profile -o coverage/report.html

.PHONY: coverage/e2e
coverage/e2e: export E2E_TESTS_MATRIX = V
coverage/e2e: deps build-coverage/$(current_os)
	-$(RM) coverage/e2e
	mkdir -p coverage/e2e
	-go test ./e2e/... -v
	go tool covdata func -i=./coverage/e2e

.PHONY: coverage/integration
coverage/integration: deps
	-$(RM) coverage/integration
	mkdir -p coverage/integration
	-go test ./... -run 'TestIntegration.*'
	go tool covdata func -i=./coverage/integration

.PHONY: coverage/unit
coverage/unit: deps
	-$(RM) coverage/unit
	mkdir -p coverage/unit
	-go test ./... -short -cover -args -test.gocoverdir="${PWD}/coverage/unit"
	go tool covdata func -i=./coverage/unit

.PHONY: site/setup
site/setup:
	-$(RM) site
	git clone git@github.com:CodeIntelligenceTesting/cifuzz.wiki.git site

.PHONY: site/generate
site/generate: deps
	$(RM) ./site/*.md
	go run ./cmd/gen-docs/main.go --dir ./site/
	cp -R ./docs/*.md ./site

.PHONY: site/update
site/update:
	git -C site add -A
	git -C site commit -m "update docs" || true
	git -C site push

.PHONY: build-container-image
build-container-image: export DOCKER_BUILDKIT = 1
build-container-image: build/linux
ifneq ($(current_os),windows)
	docker build --platform linux/amd64 -f docker/cifuzz-base/Dockerfile -t $(image_tag) .
endif

push-container-image: build-container-image
	# Exit if GITHUB_TOKEN or GITHUB_USER are not set
	if [ -z "${GITHUB_TOKEN}" ] || [ -z "${GITHUB_USER}" ]; then \
		echo "GITHUB_TOKEN or GITHUB_USER not set"; \
		exit 1; \
	fi
	echo "${GITHUB_TOKEN}" | docker login ghcr.io -u "${GITHUB_USER}" --password-stdin
	docker push "$(image_tag)"

.PHONY: build-llvm-test-container-image
build-llvm-test-container-image: export DOCKER_BUILDKIT = 1
build-llvm-test-container-image: build/linux
ifneq ($(current_os),windows)
	docker build --platform linux/amd64 -f docker/llvm-test/Dockerfile -t cifuzz-llvm-test:latest .
endif

.PHONY: installer-via-docker
installer-via-docker: export DOCKER_BUILDKIT = 1
installer-via-docker:
	@echo "Building a cifuzz Linux installer"
	mkdir -p build/bin
	docker build --platform linux/amd64 -f docker/cifuzz-builder/Dockerfile . --target bin --output build/bin

.PHONY: start-container-registry
start-container-registry:
ifneq ($(current_os),windows)
	docker stop cifuzz-e2e-test-registry || true
	docker run --rm -d -p 5000:5000 --name cifuzz-e2e-test-registry registry:2
endif
