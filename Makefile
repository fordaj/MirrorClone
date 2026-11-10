# Makefile — build and cross-compile mirror-clone.
#
# Targets:
#   make build      Build for the host platform into dist/host/
#   make cross      Cross-compile every release platform into dist/<platform>/
#   make platforms  Print the release platform directory names
#   make clean      Remove the build output
#
# The release version is stamped into the binary with
#   -ldflags "-X main.toolVersion=<version>"
# and is reported by `mirror-clone --version`. release.bash passes the
# version computed by versioner: VERSION=v1.2.3.4.5 make cross

SHELL := /bin/bash

DISTDIR := dist
BINARY  := mirror-clone
VERSION ?= dev
LDFLAGS := -s -w -X main.toolVersion=$(VERSION)

# Release platforms: <dist-dir>:<GOOS>:<GOARCH>
PLATFORMS := \
	macos-x86_64:darwin:amd64 \
	macos-arm64:darwin:arm64 \
	linux-x86_64:linux:amd64 \
	linux-arm64:linux:arm64 \
	windows-x86_64:windows:amd64 \
	windows-arm64:windows:arm64

PLATFORM_DIRS := $(foreach entry,$(PLATFORMS),$(DISTDIR)/$(word 1,$(subst :, ,$(entry))))

.PHONY: all build cross platforms clean

all: build

# Build for the host platform.
build:
	@mkdir -p $(DISTDIR)/host
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(DISTDIR)/host/$(BINARY) .

# Cross-compile every release platform into dist/<platform>/.
cross:
	@rm -rf $(PLATFORM_DIRS)
	@set -e; for entry in $(PLATFORMS); do \
		platform=$${entry%%:*}; \
		rest=$${entry#*:}; \
		goos=$${rest%%:*}; \
		goarch=$${rest##*:}; \
		bin=$(BINARY); \
		if [[ "$$goos" == "windows" ]]; then bin=$$bin.exe; fi; \
		echo "==> $$platform ($$goos/$$goarch)"; \
		mkdir -p "$(DISTDIR)/$$platform"; \
		CGO_ENABLED=0 GOOS=$$goos GOARCH=$$goarch \
			go build -trimpath -ldflags "$(LDFLAGS)" -o "$(DISTDIR)/$$platform/$$bin" .; \
	done

# Print the release platform directory names, one per line.
platforms:
	@for entry in $(PLATFORMS); do echo $${entry%%:*}; done

clean:
	rm -rf $(PLATFORM_DIRS) $(DISTDIR)/host
