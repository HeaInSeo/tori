# Proto Canonicalization Phase 2A Transport gRPC Note

## Purpose

이 문서는 Phase 2의 첫 실행 단위를 `transport/grpc`로 한정하기 위한 작은 note다.
이번 단계의 목적은 generated/import 전면 교체가 아니라, gRPC adapter 경계에서 가장 작은 확인 단위를 먼저 고정하는 것이다.

## Why transport/grpc comes first

- `transport/grpc`는 adapter이므로 첫 국소 실행 단위로 가장 안전하다.
- 현재 remote RPC surface가 `FetchDataBlock` 중심의 좁은 surface라서 의미 범위를 작게 유지할 수 있다.
- adapter 경계는 `service` app contract나 더 안쪽 조합 계층을 직접 흔들지 않고도 검토할 수 있는 바깥쪽 단위다.

## Scope

- `transport/grpc` adapter 경계만 먼저 본다.
- 현재 `FetchDataBlock` 중심의 narrow remote surface와 충돌하지 않는 범위만 다룬다.
- generated/import 정리의 첫 확인 단위로서 adapter layer를 읽는 데 한정한다.

## Explicit Non-Goals

- generated/import 전면 교체
- `service` contract 변경
- `protoio` 정리
- `block` 정리
- bootstrap/lifecycle, ingress, Gateway API / GRPCRoute 문제 처리
- remote surface 확장

## Success Criteria

- `transport/grpc`를 첫 실행 단위로 두는 이유가 문서상 분명해야 한다.
- 현재 narrow remote surface와 adapter boundary가 함께 읽혀야 한다.
- `service`, `protoio`, `block`, infra concern이 이번 단계 범위 밖이라는 점이 분명해야 한다.

## Rollback Criteria

- 이 단계 정의가 `FetchDataBlock` 외 remote surface 확대처럼 읽히면 안 된다.
- adapter 경계 확인이 `service` contract 변경이나 generated 대량 정리로 오해되면 안 된다.
- 범위가 `protoio`, `block`, bootstrap/ingress`까지 번지면 이 단계 정의는 다시 줄여야 한다.

## Current Working Conclusion

- `transport/grpc`는 adapter이므로 Phase 2의 첫 국소 실행 단위로 가장 안전하다.
- 현재 `FetchDataBlock` 중심의 좁은 remote surface 덕분에 의미 범위도 작게 유지할 수 있다.
- 따라서 이번 단계는 gRPC adapter 경계에서의 작은 확인 단계로 읽고, `service`, `protoio`, `block`, bootstrap/ingress/Gateway API 문제는 다음 범위로 남겨 둔다.
