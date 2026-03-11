# AGENTS.md

## 1. 목적

이 저장소의 AGENTS.md 는 Codex 및 기타 에이전트가 tori 프로젝트에서 작업할 때 따라야 할 상위 지침을 정의한다.

이 문서는 상세 설계 전체를 담지 않는다.  
대신 아래를 고정한다.

- 먼저 읽어야 할 문서
- 현재 우선순위
- 작업 방식
- 금지 사항
- 검증/보고 형식

---

## 2. 프로젝트 한 줄 설명

tori는 snapshot 기반의 데이터 카탈로그/파일 구조화 및 binding 계층을 지향하는 프로젝트다.

현재는 범용 K8s 데이터플레인으로 바로 확장하기보다, 먼저 File/Data 구조화 계층과 재연성 중심의 코어 모델을 안정화하는 단계에 있다.

---

## 3. 현재 최우선순위

현재 최우선순위는 다음과 같다.

1. **Phase 0 개발환경 baseline 확보**
2. **Track A. File/Data 구조화 계층 확정**
3. 그 안에서 **FileBlock Rule Resolution**
4. 그 다음 **Phase A-1 Current Semantics Freeze**

즉, 현재는 gRPC/K8s runtime 확장보다 **코어 구조화 계층과 개발 baseline** 이 우선이다.

---

## 4. 반드시 먼저 읽을 문서

작업을 시작하기 전에 반드시 아래 문서를 이 순서대로 읽어라.

1. `docs/tori_living_technical_draft_v0.2.md`
2. `docs/fileblock_rule_resolution_spec_v0.1.md`
3. `docs/phase_a1_current_semantics_freeze_workplan_v0.1.md`
4. `docs/tori_phase0_environment_setup_checklist_v0.1.md`

문서와 코드가 충돌하면, 충돌 사실을 먼저 보고하고 임의로 확장 해석하지 마라.

---

## 5. 핵심 설계 기준선

다음 기준은 현재 프로젝트에서 유지해야 한다.

- tori는 watcher가 아니라 **snapshot 기반 catalog/binding 계층**이다.
- **재연성**은 최상위 요구사항이다.
- pipeline은 **immutable Logic Spec** 과 **mutable하지만 실행 시점에는 고정되는 Execution Profile** 로 분리한다.
- **DataBlock은 dataset package**, **FileBlock은 typed view**, **Row는 fanout 단위**다.
- low-level rule은 내부 구현으로 두고, 사용자에게는 인식 결과와 preview를 제공하는 방향을 유지한다.
- Resolved Run Plan은 단순 실행 요청이 아니라 **재연성 고정 문서**로 간주한다.
- 현재 Track A에서는 pair-end 예시를 출발점으로 삼되, 장기적으로는 **multi-role typed schema** 로 일반화 가능한 방향을 유지한다.

---

## 6. 작업 방식

작업은 아래 원칙을 따른다.

### 6.1 spec-first

- 먼저 문서 기준선을 읽고 이해한다.
- 문서에 없는 대형 변경을 바로 구현하지 않는다.
- 설계 충돌이 있으면 먼저 보고한다.

### 6.2 phase-bounded

- 한 번에 큰 구조를 뒤집지 않는다.
- 작은 단계로 나누어 진행한다.
- 각 단계는 목표, 비목표, 성공 기준, 영향 범위를 가져야 한다.

### 6.3 rollback-tolerant

- 롤백 가능성을 정상으로 간주한다.
- 단, 롤백은 작은 범위 안에서 가능해야 한다.
- 코드만 바꾸고 문서를 방치하지 않는다.

### 6.4 one-topic-at-a-time

- 현재 주제를 벗어나는 확장을 함부로 시작하지 않는다.
- 현재 우선순위가 아닌 transport/runtime/K8s 확장은 보류한다.

---

## 7. 현재 허용되는 주요 작업 범위

현재 허용되는 작업은 주로 아래다.

