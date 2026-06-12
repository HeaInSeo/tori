# Track A Shared Filesystem Fixture Sprint Plan v0.1
### 상태: 진행 일정 기준선
### 기준선: Phase 0 baseline + A-1 current semantics freeze + A-2 duplicate minimum contract + shared filesystem fixture collection

## 1. 목적

이 문서는 현재 NAS에 수집된 fixture 결과를 Track A 진행 일정으로 전환하기 위한 스프린트 계획이다.

현재 fixture pack은 NAS 경로인 `/mnt/genomics-test/tori-public-fixtures`에 있다.
현재 실행 기준은 NAS다. Lustre/NFS 적용은 fixture mount가 준비된 뒤 같은 smoke contract로 재검증한다.
유전체 fixture 파일은 저장소나 원격 테스트 장비 로컬 저장소로 복사하지 않는다.

관련 입력 문서:

- `docs/nas_fixture_test_coverage_report_v0.1.md`
- `docs/nas_fixture_run_results_v0.1.md`
- `docs/duplicate_policy_contract_v0.1.md`
- `docs/pipeline_facing_binding_contract_v0.1.md`
- `docs/track_a_nas_completion_remaining_work_2026_06_12.md`

## 2. 운영 원칙

- Track A File/Data structuring 범위에 한정한다.
- service/gRPC/runtime/K8s 확장은 이번 스프린트 계획의 범위 밖이다.
- shared filesystem fixture는 real-data regression anchor로 쓰되, 저장소 내부에는 작은 synthetic fixture만 둔다.
- 각 스프린트는 `make test-core` green을 유지해야 한다.
- shared filesystem smoke는 명시적으로 실행하는 opt-in 검증으로 둔다.

## Sprint 0 - Shared Filesystem Smoke Baseline

목표:

- shared filesystem fixture가 현재 개발 환경에서 접근 가능한지 확인한다.
- 현재 pair-end happy path와 duplicate collision path를 반복 가능한 명령으로 고정한다.
- fixture를 복사하지 않고 실제 tori FileBlock 경로를 검증한다.

비목표:

- 새 typed-view rule을 만들지 않는다.
- missing/extra 정책을 바꾸지 않는다.
- transport/runtime 경로를 건드리지 않는다.

작업:

- `make test-shared-fs-fixtures` 진입점을 추가한다.
- `make test-nas-fixtures`는 현재 NAS 경로를 쓰는 호환 alias로 유지한다.
- valid paired FASTQ는 shared filesystem의 file list와 `rule.json`을 입력으로 사용해 `GenerateFileBlock` 결과 row 수와 header를 확인한다.
- invalid paired FASTQ는 `DuplicateCollisionError` surface를 확인한다.

성공 기준:

- `make test-core`가 green이다.
- fixture root가 mount된 환경에서 `make test-shared-fs-fixtures`가 green이다.
- fixture root가 없는 일반 환경의 기본 테스트는 영향을 받지 않는다.

현재 상태:

- 완료.

완료 메모:

- NAS/Lustre mount가 read-only일 수 있으므로 smoke test는 shared filesystem 디렉토리에 산출물을 쓰지 않는다.
- `rule.json`만 임시 디렉토리로 읽어 오고, 유전체 fixture 파일은 복사하지 않는다.
- `fileblock.csv`, `*.pb`, `invalid_files_*` 산출물은 테스트 임시 디렉토리에만 생성된다.
- 2026-06-02 기준 `make test-core`와 `make test-shared-fs-fixtures`가 모두 green이다.

## Sprint 1 - Invalid Fixture Split

목표:

- 현재 `paired_fastq_invalid`에 섞여 있는 missing-role case와 duplicate-role case를 분리한다.
- duplicate fail-fast 때문에 missing-role 관찰이 가려지는 문제를 제거한다.

비목표:

- strict validation/report surface를 새로 정의하지 않는다.
- multi-role schema를 도입하지 않는다.

작업:

- shared filesystem fixture pack에 다음 디렉토리를 분리한다.
  - `paired_fastq_missing_role`
  - `paired_fastq_duplicate_role`
