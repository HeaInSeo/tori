# Track A Schema Validation Boundary Design v0.1
### 상태: Sprint 6 design boundary
### 기준선: resolver preview boundary + role normalization decision + pair-end regression contract

## 1. 목적

이 문서는 `ResolverPreview`에서 관찰한 summary와 향후 schema validation/report contract를 분리한다.

현재 목표는 validator 구현이 아니다.
현재 목표는 어떤 signal이 관찰값이고, 어떤 signal이 즉시 오류이며, 어떤 signal이 다음 validator 후보인지 경계를 고정하는 것이다.

## 2. 현재 입력 signal

현재 Track A에서 확인된 signal은 다음과 같다.

| Signal | 현재 source | 현재 의미 | 현재 강도 |
|---|---|---|---|
| `DuplicateCollisionError` | `GroupFiles` / `GenerateResolverPreviewFromDir` | 같은 row/role key 충돌 | typed error |
| `UnresolvedRoleCount` | `ResolverPreview` | observed key가 `roleNormalization`에 없음 | observation |
| missing header role | `headers + rowMap` helper boundary | header에는 있으나 row 값이 없음 | observation / current invalid-row path 후보 |
| extra row role | `headers + rowMap` helper boundary | header 밖 row-local key 존재 | observation |

이 네 signal은 같은 강도의 enum/state가 아니다.

## 3. 현재 결정

### 3.1 Duplicate

Duplicate는 현재 이미 typed error다.

- `DuplicateCollisionError`는 preview 단계에서도 보존된다.
- reason code `duplicate_role_in_row`는 v0.1 contract다.
- `RoleKey`는 normalized role이 아니라 observed key다.

향후 schema validator가 생겨도 duplicate는 별도 collision class로 유지한다.
missing/extra/unresolved와 같은 completeness bucket에 섞지 않는다.

### 3.2 Unresolved role

`UnresolvedRoleCount`는 strict validation 결과가 아니다.

현재 의미:

- `roleNormalization` map이 없거나,
- map에 observed key가 없어서,
- normalized role을 계산하지 못했다는 preview summary다.

따라서 pair-end fixture에서 `R1/R2`가 unresolved인 것은 현재 오류가 아니다.
현재 pair-end fixture의 `rule.json`에는 normalization map이 없기 때문이다.

### 3.3 Missing role

Missing은 현재 두 층으로 분리된다.

- preview layer: row가 어떤 observed role을 갖는지 관찰한다.
- FileBlock generation layer: 현재 `FilterGroupsByHeaders` 경로에서 valid row 제외와 invalid report 생성이 발생할 수 있다.

즉 missing-role preview가 invalid report를 쓰지 않는 것은 의도된 boundary다.
invalid report는 current FileBlock generation behavior로 남긴다.

### 3.4 Extra role

Extra는 현재 export surface 밖에 남는 observed key다.

현재 CSV/FileBlock export는 header-defined surface를 우선한다.
extra role은 향후 report 후보지만, 현재 strict error로 승격하지 않는다.

## 4. Validation 후보 vocabulary

향후 schema validation을 구현한다면 최소 후보 vocabulary는 다음 정도로 제한한다.

| Candidate | 적용 대상 | 설명 | v0.1 구현 여부 |
|---|---|---|---|
| `duplicate_role_in_row` | duplicate | 같은 row의 같은 observed/normalized role 충돌 | 이미 구현 |
| `unresolved_observed_role` | normalization | observed key가 typed role로 해석되지 않음 | 미구현 |
| `missing_required_role` | schema completeness | required role이 row에 없음 | preview helper 구현 |
| `extra_observed_role` | schema completeness | schema 밖 observed role 존재 | preview helper 구현 |
| `unpaired_index_role` | typed-view pairing | index만 있고 primary role 없음 | pairing preview helper 구현 |

이 vocabulary는 public API schema가 아니다.
다음 구현 단위를 고르기 위한 internal design vocabulary다.

## 5. Pair-end와 BAM/BAI에 대한 해석

### 5.1 Pair-end valid

현재 pair-end valid fixture:

- observed roles: `R1`, `R2`
- `roleNormalization`: 없음
- preview: unresolved로 남음
- FileBlock: valid rows 생성

해석:

- unresolved는 오류가 아니다.
- pair-end alias normalization을 추가할지는 별도 결정이다.

