# tori Product Readiness Sprint Plan v0.1
### 기준일: 2026-06-21
### 상태: 제안 일정 — RUO/research-first, clinical use 별도 결정

## 1. 목적과 제품 경계

이 계획은 tori를 먼저 **재현 가능한 genomics input catalog + pipeline binding** 제품으로 출시 가능한 수준까지 올리기 위한 일정이다.

이 문서가 다루는 첫 출시 목표는 다음으로 제한한다.

- 연구용/RUO 환경에서 snapshot 기반으로 입력 파일을 구조화한다.
- 입력 snapshot, rule, FileBlock, binding, 실행 조건을 재현 가능한 기록으로 남긴다.
- 공유 파일시스템(NAS와 Lustre)에서 같은 Track A 계약을 검증한다.

다음은 이 일정의 출시 주장에 포함하지 않는다.

- variant calling 또는 임상 해석의 정확성 주장
- 진단/치료 의사결정용 clinical release
- 의료기기 또는 LDT 규제 적합성 선언
- K8s runtime을 포함한 대규모 실행 플랫폼

임상 사용으로 확장할지 여부는 RUO release 이후 intended use, 관할 규제, 분석 assay를 입력으로 하는 별도 프로그램에서 결정한다.

## 2. 현재 출발점

완료된 기준선:

- Phase 0 core baseline과 `make test-core`
- NAS shared-fixture Track A smoke (`make test-shared-fs-fixtures`)
- pair-end valid/missing/duplicate regression anchor
- BAM/BAI, CRAM/CRAI, VCF/CSI, BCF/CSI, FASTA/FAI의 현재 probe 범위
- duplicate typed error와 header-exact validation

아직 없는 제품 준비 요소:

- source snapshot manifest와 파일 digest 기반 identity
- materialized FileBlock / Row identity
- pipeline binding과 Resolved Run Plan의 구현
- immutable logic spec / fixed execution profile 기록
- 사용자·권한·감사·보존 정책
- 운영 관측, backup/restore, release qualification
- Lustre parity 검증

## 3. 일정 전제

- Sprint는 2주로 잡는다. 일정은 2026-06-22 시작을 가정한다.
- Track A 코어와 제품화 작업을 한 Sprint에서 동시에 크게 확장하지 않는다.
- Lustre endpoint와 fixture mount는 Sprint 1 시작 전까지 제공돼야 한다. 제공되지 않으면 Sprint 1은 blocker 기록만 남기고 다른 Sprint를 앞당긴다.
- 각 Sprint 종료 시 최소 `make doctor`, `make test-core`, `make lint`를 실행한다. shared storage를 다루는 Sprint는 해당 smoke도 실행한다.
- 모든 Sprint는 목표, 비목표, 변경 계약, rollback 방법, 검증 결과를 문서로 남긴다.

## 4. Sprint 일정