- 각 디렉토리의 `rule.json`과 checksum/provenance를 갱신한다.
- smoke test를 두 케이스로 분리한다.

성공 기준:

- duplicate fixture는 typed error로 실패한다.
- missing-role fixture는 duplicate 없이 invalid row path를 관찰할 수 있다.
- fixture coverage report가 split 결과를 반영한다.

현재 상태:

- 완료.

완료 메모:

- `paired_fastq_missing_role`와 `paired_fastq_duplicate_role` 디렉토리를 shared filesystem fixture pack에 추가했다.
- missing-role fixture는 duplicate collision 없이 invalid row path를 관찰한다.
- duplicate-role fixture는 `DuplicateCollisionError` path를 독립적으로 관찰한다.
- 2026-06-02 기준 `make test-shared-fs-fixtures`는 valid, missing-role, duplicate-role 세 경로를 모두 검증한다.

## Sprint 2 - Pair-End Regression Contract

목표:

- shared filesystem pair-end fixture 결과를 current Track A regression contract로 승격한다.
- synthetic A-1 fixture와 shared filesystem real-data fixture의 역할을 분리한다.

비목표:

- pair-end 외 typed view를 구현하지 않는다.
- output protobuf binary schema 변경을 하지 않는다.

작업:

- valid pair-end shared filesystem 결과의 최소 검증 항목을 문서화한다.
- `fileblock.csv`와 `.pb` 생성 여부를 smoke 범위에서 확인한다.
- generated output은 shared filesystem 또는 테스트 임시 산출물로만 유지한다.

성공 기준:

- pair-end happy path, duplicate collision, missing-role invalid path가 서로 독립적으로 검증된다.
- `make test-core`와 `make test-shared-fs-fixtures`의 책임 차이가 README에 명확하다.

현재 상태:

- 완료.

완료 메모:

- Pair-end regression contract는 `docs/track_a_pair_end_regression_contract_v0.1.md`에 고정했다.
- `make test-shared-fs-fixtures`는 valid pair-end, missing-role, duplicate-role을 독립적으로 검증한다.
- valid/missing-role 경로는 `fileblock.csv`와 `.pb` 생성 여부도 테스트 임시 디렉토리에서 확인한다.
- synthetic A-1 fixture는 `make test-core`의 작은 semantics anchor로 유지하고, shared filesystem fixture는 opt-in real-data regression anchor로 분리했다.

## Sprint 3 - Next Typed-View Candidate Selection

목표:

- shared filesystem fixture pack에서 다음 Track A typed-view 후보 하나를 고른다.
- 구현 전에 rule/spec 후보와 성공 기준만 고정한다.

후보:

- `alignment + index`: BAM/BAI 또는 CRAM/CRAI
- `variant + index`: VCF/CSI 또는 BCF/CSI
- `reference + annotation`: FASTA/FAI/GFF3/GTF

권장:

- 첫 후보는 `alignment + index`로 둔다.
- 이유는 primary/index 관계가 선명하고, pair-end보다 작은 multi-role 확장 단위로 다루기 쉽기 때문이다.

성공 기준:

- 후보 하나의 role set, row key, duplicate/missing 처리 가설이 문서화된다.
- 구현 여부는 다음 스프린트로 넘긴다.

현재 상태:

- 완료.

완료 메모:

- 다음 typed-view 후보는 `alignment + index`로 고정했다.
- 첫 probe 후보는 BAM/BAI이고, CRAM/CRAI는 같은 category의 second fixture로 둔다.
- 후보 선정 기준은 `docs/track_a_alignment_index_typed_view_candidate_v0.1.md`에 정리했다.
- 이 단계에서는 구현을 시작하지 않았고, role/key/policy gap만 문서화했다.

## Sprint 4 - First Non-FASTQ Typed-View Probe

목표:

- Sprint 3에서 고른 후보를 대상으로 최소 rule/test probe를 만든다.
- 이 단계는 full generalization이 아니라 하나의 typed-view probe다.

비목표:

