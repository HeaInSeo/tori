# tori

영문: [README.md](README.md) | 한글: [README.ko.md](README.ko.md)

## 현재 단계
- 단계: **Track A / Phase A-2 Duplicate Policy 최소 contract 구현 완료**
- baseline 상태: **Phase 0 baseline은 실무 개발 기준으로 확보됨**
- A-1 상태: **Phase A-1 1차 freeze(A/B/C/D/E) 완료**
- A-2 상태: **최소 duplicate collision error contract가 `rules.GroupFiles`에 반영됨**
- transport boundary 상태: **첫 번째 transport boundary pass가 반영된 상태**
- 현재 우선순위: **Track A (File/Data structuring)**

## Baseline 범위
- 현재 코어 Green baseline 패키지: `config`, `db`, `rules`, `block`, `cmd`
- `service`: CLI/in-process와 transport adapter가 함께 쓰는 app service boundary
- `transport/grpc`: gRPC adapter boundary
- `cmd`: 현재 local/in-process 진입 경로
- `protoio`: protobuf file I/O boundary

## 현재 boundary 메모
- 현재 구조는 이미 첫 번째 boundary pass를 반영한다. `service`는 app orchestration, `transport/grpc`는 RPC translation, `cmd`는 service 직접 호출, `protoio`는 protobuf file load/save를 맡는다.
- `make test-core`는 여전히 Track A 코어 baseline 명령이지만, 저장소를 더 이상 `service`가 깨진 영역인 것처럼 설명하지 않는다.
- 다음 architecture 과제는 broad transport feature 확장이 아니다. 현재 초점은 `contract ownership`, `remote surface`, `Phase 2 migration order`를 기준선으로 유지하면서 너무 이른 broad import migration을 피하는 것이다.

## 명령
- `make test-core`: 코어 baseline 테스트와 external `api-protos` import diffusion guardrail 실행
- `make lint`: 코어+cmd 범위 fail gate
- `make lint-security`: 코어 범위 보안 관찰(report-only, `sqlclosecheck`/`gosec`)
- `make vuln`: 코어 범위 취약점 관찰(report-only)
- `make vuln-all`: 전체 범위 취약점 관찰(report-only)
- `make test`: 저장소 전체 상태 확인

## 설계 문서
- [`docs/tori_living_technical_draft_v0.2.md`](docs/tori_living_technical_draft_v0.2.md)
- [`docs/fileblock_rule_resolution_spec_v0.1.1.md`](docs/fileblock_rule_resolution_spec_v0.1.1.md)
- [`docs/phase_a1_current_semantics_freeze_workplan_v0.1.1.md`](docs/phase_a1_current_semantics_freeze_workplan_v0.1.1.md)
- [`docs/tori_phase0_environment_setup_checklist_v0.1.md`](docs/tori_phase0_environment_setup_checklist_v0.1.md)
- [`docs/duplicate_policy_design_note_v0.1.md`](docs/duplicate_policy_design_note_v0.1.md)
- [`docs/duplicate_policy_contract_v0.1.md`](docs/duplicate_policy_contract_v0.1.md)
- [`docs/architecture/transport_boundary.md`](docs/architecture/transport_boundary.md)
- [`docs/architecture/proto_contract_ownership.md`](docs/architecture/proto_contract_ownership.md)
- [`docs/architecture/remote_rpc_surface_decision_note.md`](docs/architecture/remote_rpc_surface_decision_note.md)
- [`docs/architecture/proto_canonicalization_phase1_note.md`](docs/architecture/proto_canonicalization_phase1_note.md)
- [`docs/architecture/proto_canonicalization_phase2_migration_order_note.md`](docs/architecture/proto_canonicalization_phase2_migration_order_note.md)
- [`docs/pipeline_binding_docs_index_v0.1.md`](docs/pipeline_binding_docs_index_v0.1.md)

## 보류 영역(명시)
- 최종 proto contract ownership은 아직 고정되지 않았다.
- external `api-protos` 사용은 현재 baseline에서 허용되지만, 최종 canonical ownership 모델로 선언된 상태는 아니다.
- Gateway API / GRPCRoute 배치, mesh policy, protobuf-neutral service DTO 완전 분리는 계속 보류 범위다.
