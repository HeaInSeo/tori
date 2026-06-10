# Track A Tomorrow Scope Freeze v0.1
### 상태: 2026-06-06 마감 기준
### 기준선: Phase 0 baseline + Track A shared filesystem regression

## 1. 목적

이 문서는 2026-06-06 마감 기준으로 Track A에서 완료로 볼 범위와 보류할 범위를 고정한다.

목표는 새 기능 확장이 아니라 regression baseline을 안정적으로 닫는 것이다.

## 2. 완료 범위

마감 포함 범위:

- Phase 0 core test baseline
- shared filesystem fixture smoke gate
- pair-end valid/missing/duplicate regression anchor
- duplicate collision typed error contract
- read-only resolver preview boundary
- observed-key schema validation preview helper
- typed-role validation preview helper
- BAM/BAI role normalization preview
- typed-role pairing preview helper
- complete BAM/BAI shared fixture positive pairing smoke

## 3. 보류 범위

마감 제외 범위:

- `alignment_bam_unpaired_index` shared fixture 추가
- fixture manifest/checksum 변경
- CRAM/CRAI pairing generalization
- VCF/CSI, BCF/CSI, FASTA/FAI typed-view generalization
- public diagnostics/report/protobuf 변경
- FileBlock output key normalization
- service/gRPC/K8s/runtime 복구

## 4. 마감 검증 명령

최종 검증 명령:

```sh
make test-core
make test-shared-fs-fixtures
```

두 명령이 green이면 2026-06-06 마감 기준 Track A regression baseline은 닫힌 것으로 본다.

## 5. 남은 리스크

남은 리스크:

- shared filesystem fixture pack에는 unpaired BAM index negative case가 없다.
- `unpaired_index_role`은 현재 synthetic test anchor로만 검증된다.
- current BAM/BAI fixture rule은 fixture-specific observed-key rule이며 general typed-view rule이 아니다.
- dirty worktree에는 Track A 직접 범위 밖 변경이 함께 존재한다.

## 6. 다음 단계

마감 이후 첫 후보:

- `alignment_bam_unpaired_index` fixture 추가 여부 재검토
- manifest/checksum 갱신 절차 실행
- pairing preview를 unpaired index shared fixture smoke에 연결
- 그 다음 CRAM/CRAI로 확장 여부 판단

2026-06-10 follow-up:

- `alignment_bam_unpaired_index` fixture를 추가했다.
- manifest/checksum을 갱신했다.
- pairing preview를 실제 unpaired index shared fixture smoke에 연결했다.