- multi-role schema 전체 설계를 닫지 않는다.
- DataBlock packaging 최종 정책을 확정하지 않는다.
- runtime binding으로 확장하지 않는다.

성공 기준:

- 하나의 non-FASTQ typed-view 후보가 fixture와 함께 검증된다.
- 새로 드러난 policy gap이 다음 설계 문서 입력으로 정리된다.

현재 상태:

- 완료.

완료 메모:

- 첫 non-FASTQ probe는 BAM/BAI fixture 1세트로 수행했다.
- `TestSharedFSFixtureSmoke_AlignmentBAMIndexFixtureSpecificCurrentRuleProbe`가 현재 tokenizer rule로 1개 alignment row를 생성함을 검증한다.
- 결과는 `docs/track_a_alignment_index_probe_result_v0.1.md`에 정리했다.
- 이 probe에서 current tokenizer key(`bam`, `bam_bai`)와 desired typed role(`BAM`, `BAI`) 사이의 normalization gap이 드러났다.

## Sprint 5 - Resolver Preview Boundary

예상 기간:

- 2026-06-03

목표:

- `GenerateResolverPreviewFromDir`를 shared filesystem fixture smoke에 연결한다.
- preview 단계와 FileBlock generation 단계를 분리한다.
- pair-end와 BAM/BAI probe를 같은 preview summary contract로 관찰한다.

비목표:

- FileBlock cell key를 normalized role로 바꾸지 않는다.
- strict schema validator를 구현하지 않는다.
- user-facing report/protobuf/runtime surface로 확장하지 않는다.

작업:

- valid pair-end fixture에서 read-only preview summary를 고정한다.
- missing-role pair-end fixture에서 read-only preview가 invalid report를 쓰지 않음을 고정한다.
- duplicate-role pair-end fixture에서 read-only preview가 `DuplicateCollisionError`를 보존함을 고정한다.
- BAM/BAI fixture probe에서 `bam/bam_bai -> BAM/BAI` normalization preview를 고정한다.

성공 기준:

- `make test-core`가 green이다.
- `make test-shared-fs-fixtures`가 green이다.
- preview 단계는 `fileblock.csv`, protobuf, `invalid_files_*`를 쓰지 않는다.
- FileBlock generation 단계의 current observed-key behavior는 유지된다.

현재 상태:

- 완료.

완료 메모:

- `rules.GenerateResolverPreviewFromDir`를 pair-end valid, missing-role, duplicate-role, BAM/BAI fixture smoke에 연결했다.
- pair-end fixture는 `roleNormalization`이 없으므로 `R1/R2` observed role이 unresolved preview로 남는다.
- BAM/BAI fixture는 fixture-specific rule의 `roleNormalization`으로 `BAM/BAI` preview를 확인한다.
- duplicate-role preview는 기존 `DuplicateCollisionError` contract를 유지한다.

## Sprint 6 - Schema Validation Boundary Design

예상 기간:

- 2026-06-03 ~ 2026-06-04

목표:

- preview에서 관찰한 unresolved/missing/extra를 strict validation으로 승격할지 여부의 경계를 설계한다.
- pair-end missing-role과 BAM/BAI typed role gap을 같은 schema validation 후보로 정리한다.

비목표:

- validator 구현을 바로 시작하지 않는다.
- FileBlock export key를 바꾸지 않는다.
- protobuf/service/runtime contract를 변경하지 않는다.

작업:

- current preview summary field와 validation/report field를 분리해서 문서화한다.
- `UnresolvedRoleCount`가 warning, invalid, extra 중 무엇으로 해석될 수 있는지 후보를 정리한다.
- pair-end missing-role과 BAM/BAI missing-index/unpaired-index를 같은 validation vocabulary로 표현할 수 있는지 검토한다.

성공 기준:

- 다음 구현 단위가 helper/test인지 schema validator인지 결정된다.
- 문서와 코드의 current semantics 충돌이 없다고 확인된다.
- `make test-core`와 `make test-shared-fs-fixtures`는 계속 green이다.

현재 상태:

- 완료.

완료 메모:

