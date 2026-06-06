# Track A Alignment + Index Typed-View Candidate v0.1
### 상태: Sprint 3 후보 선정 기준선
### 기준선: pair-end regression contract 이후, 구현 전

## 1. 목적

이 문서는 pair-end FASTQ 다음 Track A typed-view 후보를 `alignment + index`로 고정한다.

이 문서는 구현 문서가 아니다.
목적은 다음 Sprint 4에서 작은 probe를 시작하기 전에 fixture, role 후보, row key 후보, 정책 gap을 분리해 두는 것이다.

## 2. 후보로 선택한 이유

`alignment + index`를 첫 non-FASTQ typed-view 후보로 선택한다.

이유:

- BAM/BAI와 CRAM/CRAI는 파일+sidecar 관계가 명확하다.
- pair-end의 `R1/R2`처럼 2-role 구조를 가지지만, read pair가 아니라 primary file + index sidecar 관계다.
- multi-role 일반화 전체로 바로 가지 않고도 role cardinality, sidecar association, missing index를 작게 볼 수 있다.
- 현재 shared filesystem fixture pack에 이미 작은 public fixture가 있다.

## 3. 현재 fixture

Shared filesystem fixture root 기본값:

- `/mnt/genomics-test/tori-public-fixtures`

Fixture candidates:

- `alignment_bam/NA12878_chr21_1x.bam`
- `alignment_bam/NA12878_chr21_1x.bam.bai`
- `alignment_cram/NA12878_chr21_1x.cram`
- `alignment_cram/NA12878_chr21_1x.cram.crai`

Sprint 4의 첫 probe는 BAM/BAI를 우선한다.
CRAM/CRAI는 같은 category의 second fixture로 남긴다.

## 4. Role 후보

BAM/BAI probe의 최소 role 후보:

- `BAM`: required, one
- `BAI`: required, one

CRAM/CRAI probe의 최소 role 후보:

- `CRAM`: required, one
- `CRAI`: required, one

현재 단계에서는 `REFERENCE`, `KNOWN_SITES`, `TARGET_BED` 같은 shared reference role을 추가하지 않는다.

## 5. Row key 후보

현재 BAM/BAI fixture 이름 기준:

- `NA12878_chr21_1x.bam`
- `NA12878_chr21_1x.bam.bai`

후보 row key:

- basename에서 alignment extension과 index extension을 제거한 logical stem
- 예: `NA12878_chr21_1x`

주의:

- 현재 `rule.json` tokenizer 방식으로 바로 표현 가능한지 여부는 Sprint 4 probe에서 확인한다.
- 이 문서에서는 final row identity나 canonical FileBlock identity를 확정하지 않는다.

## 6. Duplicate / Missing 정책 가설

현재 최소 가설:

- 동일 row key에 `BAM`이 2개 이상이면 duplicate collision 후보다.
- 동일 row key에 `BAI`가 2개 이상이면 duplicate collision 후보다.
- `BAM`만 있고 `BAI`가 없으면 missing index 관찰 후보다.
- `BAI`만 있고 `BAM`이 없으면 orphan index 관찰 후보다.

단, missing/orphan을 public diagnostics contract로 승격하지 않는다.
Sprint 4에서는 probe 수준의 관찰과 테스트 경계만 확인한다.

## 7. 비범위

- multi-role schema 전체 구현
- DataBlock packaging 최종 정책
- binding/runtime 연결
- gRPC/service surface
- reference/shared metadata role 추가
- BAM/CRAM 파일 내용 파싱

## 8. Sprint 4 입력

Sprint 4의 가장 작은 입력은 다음이다.

- BAM/BAI fixture 1세트
- role 후보 `BAM`, `BAI`
- row key 후보 `NA12878_chr21_1x`
- rule/spec probe 또는 테스트 전용 grouping probe

성공 기준은 구현 착수 전에 다시 좁힌다.