| Sprint | 기간 | 목표 | 종료 기준 |
|---|---|---|---|
| 0 | 6/22–7/3 | NAS baseline을 제품 준비 입력으로 공식 종료하고 개발환경 preflight를 정착 | `doctor`, core, lint, NAS smoke green; fixture manifest/checksum 확인; NAS closure note 갱신 |
| 1 | 7/6–7/17 | Lustre parity 검증 | 승인된 mount에서 NAS와 동일한 shared-fixture smoke green; mount/options/latency 관찰 기록. Lustre 전용 기능 추가 금지 |
| 2 | 7/20–7/31 | RUO intended use와 제품 책임 경계 고정 | product requirements v0.1, non-goals, data classification, supported input/view 목록, failure policy 승인 |
| 3 | 8/3–8/14 | Snapshot manifest와 파일 identity 최소 구현 | 상대경로, size, mtime, digest 정책, source generation, rule version을 포함한 immutable manifest 및 golden test |
| 4 | 8/17–8/28 | Materialized FileBlock/Row identity | stable FileBlock/Row identity와 deterministic materialization; duplicate/missing/extra diagnostics 연결; 이전 A-1 anchor 유지 |
| 5 | 8/31–9/11 | RuleSpec v0.2 설계와 첫 multi-role migration seam | RoleSpec(required/cardinality), observed→typed role 경계 명세와 one typed-view migration. pair-end 회귀 유지 |
| 6 | 9/14–9/25 | Pipeline binding 최소 contract | FileBlock role과 pipeline input slot의 정적 binding, validation, preview, fixture/golden test. runtime 실행은 비범위 |
| 7 | 9/28–10/9 | Resolved Run Plan v0.1 | source generation, manifest digest, rule/FileBlock/binding/logic/profile version을 고정하는 계획 문서와 replay test |
| 8 | 10/12–10/23 | 실행 재현성 경계 | immutable Logic Spec과 fixed Execution Profile 모델, container/reference/script digest 요구사항, 변경 불가 필드 검증 |
| 9 | 10/26–11/6 | 보안·감사 최소선 | 사용자/역할 모델, access decision audit, artifact access boundary, secret 관리 원칙, threat model과 테스트 |
| 10 | 11/9–11/20 | 운영 신뢰성 최소선 | catalog backup/restore drill, corruption/partial-write failure test, metrics/logging, support runbook |
| 11 | 11/23–12/4 | 출시 검증과 beta qualification | requirements-to-test traceability, representative corpus, upgrade/rollback test, performance envelope, known-limitations 문서 |
| 12 | 12/7–12/18 | RUO release candidate 판정 | release checklist, security review, provenance/replay acceptance, NAS/Lustre parity 결과, go/no-go 회의 |

## 5. Sprint별 우선순위와 비목표

### Sprint 0–1: storage baseline

목표는 NAS와 Lustre에서 동일한 resolver 계약이 유지되는지 확인하는 것이다. filesystem 고유 최적화, striping 정책, HA/failover 검증은 이번 범위 밖이다.

### Sprint 2–5: data contract

이 구간이 제품 핵심이다. 파일명만으로 재현성을 주장하지 않고, source manifest와 versioned rule, stable identity를 연결한다. 여러 typed view를 한꺼번에 구현하지 않고 Sprint 2에서 정한 지원 목록만 구현한다.

### Sprint 6–8: planning contract

실행 요청이 아니라 replay 가능한 Resolved Run Plan을 만든다. K8s scheduler, gRPC server 복구, 분산 runtime은 이 단계의 비목표다.

### Sprint 9–12: product hardening

RUO 제품에 필요한 접근제어, 감사, 복구, release evidence를 만든다. 이 결과가 임상 적합성을 뜻하지는 않는다.

## 6. 공통 수용 기준

각 기능은 다음을 모두 만족해야 한다.

1. 문서의 책임 경계와 구현이 일치한다.
2. deterministic fixture/golden test가 있다.
3. 오류에는 machine-readable reason code와 사람이 읽을 수 있는 context가 있다.
4. snapshot, rule, materialization, binding, execution profile의 version/digest 관계가 기록된다.
5. 이전 fixture와 계획 문서가 의도 없이 깨지지 않는다.
6. rollback 절차와 data migration 필요 여부를 release note에 기록한다.

## 7. Release gates

### RUO beta gate (Sprint 11)

- supported data/view 목록이 문서와 테스트로 일치한다.
- NAS 및 준비된 Lustre에서 같은 shared-fixture contract가 green이다.
- 하나의 input manifest에서 동일 FileBlock과 Resolved Run Plan을 재생성하는 replay test가 green이다.
- access audit, backup/restore, upgrade/rollback drill 결과가 있다.
- 알려진 제한과 비지원 형식이 사용자 문서에 명시된다.

### RUO release gate (Sprint 12)