- `docs/track_a_schema_validation_boundary_design_v0.1.md`에 preview summary와 validation/report 후보의 경계를 고정했다.
- duplicate는 current typed error로 유지하고, unresolved/missing/extra는 observation 또는 validator 후보로 분리했다.
- `UnresolvedRoleCount`를 strict invalid로 읽지 않는 기준을 명시했다.
- 다음 구현 단위는 full schema validator가 아니라 schema validation preview helper로 좁혔다.

## Sprint 7 - Schema Validation Preview Helper

예상 기간:

- 2026-06-04

목표:

- Sprint 6에서 정리한 validation 후보를 코드의 read-only helper로 관찰한다.
- pair-end missing-role synthetic case를 기준으로 `missing_required_role` 후보를 고정한다.

비목표:

- FileBlock generation behavior를 바꾸지 않는다.
- invalid report writer를 바꾸지 않는다.
- public warning/report/protobuf surface를 만들지 않는다.
- BAM/BAI general typed-view validator를 구현하지 않는다.

작업:

- schema validation preview entry/summary 구조 후보를 `rules` 계층에 작게 둔다.
- `headers + row preview` 기준으로 missing required role 후보를 계산한다.
- unresolved role과 missing required role을 서로 다른 observation으로 유지한다.
- duplicate collision은 기존 error path를 유지한다.

성공 기준:

- pair-end valid synthetic case는 missing 후보가 없다.
- pair-end missing-role synthetic case는 `missing_required_role` 후보를 낸다.
- normalization map이 없는 `R1/R2` unresolved는 missing 후보와 섞이지 않는다.
- `make test-core`와 `make test-shared-fs-fixtures`가 green이다.

현재 상태:

- 완료.

완료 메모:

- `rules.BuildSchemaValidationPreview`를 추가했다.
- `unresolved_observed_role`, `missing_required_role`, `extra_observed_role` 후보를 read-only preview로 분리했다.
- pair-end valid synthetic case는 missing 후보가 없음을 고정했다.
- pair-end missing-role synthetic case는 `R1` unresolved observation과 `R2` missing required 후보를 분리한다.
- extra observed role 후보도 별도 observation으로 고정했다.

## Sprint 8 - Shared Filesystem Validation Preview Smoke

예상 기간:

- 2026-06-04

목표:

- Sprint 7 helper를 shared filesystem fixture smoke에 연결한다.
- pair-end valid/missing fixture의 schema validation preview 후보를 real-data fixture 기준으로 고정한다.

비목표:

- FileBlock generation behavior를 바꾸지 않는다.
- invalid report 생성 정책을 바꾸지 않는다.
- public report/protobuf/API surface를 만들지 않는다.
- BAM/BAI general typed-view validator를 구현하지 않는다.

작업:

- valid pair-end shared filesystem fixture에서 missing required 후보가 없음을 확인한다.
- missing-role shared filesystem fixture에서 `missing_required_role` 후보가 1개임을 확인한다.
- unresolved observation과 missing required 후보가 섞이지 않음을 확인한다.

성공 기준:

- `make test-core`가 green이다.
- `make test-shared-fs-fixtures`가 green이다.
- shared filesystem 원본 fixture에는 쓰지 않는다.

현재 상태:

- 완료.

완료 메모:

- `TestSharedFSFixtureSmoke_PairedFASTQ`에 `rules.BuildSchemaValidationPreview`를 연결했다.
- valid pair-end shared filesystem fixture는 missing/extra 후보가 없고, `R1/R2` unresolved observation만 유지됨을 확인한다.
- missing-role shared filesystem fixture는 `R1` unresolved observation과 `R2` `missing_required_role` 후보를 분리한다.
- FileBlock generation behavior와 invalid report 생성 정책은 변경하지 않았다.

## Sprint 9 - Alignment Index Validation Preview Candidate

예상 기간:

- 2026-06-04 ~ 2026-06-05

목표:

- BAM/BAI fixture probe에 schema validation preview helper를 연결할지 결정한다.
- `BAM/BAI` typed-view general rule로 넘어가기 전, fixture-specific rule에서 missing/unpaired-index 후보를 어떻게 관찰할지 정리한다.

