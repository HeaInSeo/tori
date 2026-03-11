# Track A / A-2 Duplicate Policy Design Note v0.1
### 상태: 설계 초안 (구현 전)
### 기준선: A-1 current semantics freeze 완료 이후

## 1) 현재 duplicate current behavior 요약

- 현재 `GroupFiles` 결과 구조는 `map[int]map[string]string` 이다.
- 동일 row/column key 충돌 시, 같은 map key에 재할당되므로 **입력 순서상 나중 값이 overwrite** 된다.
- 이 동작은 A-1 Fixture D로 freeze되었고, **known as-is behavior**로만 기록되어 있다.
- 이 동작은 final intended policy가 아니다.

## 2) 왜 위험한가

- 충돌 발생 사실이 결과에서 사라진다(조용한 데이터 손실).
- pair-end에서 동일 role 중복은 일반적으로 데이터 품질 이슈로 해석될 가능성이 높다.
- 입력 순서 의존 overwrite는 재현성/디버깅 관점에서 취약하다.
- 이후 multi-role schema로 확장해도 “충돌 감지/판정” contract가 불명확하면 동일 문제가 반복된다.

## 3) 정책 옵션 비교표

| 옵션 | 동작 | 장점 | 단점 | pair-end 관점 |
|---|---|---|---|---|
| `error` | 충돌 즉시 오류 반환/row invalid 처리 | 데이터 손실 방지, contract 명확 | 초기에 실패가 늘어날 수 있음 | 동일 role 중복을 품질 문제로 강하게 다룸 |
| `keep-first` | 첫 값 유지, 이후 값 무시(충돌 기록) | 입력 안정성(초기값 고정) | 무시된 데이터 존재, 정책 직관성 낮음 | 중복 허용 신호를 줄 수 있어 품질 관리가 약해질 수 있음 |
| `keep-last` | 마지막 값 유지(현재 as-is) | 구현 단순, 기존 동작과 연속성 | 조용한 overwrite 위험, 순서 의존 | 품질 문제를 숨길 가능성이 큼 |
| `collect-list` | role에 여러 파일 누적 후 후속 판정 | 정보 손실 최소, 확장성 높음 | 현재 구조 변경 필요(`string -> []`) | 장기적으로 유리하나 A-2 초기에 범위가 커짐 |

## 4) Track A 현재 범위에서의 1차 권장 방향

- 권장: **`error` 중심 정책**을 1차 기준으로 채택.
- 이유:
1. 현재 pair-end(`R1`, `R2`)에서 동일 role 중복은 대부분 비정상 입력으로 보는 것이 자연스럽다.
2. A-1에서 확인된 overwrite 위험(조용한 손실)을 즉시 차단할 수 있다.
3. multi-role schema로 가더라도 “required one role 중복은 오류” 규칙은 재사용 가능하다.
4. `collect-list`로 바로 가는 것보다 범위가 작고, A-2 초기 단계에 적합하다.

보완:
- `error` 채택 시에도 충돌 context(row key, role key, 후보 파일명들) 보고 포맷은 함께 정의해야 한다.

## 5) A-1 freeze와의 관계

- A-1 테스트(Fixture D/E)는 **현재 구현 회귀 감지용 anchor**다.
- A-2 duplicate policy는 그 위에 올리는 **새 contract 설계**다.
- 따라서 “A-1 freeze 테스트 존재”와 “A-2 정책 변경 설계”는 충돌하지 않는다.
- 구현 단계에서는:
1. A-1 테스트를 유지하되(현재 동작 기록),
2. 정책 전환용 신규 테스트를 별도로 추가하고,
3. 전환 시점에 current-behavior 테스트의 역할을 문서에서 명확히 재분류한다.

## 6) export ordering freeze와의 분리

- Fixture E는 serialization/output의 현재 동작 기록이다.
- duplicate policy는 grouping/validation contract 영역이다.
- 두 축은 독립적으로 다뤄야 하며, A-2 duplicate 정책 수립 시 export column 정책을 같이 확정할 필요는 없다.
- 단, 추후 canonical column policy 문서에서 header/role/serialization 정합성은 별도 트랙으로 다룬다.

## 7) 열린 질문 (A-2 입력)

1. `error`를 함수 반환 에러로 처리할지, invalid row + reason code로 처리할지.
2. 충돌 리포트 최소 필드(예: row key, role key, candidates)를 무엇으로 고정할지.
3. 현재 `FilterGroups(expectedColCount)` 단계와 duplicate 판정 단계를 어떻게 분리할지.
4. pair-end에서는 unconditional error로 시작하고, 향후 role cardinality가 `many`인 타입에서 예외를 허용할지.
5. 정책 전환 시 A-1 current-behavior 테스트를 어떤 이름/위치로 유지할지(역사 기록 vs 활성 gate).

## 8) 구현 전 결론

- 결론: **지금은 duplicate policy 구현으로 바로 진행 가능**하다.
- 근거:
1. A-1 freeze로 현재 동작 anchor가 확보되어 전환 회귀를 추적할 수 있다.
2. duplicate 정책은 grouping contract 문제로, canonical column policy와 직접 결합하지 않아도 1차 구현 가능하다.
3. canonical column policy는 별도 A-2 하위 주제로 분리하는 편이 범위 통제가 쉽다.
