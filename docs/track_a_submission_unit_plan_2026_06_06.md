# Track A Submission Unit Plan
### 기준일: 2026-06-06
### 상태: final packaging guidance, verification green

## 1. 목적

이 문서는 2026-06-06 마감 기준으로 dirty worktree를 제출 단위 관점에서 분류한다.

목표는 Track A regression baseline에 직접 필요한 변경과 보류/별도 제출 후보를 섞지 않는 것이다.

## 2. Track A 직접 제출 후보

마감 제출에 포함할 후보:

- `Makefile`
  - `make test-shared-fs-fixtures`
  - `make test-nas-fixtures`
  - shared filesystem fixture root 변수
- `README.md`
- `README.ko.md`
  - shared filesystem fixture smoke command 안내
  - Track A 문서 링크 추가
- `rules/rules.go`
  - resolver preview
  - observed-key schema validation preview
  - typed-role validation preview
  - typed-role association preview
- `rules/rules_test.go`
  - duplicate/current semantics anchors
  - schema/typed-role/association synthetic anchors
- `block/shared_fs_fixture_smoke_test.go`
  - shared filesystem pair-end smoke
  - BAM/BAI positive association smoke
- `block/fileblock.go`
  - FileBlock generation layer에서 header exact validation을 적용한다.
- `block/fileblock_test.go`
  - missing/extra role row가 FileBlock row로 들어가지 않는 regression anchor다.
- `docs/tori_phase0_environment_setup_checklist_v0.1.md`
  - NAS/shared fixture pack 기준을 Phase 0 checklist에 반영한다.
- Track A 문서:
  - `docs/nas_fixture_run_results_v0.1.md`
  - `docs/nas_fixture_test_coverage_report_v0.1.md`
  - `docs/track_a_pair_end_regression_contract_v0.1.md`
  - `docs/track_a_alignment_index_typed_view_candidate_v0.1.md`
  - `docs/track_a_alignment_index_probe_result_v0.1.md`
  - `docs/track_a_alignment_index_validation_preview_candidate_v0.1.md`
  - `docs/track_a_fixture_probe_vs_typed_view_rule_v0.1.md`
  - `docs/track_a_role_normalization_seam_v0.1.md`
  - `docs/track_a_role_normalization_decision_v0.1.md`
  - `docs/track_a_schema_validation_boundary_design_v0.1.md`
  - `docs/track_a_shared_fs_fixture_sprint_plan_v0.1.md`
  - `docs/track_a_sidecar_association_policy_design_v0.1.md`
  - `docs/track_a_tomorrow_scope_freeze_v0.1.md`
  - `docs/track_a_final_readiness_report_2026_06_06.md`

## 3. Track A behavior note

`block/fileblock.go` 변경은 FileBlock generation behavior를 좁힌다.

기존 count-only validation은 `R1 + EXTRA`처럼 header 수와 role 수만 같은 row를 valid로 볼 수 있었다.
이번 제출 묶음은 `FilterGroupsByHeaders`를 사용해 header exact match만 valid로 본다.

이 변경은 schema validation preview 문서와 shared missing/extra semantics에 맞추기 위해 Track A 제출 범위에 포함한다.

## 4. 보류 또는 별도 제출 후보

현재 Track A 마감 제출에서 분리할 후보:

- `db/store.go`
  - sqlclosecheck 대응성 변경이다.
  - Track A FileBlock/preview baseline과 직접 관련이 없다.
- `protos/ichthys/v1/*.proto`
- `protos/ichthys/v1/*.pb.go`
- `protos/ichthys/v1/*_grpc.pb.go`
  - proto package 변경 및 generated output이다.
  - AGENTS 기준 현재 transport/protobuf 영역은 Track A 직접 범위가 아니다.
- `buf.yaml`
- `bin/buf`
- `bin/govulncheck`
  - tool/binary 산출물이다.
  - binary는 제출 단위에 포함하지 않는 편이 안전하다.
- `reports/*.txt`
- `reports/govulncheck-core.*`
  - 관찰 report 산출물이다.
  - 제출한다면 Phase 0 tooling/report 제출 단위로 분리한다.

## 5. 최종 검증

마감 제출 전 검증 명령:

```sh
make test-core
make test-shared-fs-fixtures
```

2026-06-06 마지막 확인 기준 두 명령은 green이다.

## 6. 주의

현재 worktree에는 Track A 직접 범위, Phase 0 tooling/report 범위, transport/protobuf 보류 범위가 함께 있다.

제출 전에는 파일 단위로 선택해야 한다.
특히 proto/generated 파일과 tool binary는 Track A regression baseline 제출에 섞지 않는다.