비목표:

- BAM/BAI general typed-view validator를 구현하지 않는다.
- CRAM/CRAI로 확장하지 않는다.
- public diagnostics/report/protobuf surface를 만들지 않는다.

작업:

- 현재 BAM/BAI probe의 `roleNormalization` 결과와 schema validation preview 후보가 충돌하지 않는지 확인한다.
- unpaired index 후보를 구현할지, 문서 후보로만 둘지 결정한다.
- missing BAM 또는 missing BAI fixture가 필요한지 산정한다.

성공 기준:

- 다음 구현이 fixture 추가인지 helper 확장인지 결정된다.
- `make test-core`와 `make test-shared-fs-fixtures`가 green이다.

현재 상태:

- 완료.

완료 메모:

- BAM/BAI fixture probe에 `rules.BuildSchemaValidationPreview`를 연결했다.
- 현재 preview는 typed role `BAM/BAI`가 아니라 observed-key header `bam/bam_bai` 기준으로 missing/extra/unresolved 후보가 없음을 확인한다.
- `docs/track_a_alignment_index_validation_preview_candidate_v0.1.md`에 general typed-role validation으로 바로 읽지 않는 경계를 문서화했다.
- 다음 구현은 fixture 추가보다 typed-role validation preview helper를 synthetic test로 먼저 진행하는 것으로 결정했다.

## Sprint 10 - Typed Role Validation Preview Helper

예상 기간:

- 2026-06-05

목표:

- observed-key schema preview와 별도로 typed role 기준 validation preview helper를 추가한다.
- BAM/BAI synthetic case에서 normalized role 기준 missing candidate를 관찰한다.

비목표:

- FileBlock cell key를 normalized role로 바꾸지 않는다.
- shared filesystem fixture pack을 변경하지 않는다.
- unpaired index policy를 public diagnostics로 승격하지 않는다.

작업:

- `ResolverPreview`의 role normalization 결과를 typed role set으로 투영한다.
- typed role header 후보(`BAM`, `BAI`)를 입력받아 missing required role 후보를 계산한다.
- BAM-only synthetic case에서 `BAI` missing 후보를 고정한다.
- complete BAM/BAI synthetic case는 missing 후보가 없음을 고정한다.

성공 기준:

- `make test-core`가 green이다.
- `make test-shared-fs-fixtures`가 green이다.
- 기존 observed-key `BuildSchemaValidationPreview` semantics와 충돌하지 않는다.

현재 상태:

- 완료.

완료 메모:

- `rules.BuildTypedRoleValidationPreview`를 추가했다.
- helper 입력은 `ResolverPreview`와 required typed role 목록으로 제한했다.
- complete BAM/BAI synthetic case는 missing 후보가 없음을 고정했다.
- BAM-only synthetic case는 `missing_required_role` for `BAI` 후보를 낸다.
- observed-key `BuildSchemaValidationPreview`와 typed-role helper가 같은 입력에서 서로 다른 role vocabulary를 쓰는 경계를 테스트로 고정했다.

## Sprint 11 - Unpaired Index Policy Boundary

예상 기간:

- 2026-06-05

목표:

- `BAI` only row를 단순 missing primary로 볼지, unpaired index 후보로 볼지 결정한다.
- 결정 전에는 public diagnostics/report surface로 승격하지 않는다.

비목표:

- shared filesystem fixture pack을 즉시 변경하지 않는다.
- CRAM/CRAI generalization을 시작하지 않는다.
- FileBlock output key를 normalized role로 바꾸지 않는다.

작업:

- `BAI` only synthetic case를 현재 helper로 관찰한다.
- `missing_required_role` for `BAM`만으로 충분한지 평가한다.
- `unpaired_index_role`이 필요하면 별도 helper vocabulary로 분리할지 문서화한다.

성공 기준:

- 다음 구현이 helper 확장인지 fixture 추가인지 결정된다.
- `make test-core`와 `make test-shared-fs-fixtures`가 green이다.

