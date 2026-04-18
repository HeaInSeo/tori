# Tori Proto Ownership Sprint Plan

작성일: 2026-04-18  
상태: working plan

관련 문서:

- [proto_contract_ownership.md](proto_contract_ownership.md)
- [transport_boundary.md](transport_boundary.md)
- [remote_rpc_surface_decision_note.md](remote_rpc_surface_decision_note.md)
- [proto_canonicalization_phase2_migration_order_note.md](proto_canonicalization_phase2_migration_order_note.md)
- [api-protos sunset plan](../../../api-protos/docs/API_PROTOS_SUNSET_PLAN.md)

## 목적

이 문서는 `tori` 가 자기 proto source ownership을 회수하고, 최종적으로 `api-protos` 제거까지 가기 위한 스프린트 일정을 고정한다.

현재 기준선:

- `tori` 는 app/service 와 `transport/grpc` 경계를 유지한다.
- remote RPC surface 는 `FetchDataBlock` 중심으로 좁게 유지한다.
- `SyncFolders` 는 local-only/deferred 로 유지한다.
- source ownership 정리와 generated import 교체는 같은 작업이지만 같은 스프린트 안에서 순서를 분리해 다룬다.

## Ownership Scope

이번 계획에서 `tori` 가 회수할 대상:

- `datablock/ichthys/v1/datablock_service.proto`
- `syncfolders/ichthys/v1/syncfolders_service.proto`
- legacy candidate `protos/apis.proto` 정리

이번 계획에서 하지 않는 것:

- remote surface 확대
- `SyncFolders` remote RPC 승격
- protobuf-neutral DTO 전면 전환
- mesh retry, timeout, mTLS 정책 확정

## Sprint 0

기간 목표: 기준선 고정과 신규 확산 차단

할 일:

- `tori` 안 canonical proto owner 방향을 문서상 확정
- external `api-protos` import 가 신규 경계로 퍼지지 않도록 guardrail 유지
- local source proto 위치 후보를 결정
- `protos/apis.proto` 를 legacy duplicate 로 취급하는 규칙 문서화

완료 기준:

- `tori` 저장소 안에 canonical candidate 와 legacy duplicate 의 역할 차이가 문서상 분명하다
- 신규 코드가 `api-protos` import 를 더 넓히지 않는다
- migration order 가 `transport/grpc -> protoio -> block -> service` 로 유지된다

## Sprint 1

기간 목표: source `.proto` 를 `tori` 안으로 가져오고 transport adapter 기준선 유지

현재 상태:

- canonical source 초안이 `tori/protos/ichthys/v1/` 아래에 추가되었다
- 아직 active import path 는 external `api-protos` 를 유지한다
- 이번 스프린트에서는 source ownership 만 회수하고 import 전환은 시작하지 않는다

할 일:

- `datablock_service.proto` 를 `tori` 안 canonical 위치로 이동
- `syncfolders_service.proto` 를 `tori` 안 canonical 위치로 이동
- `FetchDataBlock` contract 를 현행 narrow remote semantics 기준으로 재확인
- `transport/grpc` 가 새 local source 에서 생성된 코드를 사용하도록 준비

완료 기준:

- `tori` 저장소 안에 `DataBlockService`, `SyncFolders` canonical source `.proto` 가 존재한다
- `FetchDataBlock` 의미가 기존 문서와 충돌하지 않는다
- `transport/grpc` migration 이 가능한 local generated path 가 준비된다

Sprint 1 canonical paths:

- `tori/protos/ichthys/v1/datablock_service.proto`
- `tori/protos/ichthys/v1/syncfolders_service.proto`

Sprint 1 hold line:

- external import 제거를 시작하지 않는다
- `transport/grpc`, `protoio`, `block`, `service` import 변경은 Sprint 2로 넘긴다
- legacy `protos/apis.proto` tombstone 정리는 Sprint 3로 넘긴다

## Sprint 2

기간 목표: generated/import 경로를 가장 바깥 경계부터 교체

현재 상태:

- `datablock` 계열 cutover 가 local generated path 기준으로 완료되었다
- `transport/grpc`, `protoio`, `block`, `service` 가 모두 `tori/protos/ichthys/v1` 를 사용한다
- external `api-protos` datablock import 는 active code path 에서 제거되었다

제약:

