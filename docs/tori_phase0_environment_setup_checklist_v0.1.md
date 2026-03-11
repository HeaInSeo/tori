# tori Phase 0 개발환경 세팅 체크리스트 v0.1

## 1. 문서 목적

이 문서는 tori 개발을 본격적으로 진행하기 전에, 최소한의 개발환경 baseline을 확보하기 위한 Phase 0 체크리스트다.

현재 tori의 1차 구현 목표는 **Track A. File/Data 구조화 계층 확정**이며, 특히 첫 상세 설계 단위는 **FileBlock Rule Resolution** 이다.

따라서 Phase 0의 목적은 다음과 같다.

- Track A 개발을 안전하게 진행할 수 있는 최소 품질 루프를 만든다.
- 로컬 개발 baseline을 확보한다.
- 테스트/린트/도구 버전/온보딩 문서를 정리한다.
- 지금 당장 필요하지 않은 transport/runtime/K8s 확장은 보류한다.

이 문서는 대규모 리팩터링 계획서가 아니라, **Track A 착수를 위한 개발환경 정비 문서**다.

---

## 2. Phase 0 범위

### 2.1 포함 범위

- `go test ./...` 실패 원인 분류
- 최소 Green baseline 확보
- lint 도구 및 설정 baseline 도입
- Makefile / toolchain baseline 정리
- README 최소 온보딩 기준 보강
- 패키지 경계 후보 정리
- 이후 Track A 개발에 필요한 fixture/test 실행 기반 점검

### 2.2 제외 범위

이번 단계에서는 아래를 직접 구현하지 않는다.

- gRPC 서버 실동작 복구
- K8s overlay / Tilt / ko / 배포 루프 도입
- production runtime 설계 확장
- 대규모 패키지 구조 재편
- Track A 이후의 Binding / Runtime / Metadata 설계 구현

---

## 3. 현재 인식한 상태

현재까지 파악된 상태는 다음과 같다.

- tori는 snapshot 기반 파일 구조화/카탈로그 엔진 방향은 맞다.
- rule 기반 파일 그룹핑과 DataBlock/FileBlock 경로의 초기 구현이 존재한다.
- 상위 개발 문서와 Track/Phase 기준선은 이미 존재한다.
- Phase 0 baseline은 실무 착수 기준으로 확보되었고, 현재는 Track A / Phase A-1 freeze를 진행 중이다.
- `go test ./...` 는 여전히 `service` 보류 영역 때문에 Green이 아니다.
- 코어 범위 기준 lint/toolchain baseline은 도입되었고 `.golangci.yml`이 저장소에 반영되어 있다.
- GitHub Actions 기준선(`core-ci`, `security-observe`)이 존재한다.
- README/README.ko 온보딩 문서가 정리되어 있고, 기존 원문은 legacy로 보관되어 있다.
- 코어 baseline(`config`, `db`, `rules`, `block`, `cmd`)은 `make test-core`로 Green 상태를 유지한다.

즉, Phase 0의 최소 개발환경 baseline은 확보되었고, 현재는 이를 기반으로 Track A 본작업(Phase A-1 이후 단계)으로 진행 가능한 상태다.

---

## 4. Phase 0 목표

이번 단계의 목표는 아래 네 가지다.

1. **테스트 상태를 이해하고 정리한다.**
2. **로컬 개발 품질 루프를 만든다.**
3. **패키지 경계와 이후 구조화 포인트를 드러낸다.**
4. **Track A 구현을 시작할 수 있는 상태로 만든다.**

---

## 5. 세부 체크리스트

## 5.1 테스트 baseline 점검

### 목표

`go test ./...` 가 왜 실패하는지 분류하고, 유지할 실패와 정리할 실패를 구분한다.

### 해야 할 일

- [x] 현재 `go test ./...` 결과를 다시 실행해 재현한다.
- [x] 실패를 아래 범주로 분류한다.
    - [x] 실제 코드/설계 불일치
    - [x] 테스트 잔존물/미완성 테스트
    - [x] 환경 의존 경로 문제
    - [x] fixture/파일 배치 문제
    - [x] 설정값 불일치
