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
- The next architecture task is not broad transport feature expansion. The current concern is keeping contract ownership, remote surface, and Phase 2 migration order explicit without starting broad import migration too early.

## Commands
- `make doctor` checks that `GOROOT`, when set, points to an existing Go installation before Go-based targets run.
- `make test-core` runs the core baseline tests and the external `api-protos` removal guardrail.
- `make test-shared-fs-fixtures` runs the opt-in shared filesystem fixture smoke test using `TORI_SHARED_FIXTURE_ROOT` (default: `/mnt/genomics-test/tori-public-fixtures`; NAS is active, Lustre is deferred until the fixture mount is ready).
- `make test-nas-fixtures` is a compatibility alias for the shared filesystem fixture smoke test.
- `make lint`: fail gate for core + cmd scope
- `make lint-security`: report-only security observation (`sqlclosecheck`, `gosec`) for core scope
- `make vuln`: report-only vuln scan for core scope
- `make vuln-all`: report-only vuln scan for all packages
- `make test`: full repository status check

If `make doctor` reports an invalid `GOROOT`, remove or correct that shell environment export. The repository requires Go 1.25.5; a stale downloaded-toolchain path must not be used as `GOROOT`.

## Design Documents
- [`docs/tori_living_technical_draft_v0.2.md`](docs/tori_living_technical_draft_v0.2.md)
- [`docs/fileblock_rule_resolution_spec_v0.1.1.md`](docs/fileblock_rule_resolution_spec_v0.1.1.md)
- [`docs/phase_a1_current_semantics_freeze_workplan_v0.1.1.md`](docs/phase_a1_current_semantics_freeze_workplan_v0.1.1.md)
- [`docs/tori_phase0_environment_setup_checklist_v0.1.md`](docs/tori_phase0_environment_setup_checklist_v0.1.md)
- [`docs/duplicate_policy_design_note_v0.1.md`](docs/duplicate_policy_design_note_v0.1.md)
- [`docs/duplicate_policy_contract_v0.1.md`](docs/duplicate_policy_contract_v0.1.md)
- [`docs/track_a_pair_end_regression_contract_v0.1.md`](docs/track_a_pair_end_regression_contract_v0.1.md)
- [`docs/track_a_alignment_index_typed_view_candidate_v0.1.md`](docs/track_a_alignment_index_typed_view_candidate_v0.1.md)
- [`docs/track_a_alignment_index_probe_result_v0.1.md`](docs/track_a_alignment_index_probe_result_v0.1.md)
- [`docs/track_a_role_normalization_seam_v0.1.md`](docs/track_a_role_normalization_seam_v0.1.md)
- [`docs/track_a_role_normalization_decision_v0.1.md`](docs/track_a_role_normalization_decision_v0.1.md)
- [`docs/track_a_fixture_probe_vs_typed_view_rule_v0.1.md`](docs/track_a_fixture_probe_vs_typed_view_rule_v0.1.md)
- [`docs/architecture/transport_boundary.md`](docs/architecture/transport_boundary.md)
- [`docs/architecture/proto_contract_ownership.md`](docs/architecture/proto_contract_ownership.md)
- [`docs/architecture/proto_ownership_sprint_plan.md`](docs/architecture/proto_ownership_sprint_plan.md)
- [`docs/architecture/remote_rpc_surface_decision_note.md`](docs/architecture/remote_rpc_surface_decision_note.md)
- [`docs/architecture/proto_canonicalization_phase1_note.md`](docs/architecture/proto_canonicalization_phase1_note.md)
- [`docs/architecture/proto_canonicalization_phase2_migration_order_note.md`](docs/architecture/proto_canonicalization_phase2_migration_order_note.md)
- [`docs/pipeline_binding_docs_index_v0.1.md`](docs/pipeline_binding_docs_index_v0.1.md)
- [`docs/track_a_shared_fs_fixture_sprint_plan_v0.1.md`](docs/track_a_shared_fs_fixture_sprint_plan_v0.1.md)
- [`docs/track_a_nas_completion_remaining_work_2026_06_12.md`](docs/track_a_nas_completion_remaining_work_2026_06_12.md)
- [`docs/product_readiness_sprint_plan_v0.1.md`](docs/product_readiness_sprint_plan_v0.1.md)

## Deferred Area (Explicit)
- Final proto contract ownership is narrowed but not fully closed for all services yet.
- `tori` active code paths no longer depend on external `api-protos`.
- `syncfolders` source exists under local proto ownership, but remote RPC exposure remains deferred and local-only.
- Gateway API / GRPCRoute placement, mesh policy, and full protobuf-neutral service DTO separation remain deferred.