현재 상태:

- 완료.

완료 메모:

- `BAI` only synthetic case를 typed-role validation preview 테스트로 고정했다.
- 현재 helper는 `missing_required_role` for `BAM`을 반환한다.
- 이 결과는 typed role completeness preview로 유지하고, `unpaired_index_role`은 아직 구현하지 않는다.
- unpaired index는 primary/index pairing rule이 필요하므로 completeness helper와 분리한다.

## Sprint 12 - Primary/Index Pairing Policy Design

예상 기간:

- 2026-06-05

목표:

- `BAM/BAI`처럼 primary/index 관계가 있는 typed view에서 pairing violation을 어떻게 표현할지 설계한다.
- completeness missing과 primary/index pairing violation을 섞지 않는다.

비목표:

- public diagnostics/report/protobuf 변경을 하지 않는다.
- shared filesystem fixture pack을 즉시 변경하지 않는다.
- CRAM/CRAI 확장 규칙을 구현하지 않는다.

작업:

- primary role과 index role vocabulary를 문서상 정의한다.
- `unpaired_index_role`이 필요한 조건을 정한다.
- helper 추가가 필요하면 `BuildTypedRoleValidationPreview`와 별도 함수로 둘지 결정한다.

성공 기준:

- 다음 구현 단위가 synthetic helper인지 shared fixture 추가인지 결정된다.
- `make test-core`와 `make test-shared-fs-fixtures`가 green이다.

현재 상태:

- 완료.

완료 메모:

- `docs/track_a_primary_index_pairing_policy_design_v0.1.md`를 추가했다.
- primary role은 `BAM`, index role은 `BAI`로 문서상 정의했다.
- `unpaired_index_role`은 `BuildTypedRoleValidationPreview`에 넣지 않는 것으로 결정했다.
- 다음 구현 후보는 별도 pairing preview helper로 산정했다.
- shared filesystem fixture pack 변경은 helper semantics가 synthetic test로 닫힌 뒤로 미뤘다.

## Sprint 13 - Typed Role Pairing Preview Helper

예상 기간:

- 2026-06-05

목표:

- primary/index pairing violation을 completeness preview와 분리한 helper로 관찰한다.
- `BAI` only synthetic case에서 `unpaired_index_role` 후보를 낸다.

비목표:

- public diagnostics/report/protobuf 변경을 하지 않는다.
- shared filesystem fixture pack을 변경하지 않는다.
- CRAM/CRAI generalization을 하지 않는다.

작업:

- pairing preview entry/result 타입을 기존 preview 타입으로 재사용할지 결정한다.
- `BuildPrimaryIndexPairingPreview` 후보를 구현한다.
- `BAI` only synthetic case는 `unpaired_index_role`을 낸다.
- `BAM` only synthetic case는 pairing entry를 내지 않는다.

성공 기준:

- `make test-core`가 green이다.
- `make test-shared-fs-fixtures`가 green이다.
- `BuildTypedRoleValidationPreview`의 current completeness semantics가 유지된다.

현재 상태:

- 완료.

완료 메모:

- `rules.BuildPrimaryIndexPairingPreview`를 추가했다.
- 기존 `SchemaValidationPreview` 타입을 재사용하되 reason code는 `unpaired_index_role`로 분리했다.
- `BAI` only synthetic case는 `unpaired_index_role`을 낸다.
- `BAM` only synthetic case와 complete `BAM/BAI` synthetic case는 pairing entry를 내지 않는다.
- `BuildTypedRoleValidationPreview`의 current completeness semantics는 유지했다.

## Sprint 14 - Pairing Preview Fixture Connection Decision

예상 기간:

- 2026-06-05

목표:

- pairing preview를 shared filesystem smoke에 연결할지, unpaired index fixture 추가 전까지 synthetic anchor로 둘지 결정한다.

비목표:

- public diagnostics/report/protobuf 변경을 하지 않는다.
- CRAM/CRAI generalization을 하지 않는다.
- fixture manifest/checksum을 성급히 변경하지 않는다.

작업:

