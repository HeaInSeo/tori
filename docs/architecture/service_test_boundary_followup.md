# Service Test Boundary Follow-up

## 현재 `service/service_test.go`의 문제

기존 `service/service_test.go`는 현재 구조 기준선과 맞지 않는 오래된 흔적을 많이 포함하고 있었다.

- 외부 helper 패키지 `api-protos/.../service`를 직접 import했다.
- 현재 `service` 책임이 아닌 `block` 생성/병합 책임까지 같이 검증했다.
- `db` 패키지의 파일 유틸(`FileExistsExact`, `SearchFilesByPattern`, `DeleteFiles*`)을 `service` 테스트에서 검증했다.
- 더 이상 존재하지 않는 오래된 타입/contract(`DataBlockServiceServerImpl`)에 의존했다.
- transport/gRPC 이전 구조의 흔적이 남아 현재 `service` 기준선을 흐렸다.

즉 이 파일은 현재 `service`의 테스트라기보다, 과거에 `service` 주변에 붙어 있던 여러 책임의 잔존물 모음에 가까웠다.

## 어떤 부분이 현재 `service` 책임과 맞고, 어떤 부분이 과거 흔적인지

현재 `service` 책임과 맞는 항목:

- `DataBlockCliService.GetDataBlock`의 timestamp 기반 의미론
- `DataBlockCliService.SaveFolders` / `SyncFolders`가 현재 app service 경로로 동작하는지
- `SaveDataBlockToTextFile`
- `LoadDataBlock`

과거 흔적 또는 다른 패키지 책임인 항목:

- `FileExistsExact`, `SearchFilesByPattern`, `DeleteFiles*`
  - 현재는 `db/fs.go` 책임
- `GenerateDataBlock`, `GenerateFileBlock`
  - 현재는 `block` 책임
- 외부 helper를 통한 proto load/save 검증
  - 현재는 `protoio` 책임
- `DataBlockServiceServerImpl`
  - 현재 구조에는 없는 과거 transport/server 흔적

## 왜 이번 턴에서 테스트를 먼저 정리해야 하는지

지금 단계에서 중요한 것은 테스트 수를 유지하는 것이 아니라, 테스트 기준선이 현재 구조를 올바르게 반영하게 만드는 것이다.

오래된 테스트가 남아 있으면 다음 문제가 생긴다.

- 현재 구조를 잘못 이해하게 만든다.
- 실제 경계 정리가 끝났는데도 “깨진 계약”처럼 보이게 만든다.
- 어떤 실패가 regression이고 어떤 실패가 obsolete test인지를 구분하기 어렵게 만든다.

따라서 이번 턴에서는 먼저 `service` 테스트를 현재 구조 기준선에 맞게 줄이고, 필요한 의미론만 남기는 편이 맞다.

## 이번 턴에서 유지한 테스트

- `GetDataBlock`의 현재 의미론
  - timestamp 없음
  - client timestamp가 과거
  - 동일 timestamp
  - client timestamp가 미래
- `SaveDataBlockToTextFile`
- `LoadDataBlock`
- `SaveFolders` + `SyncFolders`를 통한 local/in-process app service 경로의 최소 통합 확인

## 삭제/축소/이동한 테스트

삭제:

- `db/fs.go` 유틸 책임 테스트
- `block` 책임 테스트
- 오래된 `DataBlockServiceServerImpl` 기반 테스트
- 외부 helper import를 전제로 한 테스트

축소:

- `SyncFolders` 관련 테스트는 service app path 확인에 필요한 최소 통합 테스트 1개만 유지

이동:

- `GenerateFileBlock`, `GenerateDataBlock` 관련 검증은 이미 `block` 테스트에서 담당
- gRPC adapter 응답 shape는 `transport/grpc` 테스트에서 담당
- protobuf 바이너리 I/O는 `protoio`를 통해 간접 사용하거나 후속으로 별도 테스트 가능

## 이번 턴에서 하지 않는 것

- `service` production code의 대규모 리팩터
- `api-protos` 흡수
- proto source of truth 단일화
- `db` / `block` 테스트 재편 전반
- transport/gRPC 테스트 확장

이번 단계의 목적은 현재 구조 기준선에 맞지 않는 `service` 테스트 흔적을 걷어내고, 현재 `service` 책임에 맞는 최소 세트를 남기는 것이다.
