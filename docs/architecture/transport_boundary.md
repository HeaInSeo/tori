# Transport Boundary

## 현재 문제

tori의 현재 구조는 transport 경계가 고정되지 않은 과도기 상태다.

- 직전 구조에서는 `service` 패키지 안에 app service 성격의 로직과 gRPC server adapter 흔적이 같이 있었다.
- 직전 구조에서는 `cmd`가 local/in-process 호출 경로이지만, transport-agnostic service 사용으로 충분히 명시되지 않았다.
- `block`은 여전히 proto/generated helper에 직접 의존하므로 core 범위와 proto 경계가 완전히 분리된 상태는 아니다.
- 저장소 안에는 예전 `protos/apis.proto` 흔적과 외부 generated import가 공존해, transport contract 기준선이 흔들린다.

현재 우선순위는 gRPC 기능 확장이 아니라, transport 변경에도 core/service가 흔들리지 않도록 경계를 먼저 고정하는 것이다.

## Current Baseline

현재 저장소 기준 첫 transport boundary pass는 이미 들어가 있다.

- `service`는 app service boundary다.
- `transport/grpc`는 service 인터페이스를 호출하는 gRPC adapter다.
- `cmd`는 현재 local/in-process 경로다.
- `protoio`는 protobuf file I/O boundary다.

즉 현재 기준선은 "transport를 분리하기 전 상태"가 아니라, "1차 분리를 끝냈지만 ownership 정리가 아직 남은 상태"로 보는 편이 맞다.

## 왜 transport-agnostic core가 필요한지

tori는 앞으로 최소 두 실행 환경을 동시에 고려해야 한다.

- 싱글 머신: CLI, local adapter, in-process 호출
- Kubernetes: Pod/Service 사이 plain gRPC 통신

그리고 장기적으로는 외부 진입과 내부 통신 정책이 바뀔 수 있다.

- 외부 진입: Gateway API + GRPCRoute 가능성
- 내부 통신: Istio/mesh 실험 결과에 따라 변경 가능

이 상황에서 core/service가 gRPC나 특정 네트워크 전제에 묶이면 다음 문제가 생긴다.

- CLI/in-process 경로와 remote gRPC 경로가 같은 도메인 동작을 중복 구현하게 된다.
- transport 교체 시 core/service 수정 범위가 커진다.
- 향후 Gateway API, mesh, in-cluster Service 모델을 시험할 때 transport 레이어가 아니라 core 레이어까지 흔들린다.

따라서 현재 단계에서는 core/service를 transport-agnostic 하게 두고, transport는 adapter로 한정하는 편이 가장 보수적이다.

## gRPC를 adapter로 남기는 이유

gRPC 자체를 제거하는 것이 목표는 아니다. gRPC는 여전히 Kubernetes 환경에서 다음 이유로 유용하다.

- Pod/Service 간 typed RPC 경로로 사용하기 쉽다.
- 향후 Gateway API + GRPCRoute와의 연결점이 자연스럽다.
- protobuf contract를 통해 명시적 경계를 유지하기 쉽다.

다만 gRPC가 core를 소유하면 안 된다.

이번 단계에서 gRPC는 다음 역할만 가진다.

- protobuf request/response를 해석한다.
- service 인터페이스를 호출한다.
- transport-specific response shape를 만든다.

즉 gRPC는 transport adapter이고, 도메인 규칙과 실행 정책의 소유자는 아니다.

## local adapter를 두는 이유

싱글 머신 환경에서는 네트워크 hop 없이 같은 service 인터페이스를 바로 호출할 수 있어야 한다.

이 경로가 필요한 이유는 다음과 같다.

- CLI나 단일 프로세스 실행에서 불필요한 gRPC bootstrap을 피할 수 있다.
- local/in-process 경로가 있으면 transport와 무관하게 service semantics를 검증할 수 있다.
- 이후 테스트나 임베디드 사용 시 “network를 거치지 않는 adapter”를 제공하기 쉽다.

현재 단계에서는 `cmd -> service interface` 직접 호출을 local/in-process 경로의 기준선으로 본다.
별도 local network protocol을 설계하지 않는다.

## 싱글 머신 / Kubernetes / Gateway API / Istio 관점의 판단

### 싱글 머신

- 가장 단순한 경로는 in-process 호출이다.
- `cmd` 또는 local adapter가 service 인터페이스를 바로 호출하면 된다.
- 이 경로에서는 gRPC가 필수가 아니다.

### Kubernetes

- 현재 단계에서는 plain gRPC over Service면 충분하다.
- transport/grpc adapter가 service 인터페이스를 호출하고, Kubernetes는 그 adapter를 네트워크로 노출한다.
- core/service는 cluster 유무를 알 필요가 없다.

### Gateway API + GRPCRoute

- 외부 진입은 장기적으로 gRPC adapter 앞단의 ingress 문제다.
- core/service는 GRPCRoute를 알 필요가 없다.
- 따라서 지금 boundary를 고정해 두면 나중에 Gateway API를 붙여도 adapter/infra 영역만 바뀐다.

### Istio 실험

- Istio는 현재 확정 대상이 아니다.
- mTLS, mesh routing, retry policy 등은 transport/infra concern이다.
- 현재 코드에는 이를 전제로 한 import, config, policy를 넣지 않는다.

결론적으로 지금은 “plain gRPC adapter가 가능하다” 수준이면 충분하고, mesh는 나중 문제다.

## 이번 단계에서 확정하는 것

- `service` 계층은 gRPC server 구현을 소유하지 않는다.
- `service` 계층은 local/in-process와 gRPC adapter가 함께 사용할 인터페이스를 제공한다.
- `transport/grpc`는 service 인터페이스를 호출하는 adapter로만 남긴다.
- `cmd`는 concrete transport가 아니라 service 인터페이스에 붙는다.

## Important Clarification

현재 `service`는 일부 protobuf 타입을 반환하는 app contract를 사용한다.

- 이 상태는 현재 문서 기준선에서 허용된 과도기적 app contract다.
- 즉 "`service`가 protobuf 타입을 다룬다"는 사실만으로 transport 경계가 다시 무너졌다고 해석하지 않는다.
- 현재 단계의 핵심은 gRPC server import와 transport lifecycle이 `service`를 오염하지 않게 하는 것이다.

다만 이것이 최종 목표는 아니다.

- 장기적으로는 protobuf-neutral 내부 DTO 또는 더 엄격한 contract 분리가 후속 선택지가 될 수 있다.
- 현재 문서는 그 목표를 미리 달성했다고 주장하지 않는다.

## Deferred Work

- 최종 proto 파일 위치와 api-protos 흡수 완료 형태
- syncfolders용 gRPC contract 최종안
- Istio/mTLS 내부 통신 정책
- Gateway API/GRPCRoute 배치 방식
- core에서 proto DTO를 완전히 제거할지 여부

특히 마지막 항목은 이번 단계의 비목표다. 현재는 “gRPC import가 service/core를 오염하지 않게 하는 것”이 우선이고, proto message를 더 중립적인 내부 DTO로 분리하는 문제는 후속 단계로 둔다.
