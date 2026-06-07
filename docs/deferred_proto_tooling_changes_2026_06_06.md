# Deferred Proto Tooling Changes
### 기준일: 2026-06-06
### 상태: proto tooling commit candidate verified; binary/report artifacts deferred

## 1. 목적

이 문서는 Track A 마감 커밋 이후 worktree에 남은 proto/tool/report 변경을 분류한다.

이 변경들은 Track A File/Data regression baseline과 직접 관련이 없으므로 Track A 커밋에는 포함하지 않았다.

## 2. 남은 변경

Proto namespace 후보:

- `protos/ichthys/v1/datablock_service.proto`
- `protos/ichthys/v1/datablock_service.pb.go`
- `protos/ichthys/v1/datablock_service_grpc.pb.go`
- `protos/ichthys/v1/syncfolders_service.proto`
- `protos/ichthys/v1/syncfolders_service.pb.go`
- `protos/ichthys/v1/syncfolders_service_grpc.pb.go`

Tooling 후보:

- `buf.yaml`

Generated/local tool artifacts:

- `bin/buf`
- `bin/govulncheck`

Report outputs:

- `reports/gosec.txt`
- `reports/lint-depguard.txt`
- `reports/lint-security-summary.txt`
- `reports/lint.txt`
- `reports/sqlclosecheck.txt`
- `reports/govulncheck-core.summary`
- `reports/govulncheck-core.txt`

## 3. 확인한 내용

Proto namespace 변경은 `.proto` package를 `ichthys`에서 `ichthys.v1`로 바꾸고, generated descriptor/service names를 이에 맞춘다.

이 변경은 protobuf/transport contract 성격이 있으므로 Track A 마감 범위에 섞지 않는다.

## 4. 검증 결과

2026-06-06 기준 확인:

```sh
env BUF_CACHE_DIR=/tmp/buf-cache ./bin/buf lint
go test ./protos/ichthys/v1
go test . -run TestExternalAPIProtosImportGuardrail
```

세 명령은 통과했다.

첫 `./bin/buf lint` 시도는 기본 cache 경로 `/home/heain/.cache/buf`가 read-only라 실패했다.
`BUF_CACHE_DIR=/tmp/buf-cache`로 지정하면 통과한다.

## 5. 2026-06-07 처리 결정

별도 proto/tooling 커밋으로 처리할 범위:

- `buf.yaml`
- proto source/generated 파일

계속 보류할 범위:

- `bin/buf`
- `bin/govulncheck`
- report output files

이 결정은 Track A regression baseline 커밋과 proto/transport contract 성격 변경을 분리하기 위한 것이다.

## 6. 다음 처리 제안

별도 proto/tooling 커밋으로 처리할 후보:

- `buf.yaml`
- proto source/generated 파일

커밋하지 않는 편이 안전한 후보:

- `bin/buf`
- `bin/govulncheck`
- report output files

Report output을 유지하려면 Phase 0 tooling/report 제출 단위로 별도 분리한다.