- current complete BAM/BAI shared fixture에 pairing preview를 연결할 수 있는지 확인한다.
- missing/unpaired index fixture 없이 positive-only smoke를 추가할 가치가 있는지 판단한다.
- unpaired index fixture 추가가 필요하면 fixture 이름, expected result, manifest 갱신 범위를 산정한다.

성공 기준:

- 다음 구현이 smoke 연결인지 fixture 추가인지 결정된다.
- `make test-core`와 `make test-shared-fs-fixtures`가 green이다.

현재 상태:

- 완료.

완료 메모:

- complete BAM/BAI shared filesystem fixture에 pairing preview positive smoke를 연결했다.
- 해당 fixture는 `BuildPrimaryIndexPairingPreview(preview, "BAM", "BAI")` 결과가 entry 0개임을 확인한다.
- unpaired index negative coverage는 shared fixture가 아직 없으므로 synthetic test anchor에 유지한다.
- fixture manifest/checksum 변경은 하지 않았다.

## Sprint 15 - Unpaired Index Alignment Fixture Addition Decision

예상 기간:

- 2026-06-05

목표:

- `alignment_bam_unpaired_index` shared fixture를 추가할지 결정한다.
- fixture 추가 시 expected preview/result와 manifest/checksum 갱신 범위를 산정한다.

비목표:

- CRAM/CRAI generalization을 하지 않는다.
- public diagnostics/report/protobuf 변경을 하지 않는다.
- FileBlock output key normalization을 하지 않는다.

작업:

- unpaired index fixture가 Track A regression anchor로 필요한지 판단한다.
- fixture 이름과 파일 목록을 확정한다.
- 추가한다면 shared filesystem fixture pack manifest/checksum 갱신 절차를 실행한다.

성공 기준:

- 다음 구현이 fixture 추가인지 synthetic-only 유지인지 결정된다.
- `make test-core`와 `make test-shared-fs-fixtures`가 green이다.

현재 상태:

- 완료.

완료 메모:

- `alignment_bam_unpaired_index` shared fixture는 내일 마감 범위에서 추가하지 않기로 결정했다.
- unpaired index coverage는 `rules.BuildPrimaryIndexPairingPreview` synthetic test anchor에 유지한다.
- shared filesystem fixture pack은 complete BAM/BAI positive smoke까지만 유지한다.
- fixture manifest/checksum 변경은 마감 후 fixture expansion sprint로 보류한다.

## Sprint 16 - Tomorrow Scope Freeze and Final Verification

예상 기간:

- 2026-06-05 ~ 2026-06-06

목표:

- 2026-06-06 마감 기준으로 Track A regression baseline을 닫는다.
- 새 기능 추가보다 검증, 문서 정합성, 보류 영역 분리를 우선한다.

비목표:

- 새 shared filesystem fixture를 추가하지 않는다.
- CRAM/CRAI, VCF/CSI, reference index pairing generalization을 시작하지 않는다.
- public diagnostics/report/protobuf 변경을 하지 않는다.
- service/gRPC/K8s/runtime 영역을 복구하지 않는다.

작업:

- `make test-core`와 `make test-shared-fs-fixtures`를 최종 green으로 유지한다.
- Track A 문서와 코드 helper 의미론 충돌을 점검한다.
- 이번 마감에 포함되는 범위와 보류 범위를 최종 보고로 분리한다.
- dirty worktree에서 Track A 직접 변경과 보류/기존 변경을 구분한다.

성공 기준:

- core/shared fixture gate가 green이다.
- pair-end regression, BAM/BAI preview, primary/index pairing preview가 문서와 테스트에서 일치한다.
- 내일 이후 fixture expansion 후보가 별도 다음 단계로 남는다.

현재 상태:

- 예정.

## 3. 현재 진행 판단

현재 NAS fixture 수집은 완료된 입력으로 본다.

현재 실행 기준은 NAS로 제한한다. Lustre 적용은 fixture mount 준비 전까지 후순위 보류다.
repo 안의 반복 가능한 shared filesystem smoke gate는 Sprint 0에서 닫았고, Sprint 5에서 read-only resolver preview boundary까지 고정했다.

