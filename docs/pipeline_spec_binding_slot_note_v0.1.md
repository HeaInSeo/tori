# Pipeline Spec Binding Slot Note v0.1
### 상태: ultra-thin slot note
### 기준선: `pipeline_facing_binding_contract_v0.1.md` + `pipeline_spec_binding_ingress_note_v0.1.md`

## 1. 목적

이 문서는 pipeline spec 안에서 binding concern이 놓이는 자리(slot)만 설명하는 ultra-thin note다.
이 slot note의 읽기 시작점에 해당하는 ingress 관점은 `pipeline_spec_binding_ingress_note_v0.1.md`에 정리되어 있다.
binding concern이 그 자리 안에서 전역이 아닌 인접 locality에 놓이는 점은 `pipeline_spec_binding_locality_note_v0.1.md`에서 별도로 더 좁게 다룬다.
binding contract 자체를 다시 정의하지 않으며, pipeline spec 전체 구조도 설계하지 않는다.

## 2. Binding Concern의 자리

현재 binding concern은 pipeline spec의 정적 설명 층(static descriptive layer)에 속하는 것으로만 본다.
즉 node 간 입력-출력 대응, `headers + rowMap` 관찰, export 가능성 같은 정적 대응을 설명하는 자리에서만 참조된다.

현재 이 concern은 아래 자리가 아니다.

- execution lifecycle
- runtime readiness
- transport payload
- service interaction

## 3. Pipeline Spec 안에서 지금 읽을 수 있는 방식

- `duplicate`는 정적 충돌 signal로만 읽는다.
- `missing`/`extra`는 observational/permissive signal로만 읽는다.
- `bound`는 정적 완전성 보장이 아니라, permissive export 가능성 기준의 최소 대응 signal로만 읽는다.

이들은 pipeline spec 안에서도 동일 강도의 enum이나 state로 취급하지 않는다.

## 4. Non-Goals / 해석 금지

- runtime 단계로의 해석
- API/gRPC/service schema로의 해석
- strict validation 결과로의 해석
- concrete field layout/schema 확정
- multi-role generalized slot으로의 확장

## 5. Transition Note

이 note의 목적은 pipeline spec이 binding concern을 참조할 자리를 정하는 데 있다.
필요하면 이후 소비 위치나 표현 방식을 더 좁게 다룰 수 있지만, v0.1에서는 거기까지 확장하지 않는다.
