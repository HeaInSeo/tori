# Proto Canonicalization Phase 2 Migration Order Note

## Purpose

이 문서는 Phase 2에서 generated/import dependency를 어떤 순서로 국소 정리할지 고정하기 위한 짧은 note다.
이번 문서의 목적은 migration order를 보수적으로 정하는 것이지, source ownership 선언이나 실제 import rewrite를 시작하는 것이 아니다.

## Current Baseline

- source ownership과 generated/import migration은 다른 문제다.
- `service`는 transport-agnostic app service boundary다.
- `transport/grpc`는 adapter다.
- `protoio`는 protobuf file I/O boundary다.
- `service`의 protobuf 반환은 현재 허용된 과도기적 app contract다.
- remote RPC surface는 deliberately narrow 하다.
- external `api-protos` 의존은 기존 boundary 안에서만 유지한다.

## Why Order Matters

- generated/import 경로를 한 번에 바꾸면 boundary 충돌과 churn이 같이 커진다.
- 바깥쪽 adapter부터 안쪽 app contract 쪽으로 좁혀 들어가야 영향 범위를 더 작게 통제할 수 있다.
- 특히 `service`는 현재 protobuf 반환을 포함하므로 가장 마지막에 다루는 편이 보수적이다.

## Preferred Local Migration Order

Phase 2의 preferred local migration order는 아래와 같다.

1. `transport/grpc`
2. `protoio`
3. `block`
4. `service`

## Per-Step Rules

- `transport/grpc`
  - adapter이므로 가장 먼저 다뤄도 core/service boundary를 덜 흔든다.
- `protoio`
  - file I/O boundary라 transport 다음의 좁은 단위로 정리하기 쉽다.
- `block`
  - pb 타입 조합 책임이 있으므로 `protoio` 다음에 국소 정리한다.
- `service`
  - 과도기적 protobuf 반환 app contract를 포함하므로 가장 마지막에 다룬다.

## What Phase 2 Still Does Not Do

- 전면 import rename
- generated code 대량 재생성
- remote surface 확장
- `service` contract 재설계
- bootstrap/lifecycle, ingress, Gateway API / GRPCRoute 문제 처리

## Entry Conditions

- Phase 1 note 기준으로 local source candidate를 working canonical candidate로 다룰 준비가 되어 있어야 한다.
- ownership과 migration order가 문서상 분리되어 있어야 한다.
- external dependency 확산 방지 가드레일이 유지되어 있어야 한다.

## Exit Conditions

- 국소 정리 순서가 문서상 흔들리지 않아야 한다.
- 각 단계가 boundary conflict를 최소화하는 순서로 읽혀야 한다.
- `service`를 마지막 단계로 남기는 이유가 계속 유지되어야 한다.

## Current Working Conclusion

- Phase 2는 source ownership 선언 단계가 아니라 generated/import 경로를 국소 정리하는 단계다.
- preferred order는 `transport/grpc -> protoio -> block -> service` 다.
- 이 순서는 adapter와 file I/O boundary를 먼저 다루고, 과도기적 protobuf app contract를 가진 `service`를 마지막에 남겨 현재 기준선과 가장 덜 충돌한다.
