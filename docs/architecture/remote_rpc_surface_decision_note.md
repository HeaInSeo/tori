# Remote RPC Surface Decision Note

## Purpose

이 문서는 현재 단계에서 어떤 기능을 remote RPC surface로 열고, 어떤 기능은 local-only 또는 deferred로 둘 것인지 작은 범위로 고정하기 위한 note다.
이번 단계의 목적은 remote contract를 넓히는 것이 아니라, 현재 remote surface를 의도적으로 좁게 유지하는 기준선을 남기는 데 있다.

## Current Baseline

- `service`는 transport-agnostic app service boundary다.
- `transport/grpc`는 service를 호출하는 adapter다.
- `cmd`는 현재 local/in-process 진입 경로다.
- `protoio`는 protobuf file I/O boundary다.
- 현재 `service`의 protobuf 반환은 허용된 과도기적 app contract다.
- 현재 `transport/grpc`에서 실제로 adapter shape가 확인되는 항목은 `FetchDataBlock`이다.

## Current Decision

| Surface Item | Current Handling | Note |
|---|---|---|
| `FetchDataBlock` | current remote-allowed candidate | 현재 gRPC adapter에서 request/response translation과 테스트 기준선이 이미 존재한다. |
| `SaveFolders` | local-only for now | 현재는 CLI/in-process 경로에서 쓰는 app operation으로 둔다. |
| `SyncFolders` | deferred / local-only for now | 현재는 local path 기준으로 유지하고, remote surface로 바로 승격하지 않는다. |
| gRPC server bootstrap/lifecycle | deferred | 이는 app contract가 아니라 infra/adapter concern으로 둔다. |
| Gateway API / GRPCRoute attachment | infra concern | core/service 바깥의 attachment 문제로 두고 이번 단계에서 remote contract에 포함하지 않는다. |

## Interpretation Of The Decision

- 현재 remote RPC surface는 deliberately narrow 하다.
- 이 문서는 "`service`에 메서드가 있으니 모두 remote RPC로 열어야 한다"는 해석을 거부한다.
- `FetchDataBlock`만 현재 remote-allowed candidate로 두는 것은 adapter 기준선이 이미 확인된 범위만 remote surface로 인정하겠다는 뜻이다.
- `SaveFolders`와 `SyncFolders`는 현재 app service 계약 안에 존재하지만, 그것만으로 remote contract 승격을 의미하지는 않는다.

## Why This Narrow Surface Is Preferred

- 현재 단계의 우선순위는 transport-agnostic boundary 유지이지 remote 기능 확대가 아니다.
- 좁은 remote surface는 `service`와 `transport/grpc`의 책임 분리를 더 보수적으로 유지한다.
- `FetchDataBlock`은 이미 adapter translation과 테스트가 있어 작은 remote candidate로 다루기 적합하다.
- 반면 folder mutation/sync 계열을 바로 remote surface로 올리면 app contract, infra concern, 운영 정책이 한 번에 섞일 위험이 있다.

## Non-Goals For This Stage

- `SaveFolders`의 remote RPC 승격
- `SyncFolders`의 remote RPC 승격
- gRPC server bootstrap/lifecycle 설계 확정
- Gateway API / GRPCRoute 배치 방식 확정
- service 시그니처 변경 또는 protobuf-neutral DTO 전환

## Revisit Conditions

- `FetchDataBlock` 외 다른 operation에 대해 adapter-level request/response contract를 별도로 좁게 정의할 필요가 생길 때
- local-only operation을 remote로 열어야 하는 운영 시나리오가 문서 수준에서 구체화될 때
- gRPC bootstrap/lifecycle과 Gateway attachment를 infra concern으로 문서화한 뒤, core/service와 분리된 상태에서 surface를 다시 볼 수 있을 때

## Narrow Remote Semantics Note

- 현재 remote surface에서 `FetchDataBlock`은 read-like retrieval semantics 중심으로 본다.
- 현재 보장 대상으로 읽는 범위는 version/timestamp 계열 retrieval semantics와 충돌하지 않는 범위에 한정한다.
- mutation/sync orchestration은 아직 remote surface 의미에 포함하지 않는다.
- `SaveFolders`, `SyncFolders`의 remote execution contract는 아직 범위 밖이다.
- bootstrap/lifecycle, deployment, ingress, Gateway API / GRPCRoute attachment는 infra concern으로 범위 밖이다.

## Current Working Conclusion

현재 단계의 working conclusion은 다음이다.

- remote RPC surface는 우선 `FetchDataBlock` 중심의 좁은 surface로 유지한다.
- `SaveFolders`와 `SyncFolders`는 지금은 local-only 또는 deferred로 읽는다.
- gRPC server bootstrap/lifecycle과 Gateway API / GRPCRoute attachment는 remote contract 자체가 아니라 infra/adapter concern으로 둔다.
- 따라서 현재는 remote surface를 넓히는 것보다, 좁은 surface를 분명히 유지하는 편이 기준선에 더 맞다.
