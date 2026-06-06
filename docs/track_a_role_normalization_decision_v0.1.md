# Track A Role Normalization Decision v0.1
### 상태: 구현 전 결정 노트
### 기준선: role normalization seam v0.1

## 1. 결정 요약

v0.1에서는 role normalization map을 **rule spec 쪽에 둔다**.

`DuplicateCollisionError.RoleKey`는 당분간 **observed key** 의미를 유지한다.
normalized role을 표현해야 하는 경우에는 새 필드 또는 새 구조를 도입할 때 별도로 다룬다.

## 2. 왜 rule spec 쪽인가

선택지:

- rule spec map
- recognizer-specific built-in map

현재 결정:

- rule spec map을 우선한다.

이유:

- Track A는 아직 범용 recognizer registry를 구현하는 단계가 아니다.
- BAM/BAI, CRAM/CRAI, pair-end alias는 fixture/rule별로 작게 검증하는 편이 rollback하기 쉽다.
- built-in recognizer로 바로 넣으면 hidden policy가 생겨 문서/fixture와 분리될 위험이 있다.
- rule spec map은 current tokenizer 결과와 desired typed role 사이의 관계를 명시적으로 드러낸다.

## 3. 개념적 rule shape

구현 전 개념 예시:

```json
{
  "roleNormalization": {
    "bam": "BAM",
    "bam_bai": "BAI"
  }
}
```

CRAM/CRAI 예시:

```json
{
  "roleNormalization": {
    "cram": "CRAM",
    "cram_crai": "CRAI"
  }
}
```

Pair-end alias 예시:

```json
{
  "roleNormalization": {
    "R1": "R1",
    "read1": "R1",
    "1": "R1",
    "R2": "R2",
    "read2": "R2",
    "2": "R2"
  }
}
```

이 예시는 최종 JSON schema가 아니다.
현재는 위치와 책임만 고정한다.

## 4. RoleKey 의미

현재 `DuplicateCollisionError.RoleKey`는 observed column key를 뜻한다.

v0.1 결정:

- 기존 `RoleKey` 의미를 바꾸지 않는다.
- normalized role이 필요하면 `NormalizedRole` 같은 별도 표현을 새 contract에서 추가한다.
- 기존 duplicate v0.1 contract를 깨지 않는다.

이유:

- A-2 duplicate contract와 현재 테스트가 `RoleKey`를 current column key로 보고 있다.
- 필드 의미를 조용히 바꾸면 regression anchor가 약해진다.
- observed key와 normalized role을 동시에 볼 수 있어야 전환 중 디버깅이 쉽다.

## 5. Unknown observed key 정책

v0.1에서는 unknown observed key를 바로 strict error로 승격하지 않는다.

현재 결정:

- normalization map에 없는 observed key는 normalization 단계에서 unresolved로 남긴다.
- unresolved key를 invalid/extra/warning 중 무엇으로 볼지는 schema validation/report surface에서 별도로 결정한다.

즉 normalization map 결정과 strict validation 결정은 분리한다.

## 6. 구현 단위

첫 최소 구현:

1. `RuleSet`에 optional normalization map 필드를 추가했다.
2. current `GroupFiles` 결과를 바꾸지 않는 helper를 추가했다.
3. helper는 `observed key -> normalized role` 변환 결과만 테스트한다.
4. duplicate/validation/export 동작은 바꾸지 않았다.
5. row-level normalization preview helper를 추가했지만 row map / FileBlock cell key는 rewrite하지 않았다.
6. 얇은 `RowPreview` helper를 추가해 observed roles와 role normalization preview를 함께 볼 수 있게 했다.
7. 얇은 `ResolverPreview` helper를 추가해 grouped rows 전체를 deterministic ordering으로 preview할 수 있게 했다.
8. `GenerateResolverPreview` entrypoint를 추가해 `fileNames + RuleSet`에서 current grouping 기반 preview를 만들 수 있게 했다.
   duplicate collision은 기존 `DuplicateCollisionError`로 그대로 반환한다.
9. `ResolverPreview`에 내부 summary field를 추가했다.
   현재 field는 `SourceFileCount`, `RowCount`, `ObservedRoleCount`, `UnresolvedRoleCount`에 한정한다.
   valid/invalid row count는 schema validation contract가 아니므로 아직 추가하지 않았다.
10. `GenerateResolverPreviewFromDir` entrypoint를 추가했다.
    이 경로는 `rule.json` 로딩과 file listing을 수행하지만 CSV/protobuf/invalid report를 쓰지 않는 read-only preview path다.
11. shared filesystem BAM/BAI fixture-specific probe를 `GenerateResolverPreviewFromDir` 기반으로 연결했다.
    원본 shared filesystem fixture에는 쓰지 않고 temp workdir에서 preview 산출물이 생성되지 않는 것을 확인한다.

이 helper test는 shared filesystem BAM/BAI fixture-specific probe와 분리한다.
fixture probe는 observed key regression으로 유지하고, helper는 typed role normalization만 검증한다.

성공 기준:

- `bam -> BAM`, `bam_bai -> BAI` 변환이 테스트로 고정된다.
- 기존 `make test-core`와 `make test-shared-fs-fixtures`는 그대로 green이다.
- `DuplicateCollisionError.RoleKey` 의미는 observed key로 유지된다.

구현 anchor:

- `rules.NormalizeRoleKey`
- `rules.BuildRoleNormalizationPreview`
- `rules.BuildRowPreview`
- `rules.BuildResolverPreview`
- `rules.GenerateResolverPreview`
- `rules.GenerateResolverPreviewFromDir`
- `TestNormalizeRoleKey_UsesRuleSetMapWithoutChangingObservedKeySemantics`
- `TestNormalizeRoleKey_MissingMapLeavesObservedKeyUnresolved`
- `TestBuildRoleNormalizationPreview_DoesNotRewriteRowMap`
- `TestBuildRowPreview_ContainsObservedRolesAndNormalizationPreview`
- `TestBuildResolverPreview_DeterministicRowOrdering`
- `TestGenerateResolverPreview_BuildsPreviewFromCurrentGrouping`
- `TestGenerateResolverPreview_CountsUnresolvedRoles`
- `TestGenerateResolverPreview_PreservesDuplicateCollisionError`
- `TestGenerateResolverPreviewFromDir_ReadOnlyPreviewPath`
- `TestGenerateResolverPreviewFromDir_PreservesDuplicateCollisionError`
- `TestLoadRuleSetFromFile_LoadsOptionalRoleNormalization`
- `TestSharedFSFixtureSmoke_AlignmentBAMIndexFixtureSpecificCurrentRuleProbe`

## 7. 비범위

- full schema validator 구현
- FileBlock cell key를 normalized role로 즉시 전환
- user-facing preview/report surface
- protobuf schema 변경
- service/gRPC/runtime/binding 확장
- CRAM/CRAI 추가 probe

## 8. 현재 결론

role normalization은 rule spec이 명시하는 optional map으로 시작했다.

첫 구현은 behavior 전환이 아니라 helper/test 수준으로 제한했다.

Fixture-specific probe와 typed-view rule의 경계는 `docs/track_a_fixture_probe_vs_typed_view_rule_v0.1.md`를 따른다.
