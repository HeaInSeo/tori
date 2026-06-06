# Track A Pair-End Regression Contract v0.1
### 상태: Sprint 2 regression 기준선
### 기준선: A-1 current semantics freeze + A-2 duplicate contract + shared filesystem fixture split

## 1. 목적

이 문서는 shared filesystem fixture pack에 있는 pair-end FASTQ fixture를 Track A regression contract로 승격하기 위한 최소 기준을 고정한다.

이 contract는 pair-end 전용 최종 설계가 아니다.
현재 pair-end resolver 경로를 이후 multi-role typed-view 일반화 전에 흔들리지 않게 잡아 두는 regression anchor다.

## 2. 입력 fixture

현재 shared filesystem fixture root 기본값:

- `/mnt/genomics-test/tori-public-fixtures`

현재 contract 입력:

- `paired_fastq_valid/`
- `paired_fastq_missing_role/`
- `paired_fastq_duplicate_role/`

`paired_fastq_invalid/`는 legacy mixed invalid fixture로 남기되, Sprint 2 regression contract의 직접 입력으로 보지 않는다.

## 3. 검증 contract

### 3.1 Valid paired FASTQ

`paired_fastq_valid/`는 현재 pair-end happy path다.

최소 검증 항목:

- read-only resolver preview는 source file 4개, row 2개, observed role 4개를 가진다.
- 현재 `rule.json`에는 `roleNormalization`이 없으므로 `R1/R2` observed roles는 preview에서 unresolved로 남는다.
- schema validation preview는 missing/extra 후보 없이 `R1/R2` unresolved observation만 가진다.
- read-only resolver preview는 `fileblock.csv`, protobuf, invalid report를 생성하지 않는다.
- `rule.json`의 header는 `R1`, `R2`다.
- `GenerateFileBlock` 결과는 valid row 2개를 가진다.
- `fileblock.csv`가 생성된다.
- `<workdir basename>files.pb`가 생성된다.
- 산출물은 테스트 임시 디렉토리에 생성되며 shared filesystem fixture root에 다시 쓰지 않는다.

### 3.2 Missing role

`paired_fastq_missing_role/`는 duplicate collision 없이 missing R2 invalid-row path를 관찰한다.

최소 검증 항목:

- read-only resolver preview는 source file 1개, row 1개, observed role 1개를 가진다.
- 현재 `rule.json`에는 `roleNormalization`이 없으므로 `R1` observed role은 preview에서 unresolved로 남는다.
- schema validation preview는 `R1` unresolved observation과 `R2` `missing_required_role` 후보를 분리한다.
- read-only resolver preview는 invalid report를 생성하지 않는다.
- `GenerateFileBlock` 호출은 error 없이 끝난다.
- valid row는 0개다.
- `invalid_files_*.txt`가 1개 생성된다.
- 이 상태를 strict validation/public warning/report contract로 승격하지 않는다.

### 3.3 Duplicate role

`paired_fastq_duplicate_role/`는 duplicate R1 collision path를 관찰한다.

최소 검증 항목:

- read-only resolver preview도 `DuplicateCollisionError`를 반환한다.
- preview error의 reason code는 `duplicate_role_in_row`이고 role key는 `R1`이다.
- read-only resolver preview는 `fileblock.csv`, protobuf, invalid report를 생성하지 않는다.
- `GenerateFileBlock` 호출은 error를 반환한다.
- 반환 error는 `errors.As(..., *rules.DuplicateCollisionError)`로 확인 가능하다.
- 첫 contract level에서 필요한 reason code는 `duplicate_role_in_row`다.
- role key는 `R1`이다.

## 4. Synthetic Fixture와 Shared Filesystem Fixture의 역할 분리

Synthetic A-1 fixture:

- 작고 빠른 current semantics anchor다.
- 기본 `make test-core` 안에 포함된다.
- 파일명/tokenization/grouping/export 같은 단위 의미론을 고정한다.

Shared filesystem fixture:

- 실제 유전체 파일명과 shared filesystem 접근 조건을 포함하는 real-data regression anchor다.
- opt-in 명령인 `make test-shared-fs-fixtures`로 실행한다.
- NAS, Lustre, NFS 등 shared filesystem mount에서 같은 contract를 검증해야 한다.

## 5. 비범위

- pair-end 외 typed-view rule 구현
- multi-role schema 일반화
- output protobuf binary schema 변경
- service/gRPC/runtime/K8s 연계
- missing/extra의 public diagnostics contract 승격

## 6. 현재 상태

2026-06-02 기준 Sprint 2 contract는 `TestSharedFSFixtureSmoke_PairedFASTQ`와 `make test-shared-fs-fixtures`로 검증된다.
