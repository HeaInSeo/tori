# tori Phase 0 baseline:
# - tori uses an independent golangci-lint pin (not kube-slint alignment).
# - exact pinned version: v2.11.3
# - local binary only: ./bin/golangci-lint

LOCALBIN := $(CURDIR)/bin
REPORT_DIR := $(CURDIR)/reports

GOLANGCI_LINT := $(LOCALBIN)/golangci-lint
GOLANGCI_LINT_VERSION := v2.11.3

GOVULNCHECK := $(LOCALBIN)/govulncheck
GOVULNCHECK_VERSION := v1.1.4

# Track A focused lint scope (service/transport/runtime is intentionally excluded in Phase 0).
PKGS_LINT := ./config ./db ./rules ./block ./cmd/...
PKGS_SECURITY := ./db ./rules ./block
PKGS_TEST_CORE := ./config ./db ./rules ./block ./cmd/...

.PHONY: test test-core fmt vet lint lint-security vuln vuln-all golangci-lint govulncheck

test:
	go test -race ./...

test-core:
	go test -race $(PKGS_TEST_CORE)

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
		TMP="$$(mktemp -d)"; \
		curl -fsSL "$$URL" -o "$$TMP/lint.tgz"; \
		tar -xzf "$$TMP/lint.tgz" -C "$$TMP"; \
		cp "$$TMP/golangci-lint-$$VER-$$OS-$$ARCH/golangci-lint" "$(GOLANGCI_LINT)"; \
		chmod +x "$(GOLANGCI_LINT)"; \
		rm -rf "$$TMP"'

govulncheck:
	@mkdir -p "$(LOCALBIN)"
	@test -x "$(GOVULNCHECK)" || GOBIN="$(LOCALBIN)" go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

lint: golangci-lint
	@mkdir -p "$(REPORT_DIR)"
	@$(GOLANGCI_LINT) run $(PKGS_LINT) | tee "$(REPORT_DIR)/lint.txt"

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

vuln: govulncheck
	@mkdir -p "$(REPORT_DIR)"
	@set +e; \
	$(GOVULNCHECK) $(PKGS_SECURITY) 2>&1 | tee "$(REPORT_DIR)/govulncheck-core.txt"; \
	echo "govulncheck_core_exit=$$?" | tee "$(REPORT_DIR)/govulncheck-core.summary"

vuln-all: govulncheck
	@mkdir -p "$(REPORT_DIR)"
	@set +e; \
	$(GOVULNCHECK) ./... 2>&1 | tee "$(REPORT_DIR)/govulncheck-all.txt"; \
	echo "govulncheck_all_exit=$$?" | tee "$(REPORT_DIR)/govulncheck-all.summary"
