# Pipeline Binding Docs Index v0.1
### 상태: ultra-thin index/map note

## 1. 목적

이 문서는 binding 관련 문서 묶음의 index/map note다.
새 의미를 추가하지 않으며, 기존 문서의 역할과 읽기 순서만 안내한다.

## 2. 권장 읽기 순서

1. `pipeline_facing_binding_contract_v0.1.md`
   정의 측 기준 문서다.
2. `pipeline_spec_binding_ingress_note_v0.1.md`
   pipeline spec이 binding contract를 어떤 강도로 읽기 시작하는지 정리한다.
3. `pipeline_spec_binding_slot_note_v0.1.md`
   binding concern이 pipeline spec의 어느 층/자리에 놓이는지 정리한다.
4. `pipeline_spec_binding_locality_note_v0.1.md`
   그 concern이 전역이 아니라 어디 가까이에 붙는지 정리한다.
5. `pipeline_spec_binding_anchor_boundary_note_v0.1.md`
   node-side vs edge-side anchor 선택은 아직 deferred라는 점을 정리한다.

## 3. Narrow Proof Anchor

현재 좁은 정합성 proof anchor는 `TestPipelineFacingBindingProof_SingleSyntheticCase`다.
이 proof는 observational missing/extra와 permissive export surface의 비모순성만 다루며, 그 이상으로 읽지 않는다.

## 4. 범위 밖 / Deferred

- runtime/service/gRPC
- strict validation
- warning/report surface
- field/schema layout
- node-side vs edge-side concrete anchor
- multi-role generalized model

## 5. 사용 가이드

정의를 확인하려면 contract부터 읽는다.
pipeline spec 관점만 빠르게 보려면 ingress부터 시작할 수 있지만, 문서 의미를 정확히 잡으려면 위 권장 순서를 따르는 편이 보수적이다.
설명 문서 묶음 이후의 spec-like consumer 예시는 `pipeline_spec_binding_consumer_sketch_v0.1.md`에 정리되어 있다.
