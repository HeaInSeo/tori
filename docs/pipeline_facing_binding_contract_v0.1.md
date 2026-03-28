# Track A / Pipeline-Facing Binding Contract v0.1
### 상태: 아주 얇은 초안
### 기준선: A-1 current semantics freeze + duplicate v0.1 + canonical column ordering + missing/extra observational seam

## 1. 목적 / 범위

이 문서는 Track A에서 현재 닫힌 결과를 바탕으로, pipeline이 `rules`/export 결과를 어떤 최소 개념으로 볼 수 있는지 보수적으로 정리한 v0.1 초안이다.
pipeline spec 관점의 첫 소비 진입점은 `pipeline_spec_binding_ingress_note_v0.1.md`에서 별도로 더 좁게 다룬다.

현재 문서 범위는 아래에 한정한다.

- `rules.GroupFiles` / `FilterGroups` / `ExportResultsCSV` 기준의 최소 관찰 contract
- duplicate / missing / extra / bound 를 어디까지 말할 수 있는지에 대한 현재 기준선
- pipeline spec과 연결할 수 있는 가장 작은 용어 정리

현재 문서 범위 밖:

- runtime execution lifecycle
- gRPC transport / service API
- UI semantics
- warning/report payload
- strict validation contract
- multi-role generalized binding semantics

즉 v0.1은 pipeline-facing binding contract의 완성본이 아니라, Track A 결과를 pipeline 쪽 언어로 연결하기 위한 최소 연결 문서다.

## 2. 현재 파이프라인이 볼 수 있는 최소 상태 후보

이 절의 상태들은 아직 enum/schema로 확정된 public API가 아니다.
현재 구현과 문서 기준선으로부터 파이프라인 관점에서 **가장 보수적으로 관찰 가능한 상태 후보**를 정리한 것이다.

### 2.1 `duplicate`

`duplicate`는 현재 v0.1에서 가장 강하게 닫힌 상태다.

- 의미:
  - 동일 row key
  - 동일 column/role key
  - 서로 다른 source file name 복수 개가 같은 row/role에 매핑되는 경우
- 현재 보장:
  - `rules.GroupFiles`는 duplicate collision 발생 시 `error`를 반환한다.
  - 반환 타입은 `DuplicateCollisionError`다.
  - entry reason code `duplicate_role_in_row`가 사용된다.
- 현재 해석:
  - pipeline은 duplicate를 단순 missing/invalid completeness 문제가 아니라, 별도 class의 충돌로 볼 수 있다.

단, `Entries` 순서나 richer payload schema는 v0.1에서 고정하지 않는다.

### 2.2 `missing`

`missing`은 현재 strict binding failure가 아니라, export permissive semantics 기준의 관찰 상태다.

- 의미:
  - `headers`에는 존재하지만 `rowMap`에 key가 없거나 값이 `""`인 경우
- 현재 보장:
  - export surface에서는 해당 column이 빈칸으로 남는다.
  - 내부적으로는 `headers []string, rowMap map[string]string -> missing []string` 계산 경계가 존재한다.
  - `missing` 순서는 `headers` 순서를 따른다.
- 현재 해석:
  - pipeline은 missing을 "현재 row가 header 기준으로 완전하지 않을 수 있음" 정도의 관찰로만 취급해야 한다.

현재 시점에서는 missing을 곧바로 binding failure, warning, runtime block으로 일반화하지 않는다.

### 2.3 `extra`

`extra`도 현재 strict anomaly state가 아니라, export permissive semantics 기준의 관찰 상태다.

- 의미:
  - `rowMap`에는 존재하지만 `headers`에는 없는 key
- 현재 보장:
  - export surface에는 나타나지 않는다.
  - 내부적으로는 `headers []string, rowMap map[string]string -> extra []string` 계산 경계가 존재한다.
  - `extra`는 deterministic ordering으로 계산된다.
- 현재 해석:
  - pipeline은 extra를 "현재 export canonical surface 밖에 남아 있는 row-local key" 정도로만 볼 수 있다.

현재 시점에서는 extra에 warning/report/runtime 의미를 부여하지 않는다.

### 2.4 `bound`

`bound`는 가장 보수적으로 정의해야 한다.

v0.1에서 `bound`는 아래 의미로만 사용한다.

- duplicate collision이 없고
- canonical export ordering(`ruleSet.Header`) 기준으로 row가 export 가능하며
- 현재 export permissive semantics 안에서 row/header 대응이 성립하는 최소 상태

즉 `bound`는:

- runtime execution success를 뜻하지 않는다.
- strict completeness를 뜻하지 않는다.
- future binding lifecycle state machine을 뜻하지 않는다.

현재 문맥에서 `bound`는 "pipeline이 현재 rules/export 산출물을 최소한 받아들일 수 있는 대응 상태" 이상으로 해석하지 않는다.

## 3. 현재 보장되는 것 vs 아직 보장하지 않는 것

### 3.1 현재 보장되는 것

- duplicate typed error surface
  - `DuplicateCollisionError`
  - `duplicate_role_in_row`
- canonical export ordering
  - `ExportResultsCSV` data ordering은 `ruleSet.Header` 기준
- missing/extra 계산 경계 존재
  - `headers + rowMap` 조합에서 내부 primitive 수준으로 계산 가능
- export permissive semantics 유지
  - missing은 빈칸 surface
  - extra는 export surface 비노출

### 3.2 아직 보장하지 않는 것

- strict validation contract
- warning/report contract
- runtime binding lifecycle
- transport/API payload schema
- multi-role generalized binding semantics
- missing/extra의 public error 승격
- bound/missing/extra/duplicate의 enum 고정

## 4. 내부 근거와 관찰 경계

현재 missing/extra는 `headers + rowMap` 경계에서 관찰된다.
이 경계는 export surface가 만들어지기 직전의 가장 작은 정보 조합이며, 현재는 private helper 수준의 내부 primitive에 근거한다.

따라서 다음은 구분되어야 한다.

- duplicate:
  - typed error surface를 가진 상대적으로 강한 contract
- missing/extra:
  - 내부 primitive에 근거한 observational contract
  - 아직 public contract나 API schema로 승격된 것은 아님

이 구분은 pipeline-facing 문서에서 과장 없이 현재 근거만 말하기 위해 필요하다.
현재 v0.1의 이 좁은 정합성은 `TestPipelineFacingBindingProof_SingleSyntheticCase`에서 synthetic 1-case로 고정되어 있으며, observational missing/extra와 permissive export surface의 비모순성만 다룬다.

## 5. Transition Note

v0.1은 pipeline-facing contract의 완성본이 아니라, Track A 결과와 pipeline spec 사이의 첫 번째 좁은 접점 문서다.

현재 목적은 strictness를 올리는 것이 아니라:

- duplicate / missing / extra / bound 를 같은 강도로 말하지 않도록 정리하고
- 어떤 상태가 이미 닫힌 contract인지, 어떤 상태가 아직 observational 수준인지 분리하며
- 다음 pipeline spec 논의가 현재 구현 근거를 벗어나지 않도록 기준선을 제공하는 데 있다

future work는 별도 단계에서 다룬다.
특히 strict validation, warning/report surface, runtime lifecycle, API payload schema는 이 문서에서 확정하지 않는다.
