# Track A / A-2 Duplicate Policy Minimum Contract v0.1
### 상태: 구현 직전 계약 초안
### 기준선: A-1 current behavior freeze + A-2 duplicate policy design

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

## 4. 반환 형태 제안 (v0.1 권장)

세 가지 후보를 비교했을 때, 현재 tori에는 아래 조합이 가장 적합하다.

1. 함수 `error`를 기본 반환으로 사용한다.
2. 동시에 구조화된 duplicate report entry를 생성한다.
3. `invalid row + reason`은 상위 계층 요약/집계용 표현으로 사용한다.

선정 이유:
- 현재 `GroupFiles`/`FilterGroups` 경로는 단일 함수 흐름이므로 error 도입이 작고 명확하다.
- pair-end 기준 duplicate는 데이터 품질 문제 성격이 강해 fail-fast가 자연스럽다.
- report entry를 함께 남기면 디버깅 가능성과 운영 가시성을 확보할 수 있다.

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

구현 시작 전 추가로 필요한 항목은 1개만 권장:

- `duplicate report entry`를 어디에 수집/반환할지(함수 반환 struct 또는 별도 collector) 시그니처 선택

이 항목이 정해지면 구현 범위를 작은 단위로 고정할 수 있다.
