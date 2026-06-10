# Track A Primary/Index Pairing Policy Design v0.1
### 상태: Sprint 13 helper implemented
### 기준선: typed-role validation preview helper + BAM/BAI synthetic anchors

## 1. 목적

이 문서는 primary/index typed role 관계를 completeness validation과 분리해서 정의한다.

현재 대상은 BAM/BAI다.
다만 이 문서는 BAM/BAI 전용 validator 구현 문서가 아니라, typed view에서 primary/index pairing violation을 어디에 둘지 정하는 boundary 문서다.

## 2. 현재 anchor

현재 `rules.BuildTypedRoleValidationPreview`는 normalized typed role presence만 본다.

고정된 synthetic case:

- `BAM` + `BAI`: entry 없음
- `BAM` only: `missing_required_role` for `BAI`
- `BAI` only: `missing_required_role` for `BAM`

이 helper는 row-local completeness preview다.
primary/index pairing validator가 아니다.

## 3. 용어

### 3.1 Primary role

Primary role은 typed view에서 중심 artifact를 나타낸다.

BAM/BAI 예시:

- primary role: `BAM`

### 3.2 Index role

Index role은 primary role에 붙는 보조 artifact를 나타낸다.

BAM/BAI 예시:

- index role: `BAI`

### 3.3 Pairing key

Pairing key는 primary와 index가 같은 logical artifact에 속하는지 판단하는 기준이다.

현재 resolver preview에는 별도 pairing key가 없다.
현재 grouping row가 pairing boundary처럼 보이지만, 이것을 general primary/index policy로 확정하지 않는다.

## 4. 정책 결정

Sprint 12 결정:

- `unpaired_index_role`은 typed-role completeness helper에 넣지 않는다.
- unpaired index는 별도 pairing preview helper 후보로 둔다.
- current helper의 `BAI` only 결과인 `missing_required_role` for `BAM`은 유지한다.

이유:

- completeness는 row에 required typed role이 있는지만 본다.
- unpaired index는 index file이 대응 primary와 연결되지 않았다는 pairing violation이다.
- 두 signal을 같은 helper에서 만들면 missing primary와 unpaired index가 중복 또는 충돌한다.

## 5. 후보 helper

다음 구현 후보는 별도 helper다.

예상 이름:

- `BuildPrimaryIndexPairingPreview`

예상 입력:

- `ResolverPreview`
- primary role
- index role
- pairing policy

v0.1에서는 pairing policy를 별도 구조체로 만들기보다, BAM/BAI synthetic case에 필요한 최소 입력만 받을 수 있다.

예상 후보 entry:

- `unpaired_index_role`

예상 semantics:

- row에 index role이 있고 primary role이 없으면 `unpaired_index_role` 후보를 낸다.
- row에 primary role이 있고 index role이 없으면 pairing helper는 entry를 내지 않는다.
- missing index는 completeness helper의 `missing_required_role`로 유지한다.

## 6. 왜 fixture 추가를 미루는가

Shared filesystem fixture pack에 `alignment_bam_unpaired_index`를 추가하는 것은 아직 이르다.

이유:

- pairing vocabulary가 아직 코드로 닫히지 않았다.
- fixture manifest/checksum churn을 줄이려면 helper semantics를 synthetic test로 먼저 고정하는 편이 안전하다.
- 현재 shared filesystem complete BAM/BAI fixture는 existing positive anchor로 충분하다.

## 7. 다음 구현 단위

Sprint 13의 가장 작은 구현 단위:

- `rules` 계층에 `BuildPrimaryIndexPairingPreview` 후보를 추가한다.
- `BAI` only synthetic case에서 `unpaired_index_role`을 관찰한다.
- `BAM` only synthetic case에서는 pairing entry가 없어야 한다.
- 기존 `BuildTypedRoleValidationPreview` 결과는 유지한다.

## 8. Sprint 13 구현 결과

`rules.BuildPrimaryIndexPairingPreview`를 추가했다.

현재 입력:

- `ResolverPreview`
- primary role
- index role

현재 synthetic semantics:

- `BAI` only row는 `unpaired_index_role` entry를 낸다.
- `BAM` only row는 pairing entry를 내지 않는다.
- complete `BAM` + `BAI` row는 pairing entry를 내지 않는다.

이 helper는 `SchemaValidationPreview` 타입을 재사용한다.
하지만 의미상 completeness preview가 아니라 pairing preview다.

현재 `BuildTypedRoleValidationPreview`의 semantics는 유지한다.
따라서 `BAI` only row는 completeness layer에서 `missing_required_role` for `BAM`을 내고, pairing layer에서 `unpaired_index_role`을 낸다.

## 9. 다음 하위 후보

Sprint 14에서 complete BAM/BAI shared filesystem fixture에 pairing preview를 연결했다.

현재 shared filesystem smoke semantics:

- complete `BAM` + `BAI` fixture는 pairing entry가 없어야 한다.
- `alignment_bam_unpaired_index/` fixture는 `unpaired_index_role` entry를 보고해야 한다.
- `unpaired_index_role` negative coverage는 shared fixture anchored smoke로 유지한다.

다음 후보는 아직 구현하지 않는다.

- `alignment_bam_unpaired_index` fixture 추가
- multi-index policy
- CRAM/CRAI pairing generalization

Sprint 15 결정:

- `alignment_bam_unpaired_index` shared fixture는 내일 마감 범위에서 추가하지 않는다.
- unpaired index coverage는 `rules` synthetic test로 유지한다.
- shared filesystem fixture pack은 complete BAM/BAI positive smoke까지만 유지한다.
- manifest/checksum 갱신은 마감 후 별도 fixture expansion sprint에서 다룬다.

Sprint 16 follow-up:

- shared fixture pack을 수정하지 않고 existing `alignment_bam/NA12878_chr21_1x.bam.bai` filename을 사용해 BAI-only temp workdir smoke를 추가했다.
- 이 smoke는 shared fixture root에 파일을 쓰지 않는다.
- observed-key schema preview는 missing `bam`을 보고한다.
- typed-role validation preview는 missing `BAM`을 보고한다.
- typed-role pairing preview는 `unpaired_index_role`을 보고한다.
- 따라서 unpaired index negative anchor는 synthetic-only에서 shared filename anchored temp smoke로 올라갔다.

Sprint 17 follow-up:

- shared fixture pack에 `alignment_bam_unpaired_index/NA12878_chr21_1x.bam.bai`를 추가했다.
- fixture manifest와 checksum을 2026-06-10 기준으로 갱신했다.
- shared filesystem smoke는 temp-only filename anchor가 아니라 실제 `alignment_bam_unpaired_index/` fixture directory를 읽는다.
- `unpaired_index_role` negative coverage는 shared fixture anchored smoke로 승격되었다.

Sprint 18 follow-up:

- NAS 기준 `alignment_cram/` complete fixture를 shared filesystem smoke에 연결했다.
- `cram -> CRAM`, `cram_crai -> CRAI` role normalization preview를 확인한다.
- complete `CRAM` + `CRAI` fixture는 primary/index pairing entry를 내지 않는다.
- Lustre 검증은 fixture mount 준비 전까지 보류한다.

## 10. 비범위

- public diagnostics/report surface
- protobuf/API 변경
- FileBlock cell key normalization