### 5.2 Pair-end missing role

현재 missing-role fixture:

- observed roles: `R1`
- expected headers: `R1`, `R2`
- preview: `R1`만 관찰, invalid report 생성 없음
- FileBlock: valid row 0개, invalid report 1개 생성

해석:

- preview는 read-only observation이다.
- missing required role 후보는 존재하지만 아직 schema validator contract가 아니다.

### 5.3 Pair-end duplicate role

현재 duplicate-role fixture:

- duplicate `R1` collision
- preview: `DuplicateCollisionError`
- FileBlock: `DuplicateCollisionError`

해석:

- duplicate는 current v0.1 typed error다.
- schema validator로 미루지 않는다.

### 5.4 BAM/BAI fixture probe

현재 BAM/BAI probe:

- observed roles: `bam`, `bam_bai`
- normalization preview: `BAM`, `BAI`
- FileBlock cell key: `bam`, `bam_bai`

해석:

- fixture-specific probe는 typed-view rule이 아니다.
- `BAM/BAI` schema validation은 다음 구현 후보일 뿐이다.
- missing-index/unpaired-index diagnostics는 아직 public report contract가 아니다.

## 6. 다음 구현 단위 결정

Sprint 6의 결론은 validator 구현으로 바로 들어가지 않는 것이다.

다음 가장 작은 구현 단위는 schema validator가 아니라 **schema validation preview helper**다.

이 helper의 조건:

- `ResolverPreview` 또는 grouped row map을 입력으로 받는다.
- CSV/protobuf/invalid report를 쓰지 않는다.
- FileBlock output key를 바꾸지 않는다.
- duplicate error contract를 재정의하지 않는다.
- missing/unresolved/extra 후보를 structured observation으로만 반환한다.

권장 첫 테스트:

- pair-end missing-role synthetic case에서 `missing_required_role` 후보를 관찰한다.
- pair-end valid case는 missing 후보가 없어야 한다.
- normalization map이 없는 `R1/R2` unresolved는 missing과 섞지 않는다.

구현 anchor:

- `rules.BuildSchemaValidationPreview`
- `rules.BuildTypedRoleValidationPreview`
- `rules.SchemaValidationPreview`
- `rules.SchemaValidationPreviewEntry`
- `TestBuildSchemaValidationPreview_PairedEndValidHasNoMissingRequiredRole`
- `TestBuildSchemaValidationPreview_PairedEndMissingRoleSeparatesUnresolvedFromMissing`
- `TestBuildSchemaValidationPreview_ExtraObservedRoleIsSeparateObservation`
- `TestBuildTypedRoleValidationPreview_BAMBAICompleteHasNoMissingRequiredRole`
- `TestBuildTypedRoleValidationPreview_BAMOnlyReportsMissingBAI`
- `TestBuildTypedRoleValidationPreview_StaysSeparateFromObservedKeySchemaPreview`

현재 preview helper는 두 층으로 분리되어 있다.

- `BuildSchemaValidationPreview`: observed-key header 기준 preview
- `BuildTypedRoleValidationPreview`: normalized typed role 기준 preview

두 helper는 같은 입력에서도 다른 `Role` 값을 낼 수 있다.
예를 들어 BAM-only row에서 observed-key helper는 `bam_bai` missing을 보고하고, typed-role helper는 `BAI` missing을 보고한다.

Primary/index pairing은 세 번째 층으로 분리한다.

- completeness helper는 required typed role presence만 본다.
- pairing helper 후보는 primary/index 관계 위반만 본다.
- `unpaired_index_role`은 `BuildTypedRoleValidationPreview`에 넣지 않는다.
- 설계 기준은 `docs/track_a_primary_index_pairing_policy_design_v0.1.md`에 둔다.
- 구현 anchor는 `rules.BuildPrimaryIndexPairingPreview`다.

## 7. 비범위

- public warning/report payload
- protobuf schema 변경
- service/gRPC/runtime contract
- FileBlock cell key normalization
- CRAM/CRAI generalization
- full schema validator 전환

## 8. 성공 기준

- preview summary와 validation/report 후보가 문서상 분리된다.
- `UnresolvedRoleCount`를 strict invalid로 읽지 않는 기준이 고정된다.
- duplicate는 current typed error로 유지된다.
- 다음 구현 단위가 schema validation preview helper로 좁혀진다.