- 테스트 실패 재현 및 분류
- lint/toolchain baseline 도입
- Makefile / README 최소 기준 정리
- rule resolver 현재 의미론 분석
- fixture / golden test 추가
- FileBlock / Row 구조화 관련 작은 리팩터
- Track A 문서와 코드의 정합성 점검

---

## 8. 현재 금지 또는 보류되는 작업

아래 작업은 명시적 지시 없이 먼저 시작하지 마라.

- 대규모 패키지 구조 재편
- gRPC 서버 전면 복구
- K8s overlay / Tilt / ko / 배포 루프 확장
- production dependency 대량 도입
- 문서 기준선과 무관한 기능 확장
- Track A를 건너뛰고 Binding/Runtime 영역 먼저 확장

---

## 9. 코드 변경 전 행동 규칙

코드 변경 전에 반드시 아래를 수행하라.

1. 현재 이해한 기준선을 짧게 요약한다.
2. 실제 저장소 상태를 조사한다.
3. 문제를 분류한다.
4. 가장 작은 작업 단위를 제안한다.
5. 위험/영향 범위를 적는다.
6. 그 뒤에만 구현을 시작한다.

작업이 길어질 경우 중간 보고를 남겨라.  
중간 보고에는 다음을 포함하라.

- 지금까지 확인한 사실
- 예상과 다른 점
- 다음 작은 단계
- 남은 위험

---

## 10. 테스트 / 검증 원칙

- 가능한 한 먼저 테스트 또는 fixture 기준을 정한다.
- as-is semantics를 건드릴 때는 snapshot/golden test를 선호한다.
- 실패를 숨기기 전에 왜 숨기는지 기록한다.
- Track A와 무관한 실패는 무조건 지금 해결하려 하지 말고 분류 후 보류할 수 있다.

---

## 11. 패키지 경계 원칙

향후 core / transport / runtime / cli 경계를 강화할 예정이므로, 새 코드 추가 시 아래를 의식하라.

- core domain 로직은 transport/runtime에 오염되지 않게 유지한다.
- 임시로 섞여 있더라도, 어디가 경계 위반 후보인지 보고에 명시한다.
- depguard 또는 유사 정책 도입 전이라도, import 방향은 보수적으로 유지한다.

---

## 12. 문서 업데이트 원칙

구현 결과가 문서와 충돌하거나 새로운 구조가 드러나면, 아래를 분리해서 보고하라.

- 문서 수정이 필요한가
- 코드 수정이 필요한가
- 단계 경계를 바꿔야 하는가
- 다음 설계단위로 넘어가도 되는가

문서 수정이 필요한 경우, 어느 문서의 어느 섹션이 바뀌어야 하는지 함께 제안하라.

---

## 13. 결과 보고 형식

작업 결과 보고는 아래 형식을 따른다.

### 1) 현재 이해한 기준선 요약
### 2) 실제 저장소 조사 결과
### 3) 문제 분류
- 즉시 blocker
- 후순위 과제
- 보류 영역

### 4) 이번 턴의 작업 단위
### 5) 위험/영향 범위
### 6) 검증 결과
### 7) 다음 가장 작은 작업 제안

---

## 14. 현재 세션에서 가장 중요한 해석

지금 tori는 “모든 것을 한 번에 올리는 단계”가 아니다.

현재는 다음이 핵심이다.

- 코어 구조화 계층을 안정화한다.
- 개발 baseline 을 먼저 확보한다.
- pair-end 예시를 출발점으로 current semantics 를 고정한다.
- 그 위에서 multi-role schema 일반화로 점진적으로 올라간다.

이 우선순위를 어기지 마라.

현재 `service` / gRPC / protobuf 관련 실패는 Track A 직접 범위가 아니다.
또한 일부 transport contract는 별도 buf/proto 저장소와 연관될 가능성이 있으므로,
현재 단계에서는 이 영역의 아키텍처 적합성(분리의 타당성)을 평가하거나 복구하지 않는다.

Phase 0에서는 해당 실패를 임시 격리 또는 후순위 보류 대상으로 분류하고,
코어 구조화 계층(rule resolution, FileBlock, Row, snapshot) baseline 확보를 우선한다.