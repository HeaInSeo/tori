# Track A Final Readiness Report
### 기준일: 2026-06-06
### 상태: final verification green

## 1. 완료 기준

이번 마감은 새 typed-view 기능 확장이 아니라 Track A regression baseline을 닫는 것을 완료 기준으로 둔다.

완료로 보는 범위:

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

## 2. 코드 anchor

핵심 코드 anchor:

- `rules.BuildSchemaValidationPreview`
- `rules.BuildTypedRoleValidationPreview`
- `rules.BuildPrimaryIndexPairingPreview`
- `block.TestSharedFSFixtureSmoke_PairedFASTQ`
- `block.TestSharedFSFixtureSmoke_AlignmentBAMIndexFixtureSpecificCurrentRuleProbe`

## 3. Shared Filesystem Anchor

현재 shared filesystem fixture root:

- `/mnt/genomics-test/tori-public-fixtures`

현재 마감 기준 shared smoke 입력:

- `paired_fastq_valid/`
- `paired_fastq_missing_role/`
- `paired_fastq_duplicate_role/`
- `alignment_bam/`

`alignment_bam/`은 complete BAM/BAI positive case로 사용한다.
2026-06-10 follow-up 기준 unpaired BAM index negative case는 `alignment_bam_unpaired_index/`로 shared fixture pack에 추가되었다.

## 4. 보류 범위

마감 제외 범위:

- `alignment_bam_unpaired_index` shared fixture 추가
- fixture manifest/checksum 변경
- CRAM/CRAI pairing generalization
- VCF/CSI, BCF/CSI, FASTA/FAI typed-view generalization
- public diagnostics/report/protobuf 변경
- FileBlock output key normalization
- service/gRPC/K8s/runtime 복구

## 5. 최종 검증

마감 검증 명령:

```sh
make test-core
make test-shared-fs-fixtures
```

2026-06-06 기준 두 명령은 green이다.

## 6. 잔여 리스크

잔여 리스크:

- `unpaired_index_role`은 synthetic test anchor로만 검증된다.
- current BAM/BAI probe rule은 fixture-specific observed-key rule이며 general typed-view rule이 아니다.
- shared filesystem fixture pack은 NAS 기준으로 확인되었고 Lustre 적용은 같은 shared filesystem contract로 재검증해야 한다.
- dirty worktree에는 Track A 직접 범위 밖 변경이 함께 존재한다.

## 7. 다음 단계

마감 이후 첫 후보:

- `alignment_bam_unpaired_index` fixture 추가 여부 결정
- 추가 시 manifest/checksum 갱신
- unpaired index shared smoke 연결
- 그 뒤 CRAM/CRAI 확장 여부 판단

2026-06-10 follow-up:

- `alignment_bam_unpaired_index` fixture를 추가했다.
- NAS fixture manifest/checksum을 갱신했다.
- unpaired index shared smoke는 실제 fixture directory 기반으로 승격되었다.

2026-06-10 CRAM/CRAI follow-up:

- NAS 기준 `alignment_cram/` complete fixture smoke를 추가했다.
- complete CRAM/CRAI는 primary/index pairing entry를 내지 않는 것으로 고정했다.
- Lustre 검증은 fixture mount 준비 전까지 후순위 보류다.
