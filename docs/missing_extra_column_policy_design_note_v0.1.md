# Track A / A-2 Missing/Extra Column Policy Design Note v0.1
### 상태: 설계 메모 초안 (구현 전)
### 기준선: canonical column ordering 첫 최소 patch 이후

## 1) 현재 current behavior 조사 요약

- `ExportResultsCSV`의 active baseline은 `ruleSet.Header` 기준 ordering 이다.
- 그러나 missing column / extra column 처리 규칙은 아직 명시적인 contract로 고정되지 않았다.
- 현재 구현에서 header 에 있지만 row 값이 없는 column 은 CSV 에서 빈칸으로 남는다.
- 현재 구현에서 discovered `colKey` 가 row 에 존재하더라도 header 에 없으면 CSV export surface 에는 드러나지 않는다.

## 2) 문제 경계

- missing column: `ruleSet.Header` 에 정의되어 있지만 실제 row 값이 없는 경우
- extra column: discovered `colKey` 또는 row value 에 존재하지만 `ruleSet.Header` 에 없는 경우
- 이 차이는 preview / CSV / FileBlock surface / validation 에서 무엇이 보이고 무엇이 숨겨지는지에 직접 영향을 준다.
- 따라서 ordering 기준선과 별도로 missing/extra 처리 기준선을 분리해 두어야 한다.

## 3) 정책 옵션 비교

| 옵션 | 설명 | 장점 | 단점 |
|---|---|---|---|
| `permissive` | missing 은 빈칸, extra 는 무시 | 현재 구현과 가깝고 범위가 작다 | 문제를 surface 에서 숨길 수 있다 |
| `warning/report` | missing/extra 를 구조적으로 보고하지만 export 는 계속 | 관찰 가능성을 높이면서도 흐름을 유지한다 | report surface 를 별도 정의해야 한다 |
| `strict` | missing/extra 를 에러로 취급 | contract 가 가장 명확하다 | 현재 Track A 최소 범위를 넘길 가능성이 있다 |

임시 권장 방향:
- 다음 최소 단계에서는 export surface 를 당분간 permissive 로 유지하는 편이 보수적이다.
- 즉 missing column 은 빈칸 export 를 유지하고, extra column 은 export surface 비노출을 유지한다.
- completeness/validation 축에서는 향후 warning/report 가능성을 열어 두되, 아직 strict error 정책으로 바로 전환하지는 않는다.

전환 메모:
- 다음 단계에서도 export surface 기준선은 그대로 유지한다.
- 다음 최소 patch는 export semantics 변경이 아니라, `rules` 계층 안의 기존 정보/구조로 missing/extra 관찰 경로를 탐색하는 데 한정한다.
- 즉 future warning/report 가능성을 보기 위한 조사 단계이며, UI/FileBlock/runtime/service 쪽 확장은 아직 다루지 않는다.

관찰 경계 메모:
- 현재 `headers + rowMap` 조합이 missing/extra 를 계산할 수 있는 최소 관찰 경계다.
- 이 경계 이후 `ExportResultsCSV`가 export surface 를 만들면 missing/extra 의미는 구조화된 diagnostics 로 남지 않고 소실된다.
- 따라서 다음 최소 patch는 export semantics 변경이 아니라, 이 경계에서 diagnostics 계산 가능성을 검증하는 작은 helper/test 탐색으로 제한하는 편이 적절하다.

상태 메모:
- test-only 관찰로도 `headers + rowMap` 경계에서 missing key / extra key 계산이 가능함을 확인했다.
- 이 경계는 future helper 후보로도 충분히 선명하지만, 아직 helper나 diagnostics 구조는 도입하지 않았다.
- 다음 단계는 helper 후보와 warning/report 구조 중 어느 쪽이 더 작은 단위인지 보수적으로 고르는 것이다.

## 4) 이번 단계의 1차 비목표

- multi-role 일반화는 이번 메모 범위 밖이다.
- service/runtime 연계는 이번 메모 범위 밖이다.
- 실제 구현 착수는 이번 메모 범위 밖이다.
- duplicate policy 재확장은 이번 메모 범위 밖이다.

## 5) 다음 최소 구현/검증 단위 제안

- missing column current behavior anchor test 1개를 먼저 추가해, header 에 있으나 row 값이 없는 경우 현재 CSV 가 빈칸으로 export 되는지 고정한다.
