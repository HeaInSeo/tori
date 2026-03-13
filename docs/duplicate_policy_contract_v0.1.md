# Track A / A-2 Duplicate Policy Minimum Contract v0.1
### 상태: 최소 구현 contract (`rules.GroupFiles`) 반영됨
### 기준선: A-1 current behavior freeze + A-2 duplicate policy design

## 0. 구현 상태 메모

2026-03-13 기준 최소 contract는 `rules.GroupFiles`에 반영되었다.

- duplicate collision 발생 시 `error` 반환
- 반환 타입은 `DuplicateCollisionError`
- entry reason code는 `duplicate_role_in_row`
- A-1 Fixture D overwrite anchor는 `rules/current_semantics_freeze_test.go`에서 historical skip으로 유지

즉 이 문서는 더 이상 구현 전 초안만이 아니라, 현재 활성 contract의 최소 기준선으로 사용한다.

## 1. 목적

이 문서는 duplicate policy를 구현 가능한 최소 contract로 고정한다.
범위는 Track A rule resolver의 duplicate collision 처리에 한정한다.

## 2. duplicate collision 정의

duplicate collision은 아래 조건을 모두 만족할 때 발생한다.

1. 동일 row key
2. 동일 column/role key
3. 서로 다른 source file name이 2개 이상 매핑됨

현재 구현(as-is)은 마지막 값 overwrite다.
이 문서의 contract는 future behavior(구현 목표)를 정의한다.

## 3. 최소 reason code

v0.1 최소 reason code:

- `duplicate_role_in_row`
  - 의미: 한 row에서 동일 role(column)에 복수 후보가 감지됨
  - 기본 심각도: error

확장 reason code는 다음 단계로 보류한다.

## 4. 반환 형태 확정 (v0.1)

v0.1 시그니처 결정:

1. 함수 `error`를 기본 반환으로 사용한다.
2. `DuplicateCollisionError`(typed error)가 `[]DuplicateReportEntry`를 포함한다.
3. `invalid row + reason`은 상위 계층 요약/집계용 표현으로 사용한다.

예시 형태(개념):

```go
type DuplicateReportEntry struct {
    ReasonCode      string
    RowKey          string
    RoleKey         string
    Candidates      []string
    SourceFileNames []string
    Diagnostic      string
}

type DuplicateCollisionError struct {
    Entries []DuplicateReportEntry
}
```

collector 배제 이유:
- 현재 `GroupFiles`/`FilterGroups` 경로는 단일 함수 흐름이므로 error 도입이 작고 명확하다.
- pair-end 기준 duplicate는 데이터 품질 문제 성격이 강해 fail-fast가 자연스럽다.
- collector는 시그니처/수명 관리 복잡도를 늘리고, 현재 Track A 최소 범위를 넘어선다.
- typed error 내부 entry로 충분한 진단 가능성을 확보할 수 있다.

## 5. 최소 필드 집합 (structured report entry)

v0.1 최소 필드:

1. `reason_code` (예: `duplicate_role_in_row`)
2. `row_key`
3. `role_key` (현재 column key)
4. `candidates` (충돌한 후보 파일명 배열)
5. `source_file_names` (입력 파일명 컨텍스트; v0.1에서는 candidates와 동일 허용)
6. `diagnostic` (optional text, 비어 있어도 됨)

권장 규칙:
- `candidates`는 deterministic ordering(예: sort)으로 기록
- `diagnostic`은 정책 설명이 아니라 관찰 사실만 기록

## 6. expectedColCount invalid 판정과의 관계

현재 `FilterGroups(expectedColCount)`는 count 기반 valid/invalid 분리만 수행한다.
duplicate contract는 count 판정 이전(또는 독립 단계)에서 먼저 평가해야 한다.

v0.1 제안 순서:

1. grouping 중 duplicate collision 감지
2. collision 있으면 reason code 포함 error/report 생성
3. collision 없는 결과에 대해 expectedColCount 기반 invalid 판정 수행

즉 duplicate는 단순 invalid row와 다른 class의 오류로 분리한다.

## 7. pair-end에서 error 중심으로 보는 이유

pair-end(`R1`,`R2`)에서 동일 role 중복은 일반적으로:

- 입력 혼선
- 파일 선택 오류
- 중복 업로드/스캔 이슈

에 가깝다. 현재 overwrite는 충돌 사실을 숨길 수 있으므로,
v0.1에서는 `error`를 기본으로 두는 것이 데이터 품질/재현성 관점에서 더 안전하다.

## 8. current behavior vs future contract 분리

다음 둘은 역할이 다르다.

1. A-1 Fixture D: current overwrite behavior를 기록하는 regression anchor
2. 본 contract: A-2에서 구현해야 할 future duplicate policy 목표

따라서 A-1 테스트 존재는 A-2 정책 구현과 충돌하지 않는다.
구현 단계에서는 정책 전환용 테스트를 별도로 추가한다.

## 9. 구현 진입 판정

판정: 이 최소 contract로 다음 턴 테스트-우선 구현에 진입 가능.

추가 선행 조건 없음.

## 10. 구현 직전 테스트 계획 (초안)

1. duplicate 발생 시 typed error 반환
- 동일 row/role 충돌 입력에서 `error`가 발생하고, `DuplicateCollisionError`로 type assert 가능해야 한다.

2. typed error entry 필드 검증
- `Entries` 길이, `reason_code`, `row_key`, `role_key`, `candidates`, `source_file_names`가 기대값과 일치해야 한다.

3. non-duplicate 정상 케이스 회귀 확인
- 충돌 없는 입력은 기존 grouping/invalid 분리 동작을 유지하고 duplicate error가 발생하지 않아야 한다.
