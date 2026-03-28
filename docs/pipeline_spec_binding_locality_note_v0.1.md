# Pipeline Spec Binding Locality Note v0.1
### 상태: ultra-thin locality note
### 기준선: `pipeline_spec_binding_ingress_note_v0.1.md` + `pipeline_spec_binding_slot_note_v0.1.md`

## 1. 목적

이 문서는 pipeline spec 안에서 binding concern의 locality만 설명하는 ultra-thin note다.
이 locality note가 전제하는 slot 관점은 `pipeline_spec_binding_slot_note_v0.1.md`에 정리되어 있다.
binding concern의 anchor 선택을 아직 열지 않는 경계는 `pipeline_spec_binding_anchor_boundary_note_v0.1.md`에서 별도로 더 좁게 다룬다.
binding contract 자체를 다시 정의하지 않으며, pipeline spec 전체 구조도 설계하지 않는다.

## 2. Binding Concern의 Locality

현재 binding concern은 pipeline 전체의 전역/global metadata concern으로 보지 않는다.
이 concern은 node 간 입력-출력 대응을 설명하는 node/edge 인접 locality에서 읽힌다.

즉 binding concern은:

- 개별 node의 입출력 기대
- node 사이 연결과 대응
- 정적 export 가능성

가까이에 놓이는 concern으로만 본다.

현재 이 concern은 아래를 설명하는 concern이 아니다.

- pipeline 전체 실행 상태
- 전역 runtime readiness
- 전역 transport metadata

## 3. 지금 Locality에서 읽을 수 있는 방식

- `duplicate`는 node/edge 인접 정적 충돌 signal로만 읽는다.
- `missing`/`extra`는 node/edge 인접 observational/permissive signal로만 읽는다.
- `bound`는 전역 completeness가 아니라, 해당 정적 대응 맥락에서의 최소 signal로만 읽는다.

이들을 pipeline 전체 전역 상태로 승격해서는 안 된다.

## 4. Non-Goals / 해석 금지

- global run state로의 해석
- API/gRPC/service schema로의 해석
- strict validation 결과로의 해석
- concrete field placement 확정
- multi-role generalized locality로의 확장

## 5. Transition Note

이 note의 목적은 binding concern이 pipeline spec 안에서 전역이 아니라 인접 locality에 놓인다는 점만 고정하는 데 있다.
필요하면 이후 node-side/edge-side 소비 위치를 더 좁게 논의할 수 있지만, v0.1에서는 거기까지 확장하지 않는다.
