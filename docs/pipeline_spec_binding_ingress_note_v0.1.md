# Pipeline Spec Binding Ingress Note v0.1
### 상태: ultra-thin ingress note
### 기준선: Track A closeout + `pipeline_facing_binding_contract_v0.1.md`

## 1. 목적 / 위치

이 문서는 pipeline spec이 현재 binding contract를 처음 참조할 때 사용하는 ingress note다.
이 ingress note가 참조하는 binding contract의 정의 측 기준은 `pipeline_facing_binding_contract_v0.1.md`에 정리되어 있다.
binding concern이 pipeline spec 안에서 놓이는 위치(slot)는 `pipeline_spec_binding_slot_note_v0.1.md`에서 별도로 더 좁게 다룬다.
현재 범위는 `rules`/export 기준의 최소 binding reading에 한정한다.

범위 밖:

- pipeline execution lifecycle
- runtime / service / gRPC
- transport / API payload schema

## 2. pipeline spec이 지금 참조할 수 있는 binding 정보

### `duplicate`

pipeline spec은 `duplicate`를 현재 가장 강한 signal로만 읽어야 한다.
즉 typed error surface가 이미 존재하는 충돌 상태로는 읽을 수 있지만, 그 이상 풍부한 lifecycle state로 확장해서는 안 된다.

### `missing`

pipeline spec은 `missing`을 strict failure가 아니라 observational/permissive signal로만 읽어야 한다.
현재 의미는 header 기준 row completeness가 비어 있을 수 있다는 관찰에 한정된다.

### `extra`

pipeline spec은 `extra`를 export surface 비노출을 전제로 한 observational signal로만 읽어야 한다.
현재 의미는 canonical export surface 밖에 남아 있는 row-local key 관찰에 한정된다.

### `bound`

pipeline spec은 `bound`를 strict completeness나 runtime readiness가 아니라, 현재 permissive export 가능성 기준의 최소 대응 상태로만 읽어야 한다.

## 3. 읽기 강도 제한

`duplicate`, `missing`, `extra`, `bound`는 현재 서로 같은 강도의 state contract가 아니다.

- `duplicate`는 상대적으로 강한 contract다.
- `missing`/`extra`는 observational contract다.
- `bound`는 가장 보수적인 최소 reading이다.

따라서 pipeline spec은 이들을 동일한 enum이나 state machine처럼 소비하면 안 된다.

## 4. Non-Goals / 해석 금지

이 ingress note는 아래 해석을 허용하지 않는다.

- runtime state로의 해석
- service/gRPC/API payload schema로의 해석
- strict validation 결과로의 해석
- multi-role generalized binding model로의 확장

## 5. Transition Note

이 문서의 목적은 pipeline spec과 binding contract 사이의 첫 접점을 만드는 데 있다.
필요하면 다음 단계에서 pipeline spec 쪽 소비 위치나 형태를 더 좁게 다룰 수 있지만, v0.1 ingress note는 거기까지 확장하지 않는다.
