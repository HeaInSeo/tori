# Track A Sidecar Association Policy Design v0.1
### 상태: Sprint 13 helper implemented
### 기준선: typed-role validation preview helper + BAM/BAI synthetic anchors

## 1. 목적

이 문서는 primary/sidecar typed role 관계를 completeness validation과 분리해서 정의한다.

현재 대상은 BAM/BAI다.
다만 이 문서는 BAM/BAI 전용 validator 구현 문서가 아니라, typed view에서 sidecar association violation을 어디에 둘지 정하는 boundary 문서다.

## 2. 현재 anchor

현재 `rules.BuildTypedRoleValidationPreview`는 normalized typed role presence만 본다.

고정된 synthetic case:

- `BAM` + `BAI`: entry 없음
- `BAM` only: `missing_required_role` for `BAI`
- `BAI` only: `missing_required_role` for `BAM`

이 helper는 row-local completeness preview다.
sidecar association validator가 아니다.

## 3. 용어

### 3.1 Primary role

Primary role은 typed view에서 중심 artifact를 나타낸다.

BAM/BAI 예시:

- primary role: `BAM`

### 3.2 Sidecar role

Sidecar role은 primary role에 붙는 보조 artifact를 나타낸다.

BAM/BAI 예시:

- sidecar role: `BAI`

### 3.3 Association key

Association key는 primary와 sidecar가 같은 logical artifact에 속하는지 판단하는 기준이다.

현재 resolver preview에는 별도 association key가 없다.
현재 grouping row가 association boundary처럼 보이지만, 이것을 general sidecar policy로 확정하지 않는다.

## 4. 정책 결정

Sprint 12 결정:

- `orphan_sidecar_role`은 typed-role completeness helper에 넣지 않는다.
- orphan sidecar는 별도 association preview helper 후보로 둔다.
- current helper의 `BAI` only 결과인 `missing_required_role` for `BAM`은 유지한다.

이유:

- completeness는 row에 required typed role이 있는지만 본다.
- orphan sidecar는 sidecar가 대응 primary와 연결되지 않았다는 association violation이다.
- 두 signal을 같은 helper에서 만들면 missing primary와 orphan sidecar가 중복 또는 충돌한다.

## 5. 후보 helper

다음 구현 후보는 별도 helper다.

예상 이름:

- `BuildTypedRoleAssociationPreview`

예상 입력:

- `ResolverPreview`
- primary role
- sidecar role
- association policy

v0.1에서는 association policy를 별도 구조체로 만들기보다, BAM/BAI synthetic case에 필요한 최소 입력만 받을 수 있다.

예상 후보 entry:

- `orphan_sidecar_role`

예상 semantics:

- row에 sidecar role이 있고 primary role이 없으면 `orphan_sidecar_role` 후보를 낸다.
- row에 primary role이 있고 sidecar role이 없으면 association helper는 entry를 내지 않는다.
- missing sidecar는 completeness helper의 `missing_required_role`로 유지한다.

## 6. 왜 fixture 추가를 미루는가

Shared filesystem fixture pack에 `alignment_bam_orphan_index`를 추가하는 것은 아직 이르다.

이유:

- association vocabulary가 아직 코드로 닫히지 않았다.
- fixture manifest/checksum churn을 줄이려면 helper semantics를 synthetic test로 먼저 고정하는 편이 안전하다.
- 현재 shared filesystem complete BAM/BAI fixture는 existing positive anchor로 충분하다.

## 7. 다음 구현 단위

Sprint 13의 가장 작은 구현 단위:

- `rules` 계층에 `BuildTypedRoleAssociationPreview` 후보를 추가한다.
- `BAI` only synthetic case에서 `orphan_sidecar_role`을 관찰한다.
- `BAM` only synthetic case에서는 association entry가 없어야 한다.
- 기존 `BuildTypedRoleValidationPreview` 결과는 유지한다.

## 8. Sprint 13 구현 결과

`rules.BuildTypedRoleAssociationPreview`를 추가했다.

현재 입력:

- `ResolverPreview`
- primary role
- sidecar role

현재 synthetic semantics:

- `BAI` only row는 `orphan_sidecar_role` entry를 낸다.
- `BAM` only row는 association entry를 내지 않는다.
- complete `BAM` + `BAI` row는 association entry를 내지 않는다.

이 helper는 `SchemaValidationPreview` 타입을 재사용한다.
하지만 의미상 completeness preview가 아니라 association preview다.

현재 `BuildTypedRoleValidationPreview`의 semantics는 유지한다.
따라서 `BAI` only row는 completeness layer에서 `missing_required_role` for `BAM`을 내고, association layer에서 `orphan_sidecar_role`을 낸다.

## 9. 다음 하위 후보

Sprint 14에서 complete BAM/BAI shared filesystem fixture에 association preview를 연결했다.

현재 shared filesystem smoke semantics:

- complete `BAM` + `BAI` fixture는 association entry가 없어야 한다.
- orphan sidecar negative case는 아직 shared fixture에 없다.
- `orphan_sidecar_role` negative coverage는 synthetic test로 유지한다.

다음 후보는 아직 구현하지 않는다.

- `alignment_bam_orphan_index` fixture 추가
- multi-sidecar policy
- CRAM/CRAI association generalization

Sprint 15 결정:

- `alignment_bam_orphan_index` shared fixture는 내일 마감 범위에서 추가하지 않는다.
- orphan coverage는 `rules` synthetic test로 유지한다.
- shared filesystem fixture pack은 complete BAM/BAI positive smoke까지만 유지한다.
- manifest/checksum 갱신은 마감 후 별도 fixture expansion sprint에서 다룬다.

Sprint 16 follow-up:

- shared fixture pack을 수정하지 않고 existing `alignment_bam/NA12878_chr21_1x.bam.bai` filename을 사용해 BAI-only temp workdir smoke를 추가했다.
- 이 smoke는 shared fixture root에 파일을 쓰지 않는다.
- observed-key schema preview는 missing `bam`을 보고한다.
- typed-role validation preview는 missing `BAM`을 보고한다.
- typed-role association preview는 `orphan_sidecar_role`을 보고한다.
- 따라서 orphan sidecar negative anchor는 synthetic-only에서 shared filename anchored temp smoke로 올라갔다.

## 10. 비범위

- public diagnostics/report surface
- protobuf/API 변경
- shared filesystem fixture pack 변경
- CRAM/CRAI generalization
- FileBlock cell key normalization