- [x] 각 실패를 다음 중 하나로 태깅한다.
    - [x] 즉시 수정 대상
    - [x] 임시 격리 대상
    - [x] Track A와 무관하여 후순위 보류
- [x] 최소 Green baseline 전략을 제안한다.

### 기대 산출물

- 테스트 실패 목록
- 실패 원인 분류 표
- Green baseline 확보 제안
- 임시 skip/quarantine 후보 목록

### 주의

- 실패를 무조건 삭제하지 않는다.
- 테스트를 숨기기 전에 왜 숨기는지 기록해야 한다.
- Track A와 무관한 transport/runtime 테스트는 후순위 보류 가능하다.

### 현재 코어 baseline 실행 원칙

- Phase 0/Track A 코어 baseline은 `config`, `db`, `rules`, `block`, `cmd` 범위를 기준으로 실행한다.
- `service` 패키지는 보류된 transport/runtime 영역이므로 현재 코어 baseline에서 제외한다.
- 이는 실패를 은닉하기 위한 조치가 아니라, 보류 영역과 코어 baseline을 분리해 단계적으로 안정화하기 위한 운영 규칙이다.

---

## 5.2 lint / toolchain baseline 도입

### 목표

로컬에서 반복 가능한 lint/quality baseline을 만든다.

### 해야 할 일

- [x] `golangci-lint` 버전 고정 전략을 정한다.
- [x] `.golangci.yml` 초안을 도입한다.
- [x] 최소 lint 범위를 정한다.
    - [x] `govet`
    - [x] `staticcheck`
    - [x] `errcheck`
    - [x] `ineffassign`
    - [x] `unused`
    - [ ] 필요 시 `revive`
- [x] 초기 단계에서는 과도한 린터를 한 번에 넣지 않는다.
- [x] `make lint` 또는 동등한 명령을 만든다.
- [x] `make test` 와 `make lint` 의 기본 진입점을 맞춘다.

### 기대 산출물

- `.golangci.yml`
- tool install/usage 기준
- `make lint`
- lint 실패 항목 정리

### 주의

- kube-slint 수준의 강한 정책을 한 번에 그대로 가져오지 않는다.
- Track A 착수에 필요한 최소 안정성부터 확보한다.

---

## 5.3 Makefile / 개발 명령 baseline 정리

### 목표

개발자가 반복적으로 같은 명령을 쓸 수 있도록 진입점을 표준화한다.

### 해야 할 일

- [x] 현재 Makefile 상태 점검
- [x] 최소 타깃 정의
    - [x] `make test`
    - [x] `make lint`
    - [x] `make fmt`
    - [x] `make vet` 또는 포함 여부 결정
- [x] 필요 시 tool bootstrap 타깃 제안
- [x] README와 실제 명령을 일치시킨다.

### `make test` 와 `make test-core` 역할 구분

- `make test` 는 전체 상태 확인용 명령이다.
- 현재 Phase 0/Track A의 Green baseline 기준 명령은 `make test-core` 다.
- `service` 패키지는 보류된 transport/runtime 영역이므로 현재 코어 baseline에서 제외한다.

### GitHub Actions 기준선(현재)

- `core-ci`는 현재 코어 baseline 차단용 워크플로다.
  - 실행: `make test-core`, `make lint`
  - 범위: Phase 0/Track A 코어 기준(`service` 보류 영역 제외)
- `security-observe`는 보안 관찰용 워크플로다.
  - 실행: `make lint-security`, `make vuln`
  - 결과: `reports/` artifact 업로드
  - 정책: report-only (fail gate 아님)

### 기대 산출물

- 최소 Make targets
- 개발자가 따라야 할 표준 명령 세트

---

## 5.4 README / 온보딩 baseline 정리

### 목표

README가 최소한의 개발 시작 문서 역할을 하도록 보강한다.

### README에 반드시 있어야 할 항목

