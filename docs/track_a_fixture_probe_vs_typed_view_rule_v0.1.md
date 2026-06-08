# Track A Fixture Probe vs Typed-View Rule v0.1
### 상태: scope boundary note
### 기준선: alignment/index probe + role normalization decision

## 1. 목적

이 문서는 특정 shared filesystem fixture를 이용한 probe와 일반 typed-view rule을 구분한다.

현재 `alignment_bam/NA12878_chr21_1x.bam*` probe는 특정 fixture를 이용한 regression probe다.
이 probe를 곧바로 일반 BAM/BAI typed-view rule로 해석하지 않는다.

## 2. Fixture-specific probe

Fixture-specific probe는 다음을 확인한다.

- 현재 fixture root에 접근 가능한가
- 현재 resolver가 특정 파일명 집합을 어떻게 tokenization/grouping 하는가
- 현재 rule tokenizer로 최소 row/column 구조를 만들 수 있는가
- 기존 pair-end regression을 깨지 않고 non-FASTQ fixture를 관찰할 수 있는가

현재 BAM/BAI probe가 고정하는 것:

- 입력 fixture: `alignment_bam/NA12878_chr21_1x.bam`, `alignment_bam/NA12878_chr21_1x.bam.bai`
- observed row key 후보: `NA12878_chr21_1x`
- observed column key: `bam`, `bam_bai`
- optional `roleNormalization` map이 있어도 fixture probe output cell key는 observed key로 유지됨
- generated CSV / protobuf 산출물은 테스트 임시 디렉토리에 생성됨

현재 BAM/BAI probe가 고정하지 않는 것:

- 모든 BAM/BAI 파일명의 parsing 규칙
- canonical row identity 정책
- general BAM/BAI typed-view schema
- role normalization 구현
- missing/unpaired index diagnostics contract

## 3. Typed-view rule

Typed-view rule은 fixture 하나가 아니라 view class의 의미를 다룬다.

Alignment/index typed-view rule 후보가 다뤄야 할 것:

- role schema: `BAM`, `BAI`
- role cardinality: required one
- row identity 후보: alignment stem
- primary/index pairing: `.bam.bai`는 `.bam`의 index
- duplicate/missing/unpaired-index 판단 위치
- role normalization: observed key에서 typed role로의 변환

이것은 아직 구현되지 않았다.

## 4. 왜 분리하는가

특정 fixture 이름은 probe에는 유용하지만, general rule이 되기에는 위험하다.

예:

- `NA12878_chr21_1x`는 fixture row key이지 모든 alignment row key의 형식이 아니다.
- `bam_bai`는 current tokenizer가 만든 observed key이지 canonical role이 아니다.
- ``.bam.bai` index file는 BAM에는 자연스럽지만, CRAM/CRAI나 다른 index naming에는 별도 규칙이 필요할 수 있다.

따라서 fixture probe는 "관찰 결과"로 유지하고, typed-view rule은 별도 설계 단위로 다룬다.

## 5. 테스트 명명 원칙

Shared filesystem fixture test 이름은 가능하면 아래를 드러내야 한다.

- `FixtureProbe`: 특정 fixture에 기대는 검증
- `CurrentRule`: 현재 tokenizer/rule 기반 검증
- typed-view rule이 아님

따라서 BAM/BAI 현재 테스트는 general rule test가 아니라 fixture probe test다.

## 6. 다음 작은 구현 전 조건

다음 구현이 role normalization helper라면, fixture probe와 분리해서 테스트한다.

권장 순서:

1. pure helper test로 `bam -> BAM`, `bam_bai -> BAI`를 검증한다.
2. shared filesystem fixture probe는 observed key regression으로 유지한다.
3. FileBlock cell key 전환은 별도 단계에서 다룬다.

Update note:

- `rules.BuildRoleNormalizationPreview`를 추가해 row-level observed key와 normalized role을 함께 볼 수 있게 했다.
- `rules.BuildRowPreview`를 추가해 observed roles와 normalization preview를 한 row 단위로 묶어 볼 수 있게 했다.
- `rules.BuildResolverPreview`를 추가해 grouped rows 전체를 deterministic row ordering으로 preview할 수 있게 했다.
- `rules.GenerateResolverPreview`를 추가해 `fileNames + RuleSet`에서 current grouping 기반 preview를 만들 수 있게 했다.
- duplicate collision은 기존 typed error로 유지한다.
- shared filesystem BAM/BAI fixture-specific probe도 `GenerateResolverPreview`를 사용해 실제 fixture 파일명에서 preview row와 normalized role을 확인한다.
- `ResolverPreview` summary는 source file count, row count, observed role count, unresolved role count만 다룬다.
- valid/invalid row count는 schema validation/report surface가 아니므로 아직 다루지 않는다.
- `rules.GenerateResolverPreviewFromDir`를 추가해 `rule.json` 로딩과 file listing까지 포함하는 read-only preview path를 제공한다.
- 이 directory preview path는 CSV/protobuf/invalid report를 쓰지 않는다.
- shared filesystem pair-end valid fixture도 `GenerateResolverPreviewFromDir`를 사용해 `R1/R2` observed role preview를 고정한다.
- shared filesystem BAM/BAI fixture-specific probe는 `GenerateResolverPreviewFromDir`를 사용해 temp workdir 기준 read-only preview를 먼저 검증한다.
- 이 helper는 row map을 rewrite하지 않는다.
- shared filesystem fixture probe는 계속 observed key regression으로 유지한다.

## 7. 비범위

- general typed-view rule 구현
- schema validator 구현
- FileBlock output key 전환
- CRAM/CRAI generalization
- runtime/binding/service 확장
