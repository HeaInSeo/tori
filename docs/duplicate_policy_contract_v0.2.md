# Duplicate Policy Minimum Contract v0.2 (TDI-I12 subject-scoped conflict isolation)

### 상태: 활성 contract. v0.1 whole-group fail-fast posture를 **supersede**한다.
### 기준선: docs/duplicate_policy_contract_v0.1.md (superseded), central-supplied TDI-I12 minimum contract

## 0. Supersession note (읽고 시작)

`docs/duplicate_policy_contract_v0.1.md`는 duplicate collision을 만나면 **배치(폴더/세대) 전체를
`DuplicateCollisionError`로 중단**하는 whole-group fail-fast를 canonical 동작으로 규정했다.

v0.2부터 **whole-folder / whole-generation duplicate abort는 더 이상 canonical I12 동작이 아니다.**
duplicate collision은 하나의 subject coordinate로 **격리(isolate)**되며, 같은 배치의 건강한 sibling
subject를 중단시키지 않는다. v0.1 문서는 역사적 기준선으로 보존하되(삭제 금지), publication /
projection 경로의 동작 정의는 본 문서가 대체한다.

## 1. Authoritative TDI-I12 minimum contract (그대로 구현)

1. Duplicate collision은 구조적 **CONFLICT**이다.
2. CONFLICT의 scope는 **하나의 stable subject coordinate**이다.
3. 한 subject의 duplicate가 같은 observation/publication 배치의 **건강한 sibling subject를 중단시켜서는
   안 된다.**
4. 충돌한 subject는 **conflict 증거로서 계속 보여야 한다** — 사라져서는 안 된다.
5. 임의의 duplicate 후보가 **승리한 healthy member가 되어서는 안 된다.**
6. Duplicate 증거는 **결정적(deterministic) 순서**로 다음을 보존한다:
   `reason_code = duplicate_role_in_row`, subject coordinate, role, candidates, source filenames.
7. Tori는 **source/grouping fact만** 방출한다. Tori는 pipeline runnability, authorization,
   Auto-Run policy, Run 생성을 결정하지 않는다.

## 2. Canonical implementation seam

- **Canonical grouping (publication/projection authority):** `rules.GroupFilesIsolated`
  → `rules.IsolatedGroupingResult{ Healthy, Conflicts }`.
  - `Healthy`: 충돌이 없는 subject들만 (`map[int]map[string]string`, legacy row-index → role → file).
  - `Conflicts`: `[]rules.SubjectConflict` (stable subject key로 정렬). 각 `SubjectConflict`는
    `SubjectKey` / `SubjectComponents` / `RowKey` / `Roles []DuplicateRoleEvidence` /
    `SourceFileNames`(subject의 모든 원본 파일, 정렬)를 보존한다.
  - `DuplicateRoleEvidence`: `ReasonCode="duplicate_role_in_row"`, `Role`, `Candidates`(정렬),
    `SourceFileNames`(정렬).
  - duplicate가 있어도 error를 반환하지 않는다 (배치 전체 중단 없음).
- **Publication authority:** `block.GenerateFileBlockFromDir`, `block.GenerateFileBlock`,
  `block.GenerateFileBlockWithRuleSet`, `block.ProjectFileBlock`는 모두 `GroupFilesIsolated`를
  통해 그룹핑한다. 건강한 subject만 FileBlock으로 발행되고, 충돌 subject는 healthy winner가 되지
  않으며 `rules.InvalidRowsFromConflicts`를 통해 `invalid_files` 리포트로 남아 계속 보인다.

## 3. Legacy / non-canonical 처리

- `rules.GroupFiles`는 **LEGACY / NON-CANONICAL**로 명시 fencing되었다. duplicate가 있으면 여전히
  배치 전체를 `DuplicateCollisionError`로 거부한다. 이는 두 가지 이유로 안전하다: (a) 어떤
  publication/projection 경로도 더 이상 `GroupFiles`에 의존하지 않는다(= publication authority가
  아니다), (b) 임의 승자 선택이나 충돌 은폐 없이 **큰 소리로 거부**만 한다.
- Resolver preview (`rules.GenerateResolverPreview` / `GenerateResolverPreviewFromDir`)는
  진단용(read-only, 아티팩트 미생성)으로 legacy `GroupFiles`를 계속 사용하며 duplicate에 대해
  loud-refusal을 유지한다. preview는 publication authority가 아니다.
- 따라서 **authoritative duplicate semantics는 정확히 하나**(격리)이며, legacy 경로는 fencing되어
  authoritative가 아니다. 모순되는 두 개의 authoritative semantics를 남기지 않는다.

## 4. reason code / 필드 (v0.1과의 연속성)

- reason code는 v0.1과 동일하게 `duplicate_role_in_row` 하나를 유지한다.
- `Candidates`/`SourceFileNames`는 v0.2에서 **결정적 정렬 순서를 보장한다**(v0.1은 순서 미보장).
- multi-role subject: 한 subject의 여러 role이 각각 충돌하면 `SubjectConflict.Roles`에 role별
  evidence가 role 정렬 순서로 누적된다. CONFLICT scope는 여전히 subject coordinate 하나이다.

## 5. 검증 (tests)

`rules/conflict_isolation_test.go`의 I12-T01~T05:

- **I12-T01** isolated conflict: 단일 subject의 duplicate → 해당 subject = CONFLICT,
  `duplicate_role_in_row` 증거 + candidates/source filenames 보존.
- **I12-T02** healthy sibling survives: A=conflict, B=healthy → A는 conflict 유지, B는 정상 healthy
  grouped 결과, 배치 전체 중단 없음.
- **I12-T03** deterministic evidence: 입력 순서를 바꿔도 conflict 증거가 canonical/deterministic.
- **I12-T04** multiple independent conflicts: 서로 다른 두 subject가 각자 duplicate → 독립적으로
  scope된 두 conflict, cross-subject 증거 혼합 없음, healthy sibling 보존.
- **I12-T05** no arbitrary winner: 한 role에 여러 후보 → 어떤 후보도 조용히 healthy member가 되지
  않음, subject는 healthy로 downgrade되지 않고 conflict 증거가 계속 보임.

publication authority 회귀: `block.TestGenerateFileBlock_IsolatesDuplicateConflictWithoutWholeBatchFailure`
는 duplicate가 있는 배치에서 error 없이 healthy sibling이 발행되고 충돌 subject 파일이
`invalid_files`로 계속 보이는지 확인한다.

## 6. 범위 밖 (non-goals)

full SubjectGeneration lifecycle, Auto-Run, Pipeline binding, SourceRevision redesign,
rule-storage relocation, completion-evidence redesign, supersession lifecycle, public
proto/API redesign, UI. 본 문서와 I12 격리 구현은 이 항목들을 포함하지 않는다.