- 프로젝트 한 줄 설명
- 현재 프로젝트 단계 설명
- 지금의 우선순위(Track A / Phase 0 또는 Phase A-1)
- 필수 도구
- 기본 실행 명령
- 기본 테스트/린트 명령
- 관련 설계 문서 경로
- 아직 미완인 영역(gRPC/K8s runtime 등)

### 해야 할 일

- [x] README에 개발자 시작 경로를 추가한다.
- [x] “무엇이 이미 구현되었고, 무엇이 아직 미완인가”를 적는다.
- [x] 상위 설계 문서 경로를 README에 연결한다.

---

## 5.5 패키지 경계 후보 정리

### 목표

향후 depguard 또는 유사 정책으로 고정할 패키지 경계 후보를 문서화한다.

### 현재 후보 관점

tori는 앞으로 최소 개념적으로 아래 계층을 가질 가능성이 높다.

- core domain
    - snapshot
    - file/data structure
    - rule resolution
    - materialization
- transport
    - grpc / service surface
- runtime / integration
    - future execution / K8s 연결
- cli / app entry
    - cmd

### 해야 할 일

- [x] 현재 패키지들을 위 관점으로 임시 분류한다.
- [x] core가 transport/runtime에 오염되는 지점을 찾는다.
- [x] Track A 이전에 경계 잠금이 필요한 최소 지점을 제안한다.

### 현재 분류(임시, 2026-03-11)

- core domain 후보
  - `rules`: rule load/tokenize/group/validate/export
  - `db`: snapshot/diff/store/query
  - `block`: FileBlock/DataBlock 생성 조합 로직
- transport 후보
  - `service`: DataBlock service surface(gRPC 연계 지점, 현재 보류)
- runtime/integration 후보
  - (현재 저장소 내 본격 runtime 패키지는 미정; 향후 확장 영역)
- cli/entry 후보
  - `cmd`, `main`

### 경계 위반 의심 후보(확정 아님)

- `block` 패키지가 외부 proto service helper(`api-protos/.../service`)에 직접 의존한다.
- `service` 테스트가 `db`/`block` 책임과 transport 책임을 혼합해 참조한다.
- 위 항목은 즉시 구조개편 대상이 아니라, Track A 이후 경계 잠금 후보로 기록한다.

### Track A 이전 최소 경계 잠금 후보(보수적)

- 코어 baseline 명령(`make test-core`, `make lint`)의 범위에서 `service`를 분리 유지한다.
- `rules`/`db`의 current semantics 검증 경로를 우선 고정하고, transport 연동 평가는 보류한다.

### 기대 산출물

- 패키지 경계 후보 메모
- 이후 depguard 적용 초안 후보

### 주의

- 이번 단계에서 대규모 재배치까지 하지는 않는다.
- 먼저 “어디를 잠가야 하는지”를 드러내는 것이 목적이다.

---

## 5.6 Track A 착수 준비 점검

### 목표

Phase 0 이후 바로 Phase A-1 Current Semantics Freeze 로 들어갈 수 있는지 확인한다.

### 해야 할 일

- [x] 현재 rule resolver 관련 코드 위치 확인
- [x] fixture 추가 위치 확인
- [x] golden/snapshot test 추가 가능성 점검
- [x] 문서와 실제 코드 경로의 불일치 확인
- [x] 현재 pair-end 예시 rule이 어디까지 구현돼 있는지 재정리

### 현재 점검 결과(2026-03-11)

- rule resolver 관련 코드 위치
  - `rules/rules.go`: `LoadRuleSetFromFile`, `GroupFiles`, `FilterGroups`, `SaveInvalidFiles`, `ExportResultsCSV`
  - `block/fileblock.go`: `GenerateFileBlock` (rules 결과를 FileBlock/proto 저장으로 연결)
- fixture 추가 가능 위치
  - `rules/` 하위 테스트 인접 경로(`rules/fixtures/` 또는 `rules/testdata/`)가 가장 작은 도입 지점
- golden/snapshot test 가능 위치
  - `rules/rules_test.go` 확장 또는 `rules/snapshot_test.go` 분리
  - CSV/invalid 출력 고정은 `testdata` 기반 비교가 적합