- beta에서 발견된 P0/P1 defect가 해소되었거나 명시적으로 release blocker로 판정됐다.
- 보안 검토와 운영 runbook review가 완료됐다.
- release artifact, schema/rule compatibility, migration/rollback plan이 고정됐다.

### Clinical transition decision gate (RUO release 이후)

이 gate는 출시 gate가 아니라 별도 투자/범위 결정이다.

- intended use와 대상 관할을 확정한다.
- 임상 workflow에서 tori가 책임지는 결과와 사람/외부 분석기가 책임지는 결과를 분리한다.
- assay별 analytical/clinical validation, quality management system, risk management, cybersecurity, change-control 요구사항의 gap assessment를 수행한다.
- gap assessment 승인 전에는 임상 성능 또는 진단 적합성 주장을 하지 않는다.

## 8. 주요 위험과 대응

| 위험 | 대응 |
|---|---|
| Lustre mount 미준비 | Sprint 1을 block으로 기록하고 Sprint 2–5를 앞당긴다. Lustre smoke를 NAS 결과로 대체하지 않는다. |
| 지원 형식이 계속 늘어남 | Sprint 2 지원 목록 밖 형식은 fixture/design 후보로만 유지하고 release scope에 넣지 않는다. |
| snapshot/digest 비용 과다 | digest 전략(전체/증분/지연 계산)을 Sprint 3에서 benchmark와 함께 결정한다. |
| runtime 확장 압력 | Sprint 6–8은 static planning까지만; runtime은 별도 Track으로 분리한다. |
| 임상 요구와 RUO 요구 혼합 | intended use 변경은 Sprint 2 문서 변경과 clinical transition gate를 다시 통과해야 한다. |

## 9. 다음 작업

첫 실행 단위는 Sprint 0이다.

1. `make doctor`, `make test-core`, `make lint`, `make test-shared-fs-fixtures`를 실행한다.
2. NAS fixture manifest/checksum 확인 방법과 결과를 문서화한다.
3. NAS closure note를 최신 검증일과 함께 갱신한다.
4. Lustre endpoint/mount 준비 상태를 Sprint 1 dependency로 기록한다.

### Sprint 0 실행 기록 (2026-06-21)

완료:

- `env -u GOROOT make doctor`
- `env -u GOROOT make lint`
- `env -u GOROOT make test-core`
- `env -u GOROOT make test-shared-fs-fixtures`
- NAS fixture root에서 `sha256sum -c metadata/SHA256SUMS.txt`

모든 명령이 통과했다. checksum manifest에 열거된 모든 artifact가 일치했다.
Lustre fixture endpoint/mount 정보는 아직 제공되지 않았으므로 Sprint 1의 외부 dependency로 유지한다.

### Sprint 1 preflight 기록 (2026-06-21)

원격 테스트 VM에서 기존 `/etc/fstab`의 주석 처리된 Lustre mount 후보를 확인했다.
VM에는 Lustre client/kernel module과 비대화식 sudo가 준비되어 있고, `modprobe -n -v lustre`는 필요한 module load 순서를 정상적으로 제시했다.

그러나 후보 MGS endpoint의 Lustre TCP port 연결은 `connection refused`로 실패했다.
따라서 module load와 mount는 수행하지 않았다. 이는 tori 코드 blocker가 아니라 Lustre server service, network reachability, 또는 firewall 정책을 확인해야 하는 외부 dependency다.

Sprint 1 재개 조건:

1. 운영자가 fixture filesystem용 MGS NID와 filesystem name이 현재 유효함을 확인한다.
2. 원격 테스트 VM에서 해당 MGS endpoint의 Lustre service port에 연결할 수 있다.
3. read-only fixture mount의 승인된 target path를 확인한다.

세 조건이 충족되면 mount 후 `TORI_SHARED_FIXTURE_ROOT`를 해당 root로 지정해 NAS와 동일한 checksum 및 shared-fixture smoke를 실행한다.
