# ProtoIO Boundary Follow-up

## 현재 `protoio`의 책임

현재 `protoio`는 protobuf 파일 I/O 공통 경계다.

현재 제공 책임:

- `SaveMessage`
  - protobuf message를 binary로 serialize 해서 파일로 저장
- `LoadDataBlock`
  - binary protobuf 파일을 읽어 `DataBlock`으로 역직렬화

현재 `block`과 `service`는 이 경계를 공통으로 신뢰한다.

- `block`
  - `FileBlock`, `DataBlock` 저장에 `SaveMessage` 사용
- `service`
  - `DataBlock` 로드에 `LoadDataBlock` 사용

즉 `protoio`는 도메인 조합이나 app orchestration이 아니라, protobuf 파일 저장/로드만 담당한다.

## 왜 지금 `protoio` 테스트를 먼저 추가하는지

직전 단계에서 `block`의 외부 helper 직접 의존을 제거하면서 protobuf 파일 I/O가 `protoio`로 모였다.

이제 `service`와 `block`이 같은 I/O 경계를 공통으로 사용하므로, 이 경계를 직접 테스트로 잠그는 것이 먼저다.

이 작업을 먼저 하는 이유:

- `service`와 `block`의 간접 테스트에 의존하지 않고 `protoio` 책임을 직접 검증할 수 있다.
- 저장/로드 실패가 났을 때 도메인 문제인지 I/O 경계 문제인지 분리하기 쉬워진다.
- 후속 단계에서 proto 재배치나 `api-protos` 흡수를 하더라도, 파일 I/O 기준선이 먼저 고정된다.

## 무엇을 `protoio` 테스트에서 검증하고, 무엇은 검증하지 않는지

직접 검증:

- protobuf message를 binary 파일로 저장할 수 있는지
- 저장한 `DataBlock`을 다시 로드할 수 있는지
- `SaveMessage`가 `FileBlock` 같은 다른 protobuf message에도 공통으로 동작하는지
- 존재하지 않는 파일 또는 잘못된 바이너리 입력에서 적절히 에러가 나는지

직접 검증하지 않음:

- `FileBlock`/`DataBlock` 조합 의미론
  - `block` 책임
- timestamp 기반 `GetDataBlock` 의미론
  - `service` 책임
- gRPC request/response shape
  - `transport/grpc` 책임
- textproto 저장
  - 현재 `protoio`가 공식적으로 제공하는 경로가 아님

## `service` / `block` / `protoio` 경계 정리

- `protoio`
  - protobuf binary 파일 저장/로드
- `block`
  - row map -> `FileBlock`
  - `FileBlock[]` -> `DataBlock`
- `service`
  - app orchestration
  - `db`/`block`/`protoio`를 조합한 local/in-process service path

즉 `protoio`는 protobuf 파일 I/O boundary이고, 조합/정책을 소유하지 않는다.

## 이번 턴에서 하지 않는 것

- `protoio` API 확대
- `LoadFileBlock` 같은 새 API 추가
- text/binary 복수 형식 지원 추가
- `api-protos` 흡수
- proto source of truth 단일화
- `service`/`block` 리팩터 확대

이번 단계는 현재 `protoio` 책임을 테스트로 고정하는 작은 후속 정리다.
