# Track A Alignment + Index Probe Result v0.1
### 상태: Sprint 4 첫 probe 결과
### 기준선: alignment + index typed-view candidate v0.1

## 1. 목적

이 문서는 Sprint 4의 첫 non-FASTQ typed-view probe 결과를 기록한다.

범위는 BAM/BAI fixture 1세트에 한정한다.
이 단계는 multi-role schema 구현이 아니라, 현재 rule tokenizer로 alignment + index 관계를 어디까지 regression test로 잡을 수 있는지 확인하는 probe다.

## 2. 입력 fixture

Shared filesystem fixture:

- `alignment_bam/NA12878_chr21_1x.bam`
- `alignment_bam/NA12878_chr21_1x.bam.bai`

## 3. Probe rule

현재 tokenizer 기반 probe rule:

- `delimiter`: `["_", "."]`
- `header`: `["bam", "bam_bai"]`
- `rowRules.matchParts`: `[0, 1, 2]`
- `columnRules.matchParts`: `[3, 4]`

Observed row key:

- `NA12878_chr21_1x`

Observed columns:

- `bam`
- `bam_bai`

## 4. 결과

`TestSharedFSFixtureSmoke_AlignmentBAMIndexFixtureSpecificCurrentRuleProbe`는 다음을 검증한다.

- BAM/BAI fixture가 1개 row로 묶인다.
- `GenerateResolverPreview`가 같은 fixture 파일명에서 1개 preview row를 만든다.
- preview에서는 observed key `bam`, `bam_bai`가 normalized role `BAM`, `BAI`로 확인된다.
- schema validation preview는 fixture-specific observed-key header(`bam`, `bam_bai`) 기준으로 missing/extra/unresolved 후보가 없음을 확인한다.
- `bam` cell은 `NA12878_chr21_1x.bam`이다.
- `bam_bai` cell은 `NA12878_chr21_1x.bam.bai`이다.
- optional `roleNormalization` map을 rule에 포함해도 output cell key는 아직 `BAM`/`BAI`로 전환되지 않는다.
- `fileblock.csv`와 `.pb` 산출물은 테스트 임시 디렉토리에 생성된다.

## 5. 드러난 gap

현재 rule tokenizer는 `BAI`라는 canonical role을 직접 산출하지 못하고 `bam_bai` 같은 extension-derived key를 만든다.

따라서 다음 단계에서 구분해야 할 축은 다음이다.

- current tokenizer probe column key: `bam`, `bam_bai`
- desired typed-view role: `BAM`, `BAI`

이 gap은 role normalization 또는 typed role schema 도입 전까지는 current-rule probe로만 취급한다.

## 6. 비범위

- CRAM/CRAI probe
- role normalization 구현
- schema-based validator 구현
- missing/unpaired-index diagnostics contract
- BAM 파일 내용 파싱
- runtime/binding/service 확장

## 7. 다음 최소 작업

다음 작은 작업은 `current tokenizer key`와 `desired typed role` 사이의 normalization seam을 문서화하는 것이다.
구현은 그 seam이 닫힌 뒤에 진행한다.

Update note:

- 이 seam은 `docs/track_a_role_normalization_seam_v0.1.md`에 정리했다.
- 현재 probe의 `bam`, `bam_bai`는 observed column key이며, typed-view role `BAM`, `BAI`와 구분한다.
- fixture-specific probe와 general typed-view rule의 경계는 `docs/track_a_fixture_probe_vs_typed_view_rule_v0.1.md`에 정리했다.
- `BuildSchemaValidationPreview` 연결은 아직 `BAM/BAI` general validator가 아니라 observed-key fixture schema preview다.