- 문서-코드 경로 불일치
  - 문서에서 제안한 `fixtures/`, `snapshot_test.go` 구조는 아직 미생성(경로 제안 상태)
  - 반면 현재 의미론 핵심 함수 경로는 문서와 정합
- pair-end current semantics 반영 상태
  - `rule.json`의 `header`, `rowRules.matchParts`, `columnRules.matchParts` 기반 grouping/filter/export 경로가 구현되어 있음
  - multi-role 일반화는 아직 미착수(문서 의도와 일치)

### 기대 산출물

- Track A 착수 준비 상태 보고
- 즉시 가능한 작은 개발단위 1개 제안

---

## 6. 즉시 blocker / 이후 과제 구분

## 6.1 즉시 blocker

현재 기준으로 **Track A 코어 착수 관점의 즉시 blocker는 없음**.

- 코어 baseline 명령(`make test-core`)은 Green 상태다.
- 코어 범위(`config`, `db`, `rules`, `block`, `cmd`)는 문서/명령 기준이 정리되었다.
- `service` 영역 실패는 보류된 transport/runtime 범주로 분리 관리한다.

## 6.2 이후 과제

아래는 중요하지만 Track A 직전의 즉시 blocker는 아닐 수 있다.

- `go test ./...` 전체 Red 상태 해소(`service` 보류 영역 정리 이후)
- gRPC 서비스 실동작 복구
- K8s overlay / Tilt / ko 루프
- CI / GitHub Actions 본격 도입
- 정책 게이트 강화
- transport/runtime 계층 확장

---

## 7. 권장 실행 순서

1. 테스트 실패 재현 및 분류
2. 최소 Green baseline 전략 수립
3. lint/toolchain baseline 도입
4. Makefile / README 최소 기준 정리
5. 패키지 경계 후보 문서화
6. Track A 착수 준비 점검
7. 가장 작은 Track A 개발단위 1개 시작

---

## 8. 종료 조건

Phase 0는 아래 조건을 만족하면 종료로 본다.

- [x] `go test` 상태가 이해되고 분류되었다.
- [x] 최소 Green baseline 전략(`make test-core`)이 정의되었다.
- [x] lint/toolchain baseline이 도입되었다.
- [x] 개발자 진입 명령이 문서화되었다.
- [x] README 최소 온보딩 기준이 반영되었다.
- [x] 패키지 경계 후보가 정리되었다.
- [x] Track A를 시작할 수 있는 첫 개발단위가 제안되었다.

### 종료 판정(2026-03-11)

- **조건부 종료 가능**: Phase 0의 코어 baseline 목적은 충족되었다.
- 단, `go test ./...` 전체 Green은 보류 영역(`service`) 정리 전까지 비종결 항목으로 남긴다.

---

## 9. 결과 보고 형식

Phase 0 작업 결과 보고는 아래 형식을 따른다.

### 1) 현재 이해한 기준선 요약
- 현재 프로젝트 단계
- 이번 작업의 범위
- 이번 작업의 비범위

### 2) 실제 저장소 조사 결과
- 테스트 상태
- lint/toolchain 상태
- README/Make 상태
- 패키지 구조 관찰

### 3) 문제 분류
- 즉시 blocker
- 후순위 과제
- 지금 건드리면 안 되는 것

### 4) 제안 작업 단위
- 가장 작은 작업 1개
- 그 다음 작업 후보 2~3개

### 5) 위험/영향 범위
- 각 작업이 미치는 영향
- 롤백 시 영향 범위

### 6) 권장 실행 순서
- 실제 적용 순서 제안

---

## 10. 현재 결론

현재 tori는 Phase 0에서 요구한 코어 baseline(테스트/린트/도구/온보딩/경계 후보)을 확보했다.

따라서 현재 상태의 판단은 다음과 같다.

- Phase 0는 **코어 기준으로 조건부 종료 가능**하다.
- Track A 본작업은 현재 기준으로 착수 가능하다.
- 단, `service` 보류 영역(transport/runtime)의 전체 테스트 복구는 Phase 0 종료 범위 밖의 후속 과제로 분리 유지한다.
