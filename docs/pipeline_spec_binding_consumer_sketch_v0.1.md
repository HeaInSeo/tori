# Pipeline Spec Binding Consumer Sketch v0.1
### 상태: ultra-thin consumer sketch

## 1. 목적

이 문서는 pipeline spec 본문에 들어갈 수 있는 binding consumer sketch의 ultra-thin 초안이다.
binding contract 자체를 다시 정의하지 않으며, schema/field/runtime/API 설계를 시작하지 않는다.

## 2. Consumer Sketch

본 스펙은 binding concern을 정적 설명 층의 node/edge 인접 관찰 정보로만 읽는다.
`duplicate`, `missing`, `extra`, `bound`는 동일 강도의 enum/state로 읽지 않으며, runtime readiness, execution lifecycle, transport payload를 뜻하는 것으로 해석하지 않는다.
현재 binding concern의 concrete node-side/edge-side anchor 선택도 이 스케치 범위 밖에 둔다.

## 3. 해석 금지

- runtime state 아님
- service/gRPC/API payload 아님
- strict validation result 아님
- concrete node/edge anchor 아님

## 4. Transition Note

이 sketch의 목적은 pipeline spec 문장 톤을 최소 수준으로 고정하는 데 있다.
필요하면 이후 더 구체화할 수 있지만, v0.1에서는 거기까지 확장하지 않는다.
본 sketch의 현재 시험 삽입 위치는 `tori_living_technical_draft_v0.2.md`의 `3.2.1 Binding Reading v0.1`이다.
