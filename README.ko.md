# tori

영문: [README.md](README.md) | 한글: [README.ko.md](README.ko.md)

## 현재 단계
- 단계: **Track A / Phase A-1 Current Semantics Freeze 진행 중**
- baseline 상태: **Phase 0 baseline은 실무 개발 기준으로 확보됨**
- 현재 우선순위: **Track A (File/Data structuring)**

## 코어 baseline 범위
- 현재 코어 Green baseline 패키지: `config`, `db`, `rules`, `block`, `cmd`
- `service` 패키지는 보류된 transport/runtime 영역이므로 현재 코어 baseline에서 의도적으로 제외된다.

## 기본 명령
- `make test-core`: 현재 Phase 0 / Track A 기준 코어 Green baseline 확인
- `make lint`: 코어+cmd 범위 fail gate
- `make lint-security`: 코어 범위 보안 관찰(report-only, `sqlclosecheck`/`gosec`)
- `make vuln`: 코어 범위 취약점 관찰(report-only)
- `make vuln-all`: 전체 범위 취약점 관찰(report-only)
- `make test`: 저장소 전체 상태 확인(보류 영역 `service` 포함)

## 설계 문서
- [`docs/tori_living_technical_draft_v0.2.md`](docs/tori_living_technical_draft_v0.2.md)
- [`docs/fileblock_rule_resolution_spec_v0.1.md`](docs/fileblock_rule_resolution_spec_v0.1.md)
- [`docs/phase_a1_current_semantics_freeze_workplan_v0.1.md`](docs/phase_a1_current_semantics_freeze_workplan_v0.1.md)
- [`docs/tori_phase0_environment_setup_checklist_v0.1.md`](docs/tori_phase0_environment_setup_checklist_v0.1.md)

## 보류 영역(명시)
- `service` 테스트 실패는 stale transport/runtime 계약 문제로 분류되어 보류 중이다.
- `test-core`는 이 실패를 숨기기 위한 것이 아니라, 코어 baseline을 분리해 단계적으로 안정화하기 위한 명령이다.
