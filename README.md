# tori

English: [README.md](README.md) | Korean: [README.ko.md](README.ko.md)

## Current Stage
- Phase: **Phase 0 baseline stabilization**
- Immediate priority: **Track A (File/Data structuring)**

## Baseline Scope
- Core green-baseline packages: `config`, `db`, `rules`, `block`, `cmd`
- `service` is a deferred transport/runtime area and is intentionally excluded from the current core baseline.

## Commands
- `make test-core`: current Phase 0 / Track A core green-baseline command
- `make lint`: fail gate for core + cmd scope
- `make lint-security`: report-only security observation (`sqlclosecheck`, `gosec`) for core scope
- `make vuln`: report-only vuln scan for core scope
- `make vuln-all`: report-only vuln scan for all packages
- `make test`: full repository status check (includes deferred `service` area)

## Design Documents
- [`docs/tori_living_technical_draft_v0.2.md`](docs/tori_living_technical_draft_v0.2.md)
- [`docs/fileblock_rule_resolution_spec_v0.1.md`](docs/fileblock_rule_resolution_spec_v0.1.md)
- [`docs/phase_a1_current_semantics_freeze_workplan_v0.1.md`](docs/phase_a1_current_semantics_freeze_workplan_v0.1.md)
- [`docs/tori_phase0_environment_setup_checklist_v0.1.md`](docs/tori_phase0_environment_setup_checklist_v0.1.md)

## Deferred Area (Explicit)
- `service` tests currently fail due to stale transport/runtime contracts.
- This is tracked as deferred scope in Phase 0 and is not hidden by `test-core`.
