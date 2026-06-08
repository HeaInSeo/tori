# Track A Alignment Index Validation Preview Candidate v0.1
### 상태: Sprint 10 helper implemented
### 기준선: BAM/BAI fixture probe + schema validation preview helper

## 1. 목적

이 문서는 BAM/BAI probe에 schema validation preview helper를 연결한 결과와 다음 구현 단위를 정리한다.

현재 결정은 general BAM/BAI typed-view validator 구현이 아니다.
현재 결정은 fixture-specific observed-key schema preview와 typed-role validation 후보를 분리하는 것이다.

## 2. 현재 연결 결과

`TestSharedFSFixtureSmoke_AlignmentBAMIndexFixtureSpecificCurrentRuleProbe`는 다음을 확인한다.

- resolver preview는 `bam`, `bam_bai` observed roles를 만든다.
- role normalization preview는 `bam -> BAM`, `bam_bai -> BAI`를 보여준다.
- schema validation preview는 rule header `bam`, `bam_bai` 기준으로 missing/extra/unresolved 후보가 없다.

이 결과는 현재 fixture가 완전하다는 관찰이다.
하지만 이것을 곧바로 general `BAM/BAI` typed-view schema validation으로 읽지 않는다.

## 3. 왜 general validator가 아닌가

현재 `BuildSchemaValidationPreview`는 `ruleSet.Header` 기준으로 missing/extra를 계산한다.
BAM/BAI probe의 `Header`는 typed role이 아니라 observed key다.

즉 현재 preview는 다음을 검증한다.

- observed key `bam`이 있다.
- observed key `bam_bai`가 있다.
- 두 observed key 모두 normalization map으로 typed role 후보를 가진다.

현재 preview가 아직 검증하지 않는 것:

- typed role `BAM` required one
- typed role `BAI` required one
- `BAI` only row를 unpaired index로 분류
- `BAM` only row를 missing index로 분류
- CRAM/CRAI primary/index pairing

## 4. 다음 후보

다음 구현 후보는 두 가지다.

### 후보 A: fixture 추가

Shared filesystem fixture pack에 missing/unpaired-index BAM/BAI fixture를 추가한다.

예:

- `alignment_bam_missing_index/NA12878_chr21_1x.bam`
- `alignment_bam_unpaired_index/NA12878_chr21_1x.bam.bai`

장점:

- real-data regression anchor가 먼저 생긴다.
- public fixture 기준으로 missing/unpaired-index 관찰을 고정할 수 있다.

단점:

- fixture pack 변경과 manifest/checksum 갱신이 필요하다.

### 후보 B: helper 확장

`BuildSchemaValidationPreview`와 별도로 typed role 기준 helper를 추가한다.

예:

- observed key를 normalized role로 투영한다.
- typed role header 후보 `BAM`, `BAI`를 입력받아 missing/unpaired-index 후보를 계산한다.

장점:

- 코드 단위가 작다.
- fixture pack 변경 없이 synthetic test로 시작할 수 있다.

단점:

- shared filesystem regression anchor 없이 helper만 먼저 생긴다.

## 5. 결정

Sprint 9의 결정은 **후보 B를 먼저 진행**하는 것이다.

이유:

- 현재 fixture는 complete BAM/BAI pair 하나뿐이라 missing/unpaired-index을 real fixture로 바로 검증할 수 없다.
- typed role validation helper는 synthetic case로 작게 고정할 수 있다.
- fixture 추가는 helper의 vocabulary가 닫힌 뒤 진행하는 편이 manifest/checksum churn을 줄인다.

## 6. 다음 Sprint 입력

Sprint 10의 가장 작은 구현 단위:

- `rules` 계층에 typed-role schema validation preview helper를 추가한다.
- 입력은 `ResolverPreview`와 typed role header 후보로 제한한다.
- `BAM` only synthetic case는 `missing_required_role` for `BAI` 후보를 낸다.
- `BAI` only synthetic case는 `missing_required_role` for `BAM` 또는 별도 `unpaired_index_role` 후보 중 하나로 문서 기준을 먼저 따른다.

권장:

- 첫 구현은 `missing_required_role`까지만 둔다.
- `unpaired_index_role`은 primary/index pairing rule이 필요하므로 다음 하위 단계로 미룬다.

## 7. Sprint 10 구현 결과

`rules.BuildTypedRoleValidationPreview`를 추가했다.

이 helper는 기존 `BuildSchemaValidationPreview`와 다르게 `ruleSet.Header`를 사용하지 않는다.
대신 `ResolverPreview.Rows[*].RoleNormalization`의 resolved entry를 typed role set으로 투영하고, 호출자가 넘긴 required typed role 목록과 비교한다.

현재 고정된 synthetic semantics:

- complete `BAM` + `BAI` row는 validation entry가 없다.
- `BAM` only row는 `missing_required_role` for `BAI` entry를 낸다.
- observed-key preview는 같은 입력에서 missing `bam_bai`를 보고하고, typed-role preview는 missing `BAI`를 보고한다.

이 차이는 의도적이다.
observed-key schema preview와 typed-role validation preview는 같은 helper가 아니다.

## 8. 다음 하위 후보

Sprint 11에서 `BAI` only synthetic case를 관찰한다.

현재 helper 기준 결과:

- `BAI` only row는 `missing_required_role` for `BAM` entry를 낸다.
- 이 결과는 typed role completeness 관찰로는 충분하다.
- 하지만 이 결과만으로 `unpaired_index_role`을 public/report vocabulary로 승격하지 않는다.

이유:

- unpaired index 판단은 `BAI`가 어떤 `BAM`의 index role인지에 대한 pairing rule이 필요하다.
- 현재 resolver preview는 row-local typed role presence만 본다.
- primary/index pairing rule 없이 `unpaired_index_role`을 추가하면 completeness와 pairing violation이 섞인다.

Sprint 12 결정은 `docs/track_a_primary_index_pairing_policy_design_v0.1.md`에 분리했다.

결정:

- `unpaired_index_role`은 `BuildTypedRoleValidationPreview`에 넣지 않는다.
- 다음 구현 후보는 별도 pairing preview helper다.
- `BAM` only의 missing `BAI`는 completeness helper에 남긴다.
- `BAI` only의 unpaired index 후보는 pairing helper에서만 낸다.

다음 후보는 아직 구현하지 않는다.

- `BAI` only row를 단순 `missing_required_role` for `BAM`으로 둘지, `unpaired_index_role`로 분류할지 결정
- typed role duplicate collision
- fixture pack에 missing/unpaired-index alignment fixture 추가

## 9. 비범위

- public diagnostics/report surface
- protobuf/API 변경
- FileBlock cell key normalization
- CRAM/CRAI generalization
- shared filesystem fixture pack 변경
