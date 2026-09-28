# tori Phase 0 baseline:
# - tori uses an independent golangci-lint pin (not kube-slint alignment).
# - exact pinned version: v2.11.3
# - local binary only: ./bin/golangci-lint

SHELL := /bin/bash
.SHELLFLAGS := -o pipefail -c

LOCALBIN := $(CURDIR)/bin
REPORT_DIR := $(CURDIR)/reports
TORI_NAS_FIXTURE_ROOT ?= /mnt/genomics-test/tori-public-fixtures
TORI_SHARED_FIXTURE_ROOT ?= $(TORI_NAS_FIXTURE_ROOT)

GOLANGCI_LINT := $(LOCALBIN)/golangci-lint
GOLANGCI_LINT_VERSION := v2.11.3

GOVULNCHECK := $(LOCALBIN)/govulncheck
GOVULNCHECK_VERSION := v1.1.4
BUF := $(LOCALBIN)/buf

# Track A focused lint scope (service/transport/runtime is intentionally excluded in Phase 0).
PKGS_LINT := ./config ./db ./rules ./block ./cmd/...
PKGS_SECURITY := ./db ./rules ./block
PKGS_TEST_CORE := ./config ./db ./rules ./block ./cmd/... ./service
# Packages that must stay in PKGS_TEST_CORE (tori#40: ./service was silently
# omitted from the required test/coverage scope). test-scope-check fails if any
# of them is dropped.
PKGS_TEST_REQUIRED := ./service

.PHONY: doctor test test-core coverage test-guardrail test-scope-check test-skip-report test-shared-fs-fixtures test-nas-fixtures fmt vet lint lint-depguard lint-security lint-security-check proto-lint vuln vuln-check vuln-all golangci-lint govulncheck

doctor:
	@if [[ -n "$${GOROOT:-}" && ! -d "$$GOROOT" ]]; then \
		echo "invalid GOROOT: $$GOROOT"; \
		echo "remove or correct the GOROOT export, then rerun make doctor"; \
		exit 1; \
	fi
	@command -v go >/dev/null 2>&1 || { echo "go is not available on PATH"; exit 1; }
	@go version

test:
	go test -race -shuffle=on -count=1 ./...

test-core: test-scope-check test-guardrail
	go test -race -shuffle=on -count=1 $(PKGS_TEST_CORE)

test-scope-check:
	@missing="$(filter-out $(PKGS_TEST_CORE),$(PKGS_TEST_REQUIRED))"; \
	if [[ -n "$$missing" ]]; then \
		echo "PKGS_TEST_CORE is missing required package(s): $$missing"; \
		exit 1; \
	fi
	@echo "[test-scope] PKGS_TEST_CORE covers required: $(PKGS_TEST_REQUIRED)"

coverage: test-scope-check
	@mkdir -p "$(REPORT_DIR)"
	go test -race -shuffle=on -count=1 $(PKGS_TEST_CORE) -coverprofile="$(REPORT_DIR)/cover.out" -covermode=atomic
	go tool cover -func="$(REPORT_DIR)/cover.out" | tee "$(REPORT_DIR)/coverage.txt"

# Executed/skipped test counts plus every skipped test name (tori#23), so a skip
# can't pass as coverage unnoticed. Counts include subtests.
test-skip-report: test-scope-check
	@mkdir -p "$(REPORT_DIR)"
	go test -count=1 -json $(PKGS_TEST_CORE) > "$(REPORT_DIR)/test-events.json"
	@awk '/"Action":"(pass|fail|skip)"/ && /"Test":/ { \
		match($$0, /"Action":"[a-z]+"/); a = substr($$0, RSTART + 10, RLENGTH - 11); n[a]++; \
		if (a == "skip") { match($$0, /"Package":"[^"]*","Test":"[^"]*"/); s[++k] = substr($$0, RSTART, RLENGTH) } \
	} END { \
		printf "tests pass=%d fail=%d skip=%d\n", n["pass"], n["fail"], n["skip"]; \
		for (i = 1; i <= k; i++) print "SKIP " s[i] \
	}' "$(REPORT_DIR)/test-events.json" | tee "$(REPORT_DIR)/test-skip-summary.txt"

test-shared-fs-fixtures:
	TORI_SHARED_FIXTURE_ROOT="$(TORI_SHARED_FIXTURE_ROOT)" go test -race ./block -run TestSharedFSFixtureSmoke

test-nas-fixtures: test-shared-fs-fixtures

test-guardrail:
	go test . -run TestExternalAPIProtosImportGuardrail

fmt:
	go fmt ./...

vet:
	go vet ./...