- external generated pb 와 local generated pb 는 같은 protobuf symbol 을 등록하므로 같은 바이너리 안에 함께 링크될 수 없다
- 따라서 `transport/grpc` 만 먼저 local generated path 로 바꾸고 `service` 가 외부 pb 를 유지하는 식의 반쪽 cutover 는 성립하지 않는다
- `datablock` 계열은 boundary 별 순차 교체 원칙을 유지하되, 실제 import 전환은 같은 proto package 소비 지점을 묶은 원자적 cutover 로 실행해야 한다

할 일:

- local generated code 생성 경로를 안정화
- `datablock` proto package 소비 지점을 한 번에 전환할 cutover 묶음 정의
- 원자적 cutover 대상: `transport/grpc`, `protoio`, `block`, `service`
- cutover 전까지 active import 는 external path 유지

규칙:

- 한 번에 전면 rename 하지 않는다
- boundary 별로 테스트 가능한 작은 단위만 이동한다
- 단, 같은 proto package 의 외부/로컬 generated code 를 같은 바이너리에 섞지는 않는다
- `service` 는 과도기적 protobuf app contract 를 포함하므로 cutover 묶음 안에서도 가장 높은 위험 지점으로 취급한다

완료 기준:

- cutover 묶음과 실행 순서가 문서상 명확하다
- local generated code 생성이 안정적으로 재현된다
- 이후 원자적 cutover 에 필요한 blocker 가 식별된다
- `datablock` active code path 는 external path 대신 local generated path 를 사용한다
- 기존 narrow remote surface 의미가 유지된다

## Sprint 3

기간 목표: legacy 중복 제거와 `api-protos` 잔존 의존 정리

현재 상태:

- legacy `protos/apis.proto` 와 그 생성물은 저장소에서 제거되었다
- `datablock` active path 는 이미 local generated path 기준으로 정리되었다
- 남은 작업은 문서/과거 흔적 정리와 `syncfolders` 경로 후속 정리다

할 일:

- `protos/apis.proto` 와 새 canonical proto 관계를 최종 정리
- 더 이상 쓰이지 않는 legacy generated path 와 문서 참조를 정리
- `README` 와 architecture 문서에서 external import 를 전환 상태가 아닌 과거 상태로 격하

완료 기준:

- `tori` 기준 source `.proto` ownership 이 더 이상 흔들리지 않는다
- `protos/apis.proto` 는 제거 상태가 된다
- `tori` build/test 기준선이 `api-protos` source 에 기대지 않는다

## Sprint 4

기간 목표: `api-protos` 제거 준비 완료 선언

현재 상태:

- `tori` active code path 는 external `api-protos` generated path 를 더 이상 요구하지 않는다
- local canonical source 는 `protos/ichthys/v1` 기준으로 고정되었다
- `syncfolders` 는 source ownership만 local 로 회수했고, runtime 의미는 계속 local-only/deferred 로 유지한다

할 일:

- `tori` 저장소에서 `api-protos` 의존이 제거되었는지 최종 점검
- shared build 문서, migration note, import policy 문서를 제거 가능한 상태로 정리
- `api-protos` sunset plan exit criteria 중 `tori` 책임 항목 완료 확인

완료 기준:

- `tori` 는 자기 source `.proto` 를 직접 소유한다
- `tori` active build 와 테스트가 `api-protos` generated path 를 요구하지 않는다
- `api-protos` 제거 시 `tori` 쪽 blocker 가 없다

Sprint 4 current judgment:

- `tori` 책임 범위의 `api-protos` 제거 준비는 완료 상태다
- 남은 작업은 `tori` 내부 cutover 가 아니라 저장소 간 sunset 실행과 `NodeForge` 쪽 정리다

## Recommended Order Inside Tori

1. source `.proto` 위치 확정
2. `transport/grpc`
3. `protoio`
4. `block`
5. `service`
6. legacy duplicate 정리

## Risks

- `service` 가 protobuf 타입을 직접 반환하는 과도기 상태라 마지막 단계 churn 이 커질 수 있다
- `SyncFolders` 를 remote contract 로 오해해 migration 중에 surface 를 넓힐 위험이 있다
- legacy `protos/apis.proto` 가 source authority 로 다시 읽히면 ownership 판단이 흔들린다

## Hold Line

스프린트 진행 중에도 아래는 유지한다.

- `FetchDataBlock` 중심 narrow remote surface 유지
- `SyncFolders` 는 local-only/deferred 유지
- ownership 판단과 migration cost 분리
- `api-protos` 는 새로운 개발 장소로 사용하지 않음
