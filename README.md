# tori

English: [README.md](README.md) | Korean: [README.ko.md](README.ko.md)

## Current Stage
- Phase: **Track A / Phase A-2 Duplicate Policy minimum contract implemented**
- Baseline status: **Phase 0 baseline is established for practical development**
- A-1 status: **Phase A-1 first freeze is completed (A/B/C/D/E)**
- A-2 status: **minimum duplicate collision error contract is active in `rules.GroupFiles`**
- Transport boundary status: **first transport boundary pass is in place**
- Immediate priority: **Track A (File/Data structuring)**

## Baseline Scope
- Core green-baseline packages: `config`, `db`, `rules`, `block`, `cmd`
- `service`: app service boundary shared by CLI/in-process and transport adapters
- `transport/grpc`: gRPC adapter boundary
- `cmd`: current local/in-process entry path
- `protoio`: protobuf file I/O boundary

## Current Boundary Notes
- Current structure already reflects the first boundary pass: `service` owns app orchestration, `transport/grpc` owns RPC translation, `cmd` uses the service path directly, and `protoio` owns protobuf file load/save.
- `make test-core` remains the Track A core baseline command, but the repository is no longer described as if `service` were a currently broken area.
- The next architecture task is not broad transport feature expansion. The smaller immediate concern is contract ownership: source `.proto`, generated code, app contract, and transport contract need to be documented more explicitly.

## Commands
- `make test-core` runs the core baseline tests and the external `api-protos` import diffusion guardrail.
- `make lint`: fail gate for core + cmd scope
- `make lint-security`: report-only security observation (`sqlclosecheck`, `gosec`) for core scope
- `make vuln`: report-only vuln scan for core scope
- `make vuln-all`: report-only vuln scan for all packages
- `make test`: full repository status check

## Design Documents
- [`docs/tori_living_technical_draft_v0.2.md`](docs/tori_living_technical_draft_v0.2.md)
- [`docs/fileblock_rule_resolution_spec_v0.1.1.md`](docs/fileblock_rule_resolution_spec_v0.1.1.md)
- [`docs/phase_a1_current_semantics_freeze_workplan_v0.1.1.md`](docs/phase_a1_current_semantics_freeze_workplan_v0.1.1.md)
- [`docs/tori_phase0_environment_setup_checklist_v0.1.md`](docs/tori_phase0_environment_setup_checklist_v0.1.md)
- [`docs/duplicate_policy_design_note_v0.1.md`](docs/duplicate_policy_design_note_v0.1.md)
- [`docs/duplicate_policy_contract_v0.1.md`](docs/duplicate_policy_contract_v0.1.md)
- [`docs/architecture/transport_boundary.md`](docs/architecture/transport_boundary.md)
- [`docs/architecture/proto_contract_ownership.md`](docs/architecture/proto_contract_ownership.md)

## Deferred Area (Explicit)
- Final proto contract ownership is not fixed yet.
- External `api-protos` usage is still allowed in the current baseline, but it is not yet declared as the final canonical ownership model.
- Gateway API / GRPCRoute placement, mesh policy, and full protobuf-neutral service DTO separation remain deferred.