golangci-lint:
	@mkdir -p "$(LOCALBIN)"
	@test -x "$(GOLANGCI_LINT)" || bash -c '\
		set -euo pipefail; \
		# must exist: do not fallback when tag is missing; \
		curl -fsSL "https://api.github.com/repos/golangci/golangci-lint/releases/tags/$(GOLANGCI_LINT_VERSION)" >/dev/null; \
		OS="$$(uname | tr A-Z a-z)"; \
		ARCH="$$(uname -m)"; \
		case "$$ARCH" in x86_64) ARCH=amd64 ;; aarch64|arm64) ARCH=arm64 ;; *) echo "unsupported arch: $$ARCH"; exit 1 ;; esac; \
		VER="$(GOLANGCI_LINT_VERSION)"; \
		VER="$${VER#v}"; \
		FILE="golangci-lint-$$VER-$$OS-$$ARCH.tar.gz"; \
		URL="https://github.com/golangci/golangci-lint/releases/download/$(GOLANGCI_LINT_VERSION)/$$FILE"; \
		SUM_URL="https://github.com/golangci/golangci-lint/releases/download/$(GOLANGCI_LINT_VERSION)/golangci-lint-$$VER-checksums.txt"; \
		TMP="$$(mktemp -d)"; \
		curl -fsSL "$$URL" -o "$$TMP/lint.tgz"; \
		curl -fsSL "$$SUM_URL" -o "$$TMP/checksums.txt"; \
		EXPECTED="$$(awk -v f="$$FILE" "\$$2==f{print \$$1}" "$$TMP/checksums.txt")"; \
		if [ -z "$$EXPECTED" ]; then echo "checksum not found for $$FILE"; exit 1; fi; \
		if command -v sha256sum >/dev/null 2>&1; then \
			ACTUAL="$$(sha256sum "$$TMP/lint.tgz" | awk "{print \$$1}")"; \
		elif command -v shasum >/dev/null 2>&1; then \
			ACTUAL="$$(shasum -a 256 "$$TMP/lint.tgz" | awk "{print \$$1}")"; \
		else \
			echo "no sha256 tool found (sha256sum/shasum)"; exit 1; \
		fi; \
		if [ "$$EXPECTED" != "$$ACTUAL" ]; then echo "checksum mismatch for $$FILE"; exit 1; fi; \
		tar -xzf "$$TMP/lint.tgz" -C "$$TMP"; \
		cp "$$TMP/golangci-lint-$$VER-$$OS-$$ARCH/golangci-lint" "$(GOLANGCI_LINT)"; \
		chmod +x "$(GOLANGCI_LINT)"; \
		rm -rf "$$TMP"'

govulncheck:
	@mkdir -p "$(LOCALBIN)"
	@test -x "$(GOVULNCHECK)" || GOBIN="$(LOCALBIN)" go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

lint: golangci-lint lint-depguard
	@mkdir -p "$(REPORT_DIR)"
	@$(GOLANGCI_LINT) run $(PKGS_LINT) | tee "$(REPORT_DIR)/lint.txt"

lint-depguard: golangci-lint
	@mkdir -p "$(REPORT_DIR)"
	@$(GOLANGCI_LINT) run --enable-only depguard $(PKGS_LINT) | tee "$(REPORT_DIR)/lint-depguard.txt"

proto-lint:
	@test -x "$(BUF)" || { echo "$(BUF) is required for proto-lint"; exit 1; }
	@BUF_CACHE_DIR="$${BUF_CACHE_DIR:-/tmp/buf-cache}" "$(BUF)" lint

lint-security: golangci-lint
	@mkdir -p "$(REPORT_DIR)"
	@echo "[phase0] security scan scope: $(PKGS_SECURITY)" | tee "$(REPORT_DIR)/lint-security-summary.txt"
	@set +e; \
	$(GOLANGCI_LINT) run --enable-only sqlclosecheck $(PKGS_SECURITY) \
	| tee "$(REPORT_DIR)/sqlclosecheck.txt"; \
	echo "sqlclosecheck_exit=$$?" | tee -a "$(REPORT_DIR)/lint-security-summary.txt"
	@set +e; \
	$(GOLANGCI_LINT) run --enable-only gosec $(PKGS_SECURITY) \
	| tee "$(REPORT_DIR)/gosec.txt"; \
	echo "gosec_exit=$$?" | tee -a "$(REPORT_DIR)/lint-security-summary.txt"

lint-security-check: golangci-lint
	@mkdir -p "$(REPORT_DIR)"
	$(GOLANGCI_LINT) run --enable-only sqlclosecheck $(PKGS_SECURITY) | tee "$(REPORT_DIR)/sqlclosecheck.txt"
	$(GOLANGCI_LINT) run --enable-only gosec $(PKGS_SECURITY) | tee "$(REPORT_DIR)/gosec.txt"

vuln: govulncheck
	@mkdir -p "$(REPORT_DIR)"
	@set +e; \
	$(GOVULNCHECK) $(PKGS_SECURITY) 2>&1 | tee "$(REPORT_DIR)/govulncheck-core.txt"; \
	echo "govulncheck_core_exit=$$?" | tee "$(REPORT_DIR)/govulncheck-core.summary"

vuln-check: govulncheck
	@mkdir -p "$(REPORT_DIR)"
	$(GOVULNCHECK) $(PKGS_SECURITY) 2>&1 | tee "$(REPORT_DIR)/govulncheck-core.txt"

vuln-all: govulncheck
	@mkdir -p "$(REPORT_DIR)"
	@set +e; \
	$(GOVULNCHECK) ./... 2>&1 | tee "$(REPORT_DIR)/govulncheck-all.txt"; \
	echo "govulncheck_all_exit=$$?" | tee "$(REPORT_DIR)/govulncheck-all.summary"
