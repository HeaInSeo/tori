# Block Boundary Follow-up

## 현재 `block`의 외부 helper 의존 지점

현재 `block` 패키지는 외부 generated proto helper 패키지 `api-protos/.../service`에 직접 의존한다.

직접 의존 지점:

- `block/fileblock.go`
  - `ConvertMapToFileBlock`
  - `SaveProtoToFile`
- `block/merge.go`
  - `MergeFileBlocksFromData`
  - `SaveProtoToFile`

이 helper들은 모두 `api-protos` generated module 내부의 수동 코드에 위치한다. 즉 `block`은 generated protobuf 타입만 쓰는 것이 아니라, 외부 helper 구현 계층까지 직접 알고 있다.

## 왜 이게 구조 오염인지

이 구조가 문제인 이유는 다음과 같다.

- `block`은 core/domain에 가까운 조합 계층인데, 외부 helper 구현 패키지의 위치와 API를 알아야 한다.
- helper 패키지는 계약(proto) 자체가 아니라 계약 주변의 구현 편의 코드다.
- 따라서 `api-protos`의 helper 구조가 바뀌면 core에 가까운 `block`도 함께 흔들린다.
- 직전 단계에서 transport/gRPC adapter를 core 밖으로 밀어냈지만, 현재 helper 의존은 여전히 core 쪽에 외부 구현 세부사항이 남아 있는 상태다.

핵심은 "`block`이 protobuf message 타입을 쓰는 것"과 "`block`이 외부 helper 구현 패키지를 직접 import하는 것"을 구분해야 한다는 점이다. 이번 단계의 목표는 후자를 먼저 제거하는 것이다.

## 왜 `api-protos` 흡수보다 이 단계를 먼저 하는지

`api-protos` 흡수나 proto source of truth 단일화는 다음 범위를 동반한다.

- proto 파일 위치 재배치
- generation 경로 재정의
- import 경로 정리
- generated code 기준선 정리

이 작업은 범위가 크고, 지금 턴의 핵심 질문인 "`block`의 외부 helper 직접 의존 제거"보다 넓다.

반면 이번 단계는 더 작은 경계 정리다.

- `block`이 외부 helper 구현을 직접 알지 않게 만든다.
- 도메인에 가까운 변환/병합 책임을 `tori` 내부로 되돌린다.
- 입출력 책임을 별도 패키지로 분리한다.

이 단계를 먼저 하면, 나중에 `api-protos`를 흡수하더라도 `block`이 helper 구조에 덜 묶여 있어 후속 작업이 작아진다.

## 이번 턴에서 `block` 안으로 가져오는 책임

이번 단계에서 `block` 내부로 가져오는 책임은 다음이다.

- row map -> `pb.FileBlock` 변환
- 여러 `*pb.FileBlock` -> `*pb.DataBlock` 병합

이 둘은 `block`이 이미 소유하고 있는 조합 책임과 가깝다.

- `rules` 결과를 FileBlock으로 조직화하는 일
- FileBlock들을 DataBlock으로 묶는 일

따라서 이 책임은 외부 helper보다 `block` 안에 있는 편이 자연스럽다.

## 이번 턴에서 별도 패키지로 뺄 책임

이번 단계에서 별도 `protoio` 패키지로 분리하는 책임은 다음이다.

- protobuf marshal/unmarshal
- 파일 저장/로드

이 책임은 도메인 조합이 아니라 입출력이다. 따라서 `block` 내부에 계속 두는 것보다 분리된 패키지에 두는 편이 경계가 더 명확하다.

이번 단계의 `protoio`는 작은 범위로 유지한다.

- `SaveMessage`
- `LoadDataBlock`
- 필요 시 후속 단계에서 `LoadFileBlock` 등 확장

## 이번 턴에서 하지 않는 것

- `api-protos` 흡수
- proto 파일 재배치
- generated code 경로 변경
- protobuf message를 내부 DTO로 전면 치환
- `block` 패키지 대규모 분해
- transport/runtime 확장

즉, 이번 단계는 helper 직접 import 제거에만 집중한다.
