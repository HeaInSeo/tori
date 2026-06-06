# Track A Role Normalization Seam v0.1
### 상태: alignment/index probe 이후 seam 기준선
### 기준선: current tokenizer probe + alignment/index candidate + pair-end regression contract

## 1. 목적

이 문서는 current tokenizer column key와 typed-view role 사이의 normalization seam을 고정한다.

현재 구현은 `columnRules.matchParts`로 만든 문자열을 곧바로 row cell key로 사용한다.
이 값은 아직 canonical typed role이 아니다.

따라서 다음 두 개념을 분리한다.

- observed column key: current tokenizer가 발견한 key
- normalized role: typed-view schema가 기대하는 canonical role

## 2. 왜 필요한가

Pair-end FASTQ에서는 observed key와 desired role이 우연히 잘 맞는다.

예:

- observed key `R1` -> normalized role `R1`
- observed key `R2` -> normalized role `R2`

그러나 BAM/BAI probe에서는 이 일치가 깨진다.

Current probe:

- `NA12878_chr21_1x.bam` -> observed key `bam`
- `NA12878_chr21_1x.bam.bai` -> observed key `bam_bai`

Desired typed roles:

- `BAM`
- `BAI`

즉 `bam_bai`는 index sidecar를 잘 관찰했지만, canonical role 이름으로는 아직 `BAI`가 아니다.

## 3. 현재 seam 정의

v0.1에서 role normalization seam은 아래 위치에 둔다.

1. tokenizer / grouping이 source file name에서 observed row key와 observed column key를 만든다.
2. role normalization이 observed column key를 typed-view role로 변환한다.
3. role schema validation은 normalized role을 기준으로 required/cardinality/duplicate/missing을 판단한다.

현재 구현은 1번까지만 직접 수행한다.
2번과 3번은 아직 구현하지 않는다.

## 4. Alignment/index normalization 후보

BAM/BAI 후보:

| Observed key | Normalized role | 비고 |
|---|---|---|
| `bam` | `BAM` | primary alignment file |
| `bam_bai` | `BAI` | BAM index sidecar |

CRAM/CRAI 후보:

| Observed key | Normalized role | 비고 |
|---|---|---|
| `cram` | `CRAM` | primary alignment file |
| `cram_crai` | `CRAI` | CRAM index sidecar |

주의:

- 이 표는 current tokenizer probe에서 나온 key 기준이다.
- 더 좋은 recognizer가 생기면 observed key 자체가 달라질 수 있다.
- normalized role은 typed-view schema 쪽 이름으로 유지한다.

## 5. Pair-end와의 관계

Pair-end는 normalization이 필요 없다는 뜻이 아니다.

현재 pair-end에서는 observed key가 이미 `R1`, `R2`라서 normalization seam이 드러나지 않았을 뿐이다.
장기적으로는 pair-end도 아래 alias를 흡수할 수 있어야 한다.

- `read1`, `1`, `R1` -> `R1`
- `read2`, `2`, `R2` -> `R2`

다만 현재 Sprint 범위에서는 pair-end normalization 구현을 시작하지 않는다.

## 6. Duplicate / Missing 판단 위치

v0.1 판단:

- duplicate/missing 판단은 normalized role 기준이 더 맞다.
- current implementation의 `DuplicateCollisionError.RoleKey`는 아직 observed column key다.
- 따라서 current duplicate contract와 future normalized-role contract를 혼동하지 않는다.

예:

- current probe duplicate key 후보: `bam_bai`
- future normalized duplicate role: `BAI`

전환 전까지는 current key를 regression anchor로 유지하고, normalized role은 문서 기준선으로만 둔다.

## 7. 다음 구현 전 조건

role normalization 구현에 들어가기 전에 아래를 먼저 닫는다.

- normalization map을 rule spec에 둘지, recognizer-specific built-in으로 둘지
- unknown observed key를 invalid로 볼지, extra/observational로 둘지
- normalized role collision을 current duplicate error와 어떻게 연결할지
- `RoleKey` 필드가 observed key를 계속 뜻할지, normalized role을 뜻하도록 바꿀지

## 8. 비범위

- role normalization 코드 구현
- RoleSpec 구조체 도입
- schema-based validator 전환
- FileBlock protobuf schema 변경
- service/gRPC/runtime/binding 확장

## 9. 현재 결론

Alignment/index probe는 current tokenizer로 충분히 fixture를 묶을 수 있음을 보였지만, typed-view role로 승격하려면 normalization seam이 필요하다.

따라서 다음 작은 단계는 구현이 아니라 normalization map 위치와 `RoleKey` 의미 전환 여부를 결정하는 것이다.

Update note:

- 결정은 `docs/track_a_role_normalization_decision_v0.1.md`에 정리했다.
- v0.1에서는 normalization map을 rule spec 쪽에 두고, `DuplicateCollisionError.RoleKey`는 observed key 의미를 유지한다.
