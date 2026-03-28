# Pipeline Spec Binding Anchor Boundary Note v0.1
### 상태: ultra-thin deferred boundary note
### 기준선: `pipeline_spec_binding_slot_note_v0.1.md` + `pipeline_spec_binding_locality_note_v0.1.md`

## 1. 목적

이 문서는 pipeline spec 안에서 binding concern의 anchor boundary만 설명하는 ultra-thin note다.
이 boundary note가 전제하는 locality 관점은 `pipeline_spec_binding_locality_note_v0.1.md`에 정리되어 있다.
binding contract 자체를 다시 정의하지 않으며, node-side/edge-side 설계를 시작하지 않는다.

## 2. 현재까지 닫힌 위치

- binding concern은 global이 아니다.
- pipeline spec의 static descriptive layer에 놓인다.
- node/edge 인접 locality까지는 고정되어 있다.

## 3. 아직 열지 않는 선택

현재는 binding concern이 node-side anchor인지 edge-side anchor인지 확정하지 않는다.
이는 정보 부족 때문이 아니라, 의미 과확장을 피하기 위한 의도적 deferred boundary다.

따라서 지금 단계에서는 이 concern을 node-anchored model이나 edge-anchored model로 읽지 않는다.

## 4. 해석 금지 / Non-Goals

- node schema 확정
- edge schema 확정
- field placement 확정
- API/gRPC/service payload 해석
- runtime consumption model 해석
- strict validation 결과로의 승격

## 5. Transition Note

이 note의 목적은 node/edge 인접 locality 다음 단계에서 성급한 anchor 결정을 막는 데 있다.
필요하면 이후 node-side/edge-side 소비 위치를 더 좁게 논의할 수 있지만, v0.1에서는 거기까지 확장하지 않는다.
