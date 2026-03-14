# Track A / A-2 Canonical Column Policy Design Note v0.1
### 상태: 설계 메모 초안 (구현 전)
### 기준선: A-1 current behavior freeze + A-2 duplicate policy minimum contract 이후

## 1) 현재 current behavior 요약

- `ruleSet.Header`는 CSV header 행에 `Row + headers` 형태로 직접 사용된다.
- 실제 데이터 컬럼 채움 순서는 `resultMap`에서 발견한 `colKey` 집합을 모아 `sort.Strings` 한 결과를 따른다.
- 따라서 현재 `ExportResultsCSV`는 `header`와 discovered `colKey`를 동시에 사용하지만, 두 기준이 항상 정렬되거나 일치한다고 보장하지 않는다.
- A-1 Fixture Set E는 이 serialization current behavior를 historical/current anchor로 기록한다.

## 2) 왜 문제인가

- 재연성 관점에서 header/column surface가 어디를 기준으로 고정되는지 불명확하다.
- preview / CSV / FileBlock surface 사이에 같은 role 의미가 다르게 보일 가능성이 있다.
- pair-end 예시를 넘는 확장을 당장 시작하지 않더라도, 현재 pair-end 범위 안에서 column 기준선은 분리해 둘 필요가 있다.

## 3) 정책 옵션 비교

| 옵션 | 설명 | 장점 | 단점 |
|---|---|---|---|
| `header authoritative` | `header`를 canonical column 기준으로 사용 | 사용자 의도 표현이 단순함 | discovered key와 mismatch 시 추가 판정 필요 |
| `discovered key authoritative` | 실제 발견된 `colKey`를 canonical 기준으로 사용 | current behavior와 가깝고 구현 단순 | `header`의 의미가 약해지고 preview 정합성이 흔들릴 수 있음 |
| `validated alignment / canonicalization step` | `header`와 discovered key를 비교/정렬해 canonical column 집합을 만든 뒤 export | 기준선이 가장 명시적 | 현재 단계에서 별도 정렬/검증 단계 정의가 필요 |

임시 권장 방향:
- 다음 최소 patch에서는 `ruleSet.Header`를 export/display canonical 후보로 우선 보고, discovered `colKey`는 completeness/validation 확인 축으로 다루는 가설을 먼저 검증하는 편이 보수적이다.
- 이는 최종 결정이 아니라, current behavior와 policy 후보를 가장 작은 범위에서 분리해 보기 위한 provisional recommendation 이다.

전환 메모:
- current behavior anchor와 provisional direction은 기본 실행 경로에서 동시에 유지될 수 없다.
- 다음 최소 patch에서는 `ExportResultsCSV` ordering 기준 변경, current behavior anchor test의 일부 수정/retire, provisional behavior test의 승격 가능성을 함께 다뤄야 한다.
- 이 전환은 column ordering 기준선 이동에만 한정하며, missing/extra column 정책이나 completeness/validation 전체 규칙은 아직 확정하지 않는다.
- multi-role 일반화, FileBlock/UI/runtime 확장도 이번 전환 범위 밖에 둔다.

상태 메모:
- 첫 최소 patch 반영 후 `ExportResultsCSV`의 data ordering 기준은 이제 `ruleSet.Header`를 따른다.
- header/discovered ordering divergence를 기록하던 current behavior anchor는 active baseline에서 retire 되었고, historical anchor로만 남는다.
- discovered `colKey`는 ordering 기준이 아니라 completeness/validation 확인 축으로 남겨 둔다.
- 다음 가장 작은 주제는 missing/extra column policy 정리다.

## 4) 이번 단계의 1차 비목표

- multi-role 일반화는 이번 메모 범위 밖이다.
- service/runtime 연계는 이번 메모 범위 밖이다.
- 실제 구현 착수와 serialization 동작 변경은 이번 메모 범위 밖이다.

## 5) 다음 최소 구현 단위 제안

- `ExportResultsCSV` 진입 전, `header`와 discovered `colKey`의 관계를 요약하는 작은 테스트/문서 contract를 먼저 추가해 canonical column 기준 후보를 고정한다.