## 4. 다음 작업

다음 작업은 Tomorrow Scope Freeze and Final Verification이다.

다음 작업 진입 조건은 다음과 같다.

- `make test-core` green
- `make test-shared-fs-fixtures` green
- shared filesystem fixture 파일을 로컬 checkout에 복사하지 않았음
- current tokenizer key(`bam`, `bam_bai`)와 desired typed role(`BAM`, `BAI`)의 차이가 확인됨
- pair-end missing-role preview와 duplicate-role preview가 shared filesystem 기준으로 고정됨
- preview summary와 validation/report 후보의 경계가 문서화됨
- schema validation preview helper가 synthetic case로 고정됨
- schema validation preview helper가 shared filesystem pair-end fixture smoke에 연결됨
- BAM/BAI fixture-specific observed-key schema preview와 typed-role validation 후보 경계가 문서화됨
- typed-role validation preview helper가 synthetic BAM/BAI complete/missing case로 고정됨
- BAI-only synthetic case가 current completeness preview로 고정됨
- primary/index pairing policy boundary가 문서로 분리됨
- typed-role pairing preview helper가 synthetic case로 고정됨
- complete BAM/BAI shared fixture에 pairing positive smoke가 연결됨
- unpaired index alignment shared fixture 추가는 내일 마감 범위에서 보류됨

현재 다음 작업:

- alignment/index role normalization seam은 `docs/track_a_role_normalization_seam_v0.1.md`에 문서화했다.
- role normalization decision은 `docs/track_a_role_normalization_decision_v0.1.md`에 문서화했다.
- preview/validation boundary는 `docs/track_a_schema_validation_boundary_design_v0.1.md`에 문서화했다.
- schema validation preview helper는 `rules.BuildSchemaValidationPreview`로 구현했다.
- shared filesystem validation preview smoke는 pair-end valid/missing fixture에 연결했다.
- typed-role validation preview helper는 `rules.BuildTypedRoleValidationPreview`로 구현했다.
- unpaired index는 아직 구현하지 않고 primary/index pairing policy design으로 분리했다.
- 다음 구현 후보는 `BuildPrimaryIndexPairingPreview`다.
- typed-role pairing preview helper는 `rules.BuildPrimaryIndexPairingPreview`로 구현했다.
- complete BAM/BAI shared fixture에 pairing preview positive smoke를 연결했다.
- unpaired index alignment shared fixture 추가는 Sprint 17 follow-up에서 완료했다.
- BAM/BAI alignment index validation preview 후보 결정은 `docs/track_a_alignment_index_validation_preview_candidate_v0.1.md`에 문서화했다.
- 내일 마감 기준 다음 구현은 하지 않고 최종 검증과 보고를 우선한다.
- CRAM/CRAI complete fixture probe는 NAS smoke에 연결했다.
- 마감 후 첫 follow-up으로 existing BAI filename을 이용한 temp workdir unpaired index smoke를 추가했다.
- 이후 Sprint 17 follow-up에서 `alignment_bam_unpaired_index/` real fixture를 추가하고 manifest/checksum을 갱신했다.
- shared filesystem smoke는 이제 real unpaired index fixture directory를 읽는다.

다음 작은 결정:

- VCF/CSI, BCF/CSI, FASTA/FAI primary/index pairing probe는 Sprint 19 follow-up에서 NAS smoke로 연결했다.
- NAS 기준 primary/index probe 결과는 `docs/track_a_nas_completion_remaining_work_2026_06_12.md`에 정리했다.
- 다음 후보는 Lustre fixture mount가 준비되면 같은 smoke contract를 재검증하는 것이다.
- normalization map은 v0.1에서 rule spec 쪽에 둔다는 기존 결정은 유지한다.
- current `RoleKey`는 observed key 의미를 유지한다는 기존 결정은 유지한다.

추가 경계:

- fixture-specific probe와 general typed-view rule은 `docs/track_a_fixture_probe_vs_typed_view_rule_v0.1.md` 기준으로 분리한다.
