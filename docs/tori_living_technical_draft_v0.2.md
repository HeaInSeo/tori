# tori 설계 정리 초안 v0.2
## 부제: Data Catalog / FileBlock / DataBlock / Pipeline Binding / K8s-first Runtime 정리 + 개발 프로그램 계획
### 상태: 살아 있는 기술문서 초안 + 개발 프로그램 계획 기준선 (검토 반영본)
### 목적: 지금까지의 대화를 기반으로, 흔들리지 않는 기준선을 임시 고정하고 단계적 개발 계획의 상위 기준 문서로 사용한다.

---

## 0. 문서의 성격과 사용법

이 문서는 최종 설계 확정본이 아니다.  
이 문서는 **지금까지 대화로 합의한 내용, 잠정 기준선, 열려 있는 질문**을 함께 담은 “살아 있는 기술문서 초안”이다.

이 문서의 목적은 네 가지다.

1. tori, FileBlock, DataBlock, pipeline binding, Resolved Run Plan, K8s runtime 사이의 관계를 흔들리지 않게 정리한다.
2. 이미 합의한 기준선과 아직 미해결인 부분을 분리한다.
3. 다음 설계 제약으로 넘어가더라도, 현재 주제의 핵심 문맥을 잃지 않게 한다.
4. 상세 설계와 실제 구현 사이를 연결하는 **상위 개발 프로그램 계획 문서**로도 사용한다.

이 문서는 의도적으로 다음 성격을 가진다.

- **상세하다**
- **잠정적이다**
- **열린 질문을 숨기지 않는다**
- **대화에서 나온 정정/수정 사항을 반영한다**
- **유전체 분석의 재연성(reproducibility) 요구를 설계 중심에 둔다**
- **단계별 구현/평가/롤백을 허용하는 프로그램 계획 문서 역할도 가진다**

### 0.1 v0.2에서 추가된 핵심 변화

이번 v0.2에서는 기존의 살아 있는 기술문서 성격을 유지하되, 그 위에 **개발 프로그램 계획**을 올렸다.

즉 이 문서는 이제 두 역할을 동시에 가진다.

- 도메인/아키텍처/책임 경계를 설명하는 기술 초안
- 설계단위와 개발단위를 나누고, 순서와 우선순위를 정하는 상위 계획 문서

이 변경의 이유는 다음과 같다.

- 실제 개발에서는 설계만으로 모든 것을 예측할 수 없다.
- 롤백과 수정 가능성을 두려워하기보다, 단계 경계를 명확히 해서 위험을 줄이는 편이 낫다.
- 따라서 문서도 “최종 확정본”이 아니라, **단계 기준선을 고정하고 다음 실험 단위를 안전하게 정하는 문서**여야 한다.

### 0.2 문서 운영 원칙

이 문서는 다음 운영 원칙 아래에서 사용한다.

1. **설계 우선(spec-first)**  
   구현 전에 현재 단계의 기준선을 문서로 먼저 고정한다.

2. **단계 제한 개발(phase-bounded)**  
   전체 문제를 한 번에 해결하지 않고, 현재 단계의 목표/비목표/종료 조건을 먼저 정한 뒤 구현한다.

3. **롤백 허용(rollback-tolerant)**  
   구현 중 설계 변경, 되돌림, 경계 조정은 정상적인 학습 과정으로 본다.

4. **평가 기반 전진**  
   각 단계는 구현 후 평가를 거쳐 다음 단계로 승격한다.

5. **문서-구현 연동**  
   코드만 먼저 고립적으로 바꾸지 않고, 반드시 현재 설계 문서 버전과 연결해서 진행한다.

### 0.3 현재 문서의 위치

현재 기준으로 이 문서는 다음 체계의 상위 기준 문서 역할을 한다.

- 본 문서: 전체 기준선 + 상위 개발 프로그램 계획
- 후속 상세 문서:
  - FileBlock Rule Resolution Specification
  - Materialized FileBlock / Row Identity Specification
  - DataBlock Packaging Specification
  - Binding / Resolved Run Plan Specification
  - Metadata / Preview UX Specification
  - Execution Profile / Runtime Mapping Specification

즉 지금은 세부 명세를 바로 쓰기 전에, 그 명세들이 어떤 순서와 범위로 진행되는지 먼저 고정한 상태다.

---

## 1. 배경과 문제 정의

### 1.1 출발점

tori는 원래 Lustre 클라이언트 디렉토리 또는 NFS 디렉토리 같은 파일시스템 경로를 스냅샷 형태로 관찰하고, 그 안의 변화를 감지해 다시 스캔하고 업데이트하는 방식으로 구상되었다.

초기 목적은 다음과 같았다.

- 파일시스템의 실제 데이터 상태를 반영해
- 구조화된 데이터 묶음을 만들고
- 이를 protobuf / gRPC 형태로 제공하며
- 사용자가 파이프라인에 개별 파일명을 일일이 입력하지 않아도
- 데이터와 파이프라인을 연결해서 분석할 수 있게 만드는 것

즉, 단순 파일 감시기가 아니라,
**데이터 위치를 파이프라인에 연결할 수 있게 만드는 계층**으로 출발했다.

### 1.2 왜 스냅샷 기반이었는가

초기 설계는 Lustre 확장성, NFS 같은 공유 스토리지 환경에서의 현실성 때문에 snapshot 기반으로 생각되었다.  
즉 “이벤트를 완벽하게 실시간 추적하는 시스템”보다는, **지연은 있어도 안정된 상태를 게시하는 카탈로그**가 더 현실적이라는 판단이 있었다.

### 1.3 지금 시점의 더 정확한 이해

현재까지의 논의를 통해 tori의 정체성은 더 명확해졌다.

> tori는 watcher가 아니라,  
> **snapshot 기반의 data catalog / binding 계층**이다.

즉 핵심은:

- 파일시스템 상태를 관측하고
- 규칙 또는 인식 로직을 통해 구조화하고
- FileBlock / DataBlock / Row 형태로 게시하고
- 파이프라인과 연결 가능한 데이터 선택/바인딩 기반을 제공하는 것

이다.

---

## 2. 이 문서에서 고정하려는 핵심 전제

### 2.1 유전체 분석 도메인 전제

이 시스템이 다루는 도메인은 유전체 분석이다.  
이 분야에서는 **재연성(reproducibility)** 이 매우 중요하다.

즉:

- 같은 파이프라인
- 같은 데이터
- 같은 참조 데이터
- 같은 실행 조건

이면 **같은 결과가 나와야 한다**.

이 전제 때문에 아래 항목은 선택이 아니라 필수 설계 요소다.

- image version / digest 고정
- script version / digest 고정
- 입력 데이터 generation 고정
- reference data version / digest 고정
- parameter snapshot 고정
- execution profile 고정
- materialization / runtime policy 기록
- provenance 기록 가능성 확보

### 2.2 사용자는 누구인가

이 시스템의 사용자는 전형적인 플랫폼 엔지니어만이 아니다.

주요 사용자 후보는 다음과 같다.

- **생명정보학 연구원**
- **기술 지원을 어느 정도 받는 의사**
- **더 단순한 실행 UX만 쓰는 최종 사용자(의사/연구원)**

즉 중요한 점은 다음과 같다.

1. 일부 사용자는 UI에서 직접 DAG를 만들고 node별 shell script를 작성할 수 있다.
2. 일부 사용자는 이미 만들어진 안정화 파이프라인을 선택하고 데이터만 연결해서 실행할 수 있다.
3. 즉 사용자 모델은 “개발자 vs 일반 사용자” 같은 단순 이분법이 아니라, **같은 제품 안에서 숙련도와 역할이 다른 사용자 스펙트럼**에 가깝다.

### 2.3 안정적인 파이프라인 / 불안정한 파이프라인

초기부터 모든 파이프라인이 안정적인 것은 아니다.

불안정한 파이프라인은 존재할 수 있다.

- shell script 문제
- 리소스 부족
- 환경 차이
- 입력 데이터 특성
- 실행 정책 문제

따라서 “안정적인 파이프라인”은 처음부터 주어지는 속성이 아니라,  
**점진적으로 검증되고 튜닝된 결과**다.

이 때문에 파이프라인을 다음 두 층으로 분리해서 보는 것이 중요하다.

- **immutable한 분석 로직**
- **mutable하지만 실행 시점에는 고정되어야 하는 실행 프로파일**

이 관점은 아래 설계 전체를 지배한다.

---

## 3. 전체 설계를 보는 3개 계층

현재까지의 논의를 정리하면, 이 시스템은 최소 3개의 층으로 나눠 생각해야 한다.

### 3.1 Layer A — Data Catalog Layer (tori)

책임:

- snapshot
- diff
- directory scanning
- recognizer / rule 적용
- FileBlock / DataBlock / Row 생성
- generation 관리
- query / resolve의 기반 제공
- CAS 연결
- provenance anchor의 일부 제공

즉 tori는 **데이터를 구조화하고 제공하는 계층**이다.

### 3.2 Layer B — Binding / Planning Layer

책임:

- pipeline logic과 데이터 구조 연결
- node input contract와 FileBlock 컬럼 연결
- row fanout
- execution request 해석
- Resolved Run Plan 생성

즉 이 층은 **사용자 의도와 실행 가능한 계획을 연결하는 계층**이다.

이 층은 과거 caleb에서 고민했던 “1차 파이프라인 / 2차 파이프라인” 문제의 현대화된 형태로 볼 수 있다.

### 3.3 Layer C — Runtime Layer

책임:

- single machine 실행
- K8s execution
- Run / Attempt lifecycle
- admission / scheduling / spawner
- watcher / repair / finalizer
- status offloading
- observability
- recovery

즉 이 층은 **Resolved Run Plan을 실제로 수행하는 계층**이다.

### 3.4 왜 이 3층이 중요한가

지금까지의 혼란은 사실 잘못된 설계가 아니라,  
이 서로 다른 층의 문제가 하나의 문맥에서 동시에 다뤄졌기 때문에 생긴 것이다.

- tori는 데이터 구조화 문제를 다루고 있었고
- caleb은 pipeline binding / 실행 계획 문제를 다루고 있었고
- K8s 문서는 런타임 / churn / recovery 문제를 다루고 있었다

이 문서는 이 셋을 하나의 선으로 재배치하는 역할을 한다.

## 3.5 상위 개발 프로그램 구조

현재 기준으로 tori 개발 프로그램은 다음 다섯 개 Track으로 정리한다.

### Track A. File/Data 구조화 계층 확정

목표:

- source snapshot에서 FileBlock / Row / DataBlock으로 이어지는 구조화 경로를 안정화한다.
- rule 기반 recognizer를 pair-end 예제 수준에서 multi-role typed view 체계로 승격 가능한 형태로 고정한다.
- low-level rule, metadata, grouping, validation, invalid handling 사이의 책임 경계를 정리한다.

포함 주제:

- rule.json 책임 경계
- role schema
- row grouping
- validation semantics
- invalid/duplicate 처리
- materialized FileBlock 결과 구조
- DataBlock / FileBlock / Row 관계의 1차 정리

### Track B. Identity / Generation / Reproducibility 계층 확정

목표:

- rowId, fileBlock generation, dataBlock generation, resolved reference를 안정화한다.
- 재연성과 provenance에 필요한 최소 식별/고정 정책을 수립한다.

포함 주제:

- row identity vs row ordinal
- generation 모델
- canonical FileBlock identity
- source snapshot id
- materialization snapshot
- rule/profile digest 연결

### Track C. Binding / Run Plan 계층 확정

목표:

- Pipeline Logic Spec과 FileBlock role schema를 연결한다.
- Resolved Run Plan을 내부 불변 실행 기준 문서로 구체화한다.

포함 주제:

- node input contract
- semantic role binding
- data binding resolution
- reference input binding
- parameter snapshot
- row refs / generation refs

### Track D. Metadata / Preview UX 계층 확정

목표:

- low-level rule을 사용자에게 직접 노출하지 않으면서도 설명 가능성을 보장한다.
- metadata core / accepted extension / raw metadata / feedback UX를 정리한다.

포함 주제:

- metadata projection
- recognizer explanation
- preview model
- confirm / warn / ignore UX
- 고급 사용자와 일반 사용자 노출 수준 차등

### Track E. Runtime / K8s Alignment 계층 확정

목표:

- 위 도메인 모델을 K8s Pod/Job 의미와 연결 가능한 형태로 정리한다.
- future batching / backpressure / fanout 폭증 제약을 execution profile 쪽에서 회수할 수 있게 한다.

포함 주제:

- execution profile
- resource class
- fanout default
- batching future policy
- scheduling / backpressure 제약 회수

### 3.6 현재 우선순위

현재 가장 먼저 진행할 Track은 **Track A. File/Data 구조화 계층 확정**이다.

그 이유는 다음과 같다.

- 이후 Track B의 identity / generation 안정화도 Track A의 구조화 결과를 전제로 한다.
- Binding / Run Plan도 결국 FileBlock role schema와 Row fanout 의미론 위에서만 안정적으로 정의할 수 있다.
- 지금 실제 구현이 이미 `rule.json` 기반 FileBlock 생성 방향으로 존재하므로, 가장 먼저 현재 의미론과 일반화 방향을 문서화하는 것이 구현과 가장 가깝다.

### 3.7 Track A의 개발 원칙

Track A는 다음 원칙 아래에서 진행한다.

- 한 번에 “범용 rule engine 완성”을 목표로 하지 않는다.
- 먼저 현재 pair-end 예시 구현의 의미론을 문서로 고정한다.
- 그 위에 multi-role typed schema 일반화 방향을 덧붙인다.
- 구현은 설계단위 → 제한된 개발단위 → 평가 → 문서 보정의 순환으로 진행한다.
- Track A 1차 마무리 후에만 Track B로 넘어간다.

---

## 4. tori의 정체성과 역할

### 4.1 tori는 무엇인가

현재 기준선:

> tori는 watcher가 아니라  
> **snapshot 기반 data catalog / binding 계층**이다.

즉 tori는 다음을 한다.

- filesystem 상태를 관측한다
- recognizer / rule / metadata를 이용해 파일들을 구조화한다
- FileBlock / DataBlock / Row를 게시한다
- pipeline binding이 가능하도록 query / resolve 기반을 제공한다

### 4.2 tori가 직접 알지 않아도 되는 것

초기 설계 기준으로 tori는 아래를 직접 알 필요가 없다.

- Pod / Job / Attempt
- K8s churn 정책
- Watcher / Repair / Finalizer
- K8s GC / ownerRef
- control plane failure handling

이것들은 Layer C 문제다.

### 4.3 tori가 반드시 의식해야 하는 것

반대로 tori는 아래는 강하게 의식해야 한다.

- generation
- reproducibility
- CAS
- provenance 가능성
- metadata 추출 / 인식 / 보존
- selection / packaging / query semantics

---

## 5. DataBlock / FileBlock / Row

### 5.1 FileBlock

FileBlock(FB)은 단순 R1/R2 pair 객체가 아니다.  
현재 코드와 논의를 기준으로 보면, FileBlock은 **rule-driven / recognizer-driven grouping 결과인 typed view**에 가깝다.

예:

- paired FASTQ view
- BAM+BAI view
- lane 기준 view
- sample 기준 view
- QC 전용 view

즉 FileBlock은 “파일들을 특정 방식으로 본 결과”다.

### 5.2 Row

Row는 FileBlock 내부의 실행 후보 단위다.  
즉 FileBlock이 table-like structure라면, Row는 그 table의 한 행이다.

예:

- row-p001:
  - R1 -> file A
  - R2 -> file B

현재 기준으로 Row는 다음 성질을 가진다.

- FileBlock 내부의 구조화된 입력 레코드
- fanout의 기본 후보
- binding이 실제 입력으로 전개될 때 가장 중요한 단위
- 나중에 execution unit과 1:1 또는 거의 1:1로 대응될 가능성이 높다

### 5.3 DataBlock

DataBlock(DB)은 단순 “FB 여러 개를 담는 그릇”보다는,  
**하나의 논리적 데이터셋 패키지**로 보는 것이 더 적절하다.

즉 DataBlock은:

- 사용자/시스템이 하나의 데이터셋으로 선택하는 상위 단위
- 같은 의미 또는 같은 publish/generation 경계를 가지는 상위 단위
- 여러 FileBlock view를 묶을 수 있는 패키지
- UI / 실행 요청에서 선택 가능한 데이터셋 단위

### 5.4 추천 해석

현재 잠정 기준선:

- **DataBlock = dataset package**
- **FileBlock = typed view / grouping**
- **Row = 실행 fanout 단위**

### 5.5 DataBlock은 여러 개일 수 있다

시스템 전체에는 여러 DataBlock이 존재할 수 있다.

예:

- DB-raw-fastq-set-A
- DB-aligned-bam-set-A
- DB-variant-vcf-set-A

즉 하나의 시스템 안에 여러 논리 데이터셋 패키지가 있을 수 있고,  
각각은 또 여러 FileBlock view를 가질 수 있다.

### 5.6 FileBlock 중복 가능성

같은 또는 유사한 FileBlock이 여러 DataBlock과 관계를 가질 수 있다.

따라서 다음 구조가 유리할 수 있다.

- FileBlock은 가능한 canonical object로 보고
- DataBlock은 FileBlock을 소유(복제)하기보다는 참조하는 package로 보는 구조

즉 exact duplicate가 있다면 dedup하고,  
여러 DataBlock이 하나의 canonical FileBlock을 참조하는 구조가 더 적합할 가능성이 크다.

이 부분은 아직 최종 확정은 아니지만, 현재 방향은 이쪽으로 기울어 있다.

---

## 6. FileBlock 생성과 DataBlock 생성 책임

### 6.1 초기 우려

현재 tori는 rule 기반으로 FileBlock을 자동 생성하는 쪽으로 구현이 시작되어 있다.  
하지만 여기에는 중요한 UX 문제가 있다.

- 의사나 연구원이 low-level rule을 이해하고 싶어할까?
- 사용자가 rule을 직접 알지 않고도 사용할 수 있어야 하지 않는가?
- FileBlock / DataBlock 생성에 필요한 정보는 누가 준다고 볼 수 있는가?

이 우려는 타당하다.

### 6.2 기본 방향

현재 가장 좋은 방향은 다음과 같다.

> low-level rule은 시스템 내부 구현으로 두고,  
> 사용자는 그 결과를 **메타정보 기반으로 재구성/선택**하게 한다.

즉:

- recognizer / rule은 내부 엔진
- 사용자는 “인식된 결과”와 “보이는 메타”를 조합한다

### 6.3 시스템 생성 FB vs 사용자 파생 FB

이 관점에서 FileBlock은 최소 두 종류로 나눌 수 있다.

#### Canonical FileBlock
시스템이 recognizer / rule을 통해 자동 생성한 기본 FB

#### Derived FileBlock
사용자가 UI에서 보이는 메타정보를 조합해서 재구성한 FB

예:

- Canonical FB: paired FASTQ all
- Derived FB: tumor only / batch A / NovaSeq only

### 6.4 사용자 조합형 DataBlock

사용자는 하나 이상의 FileBlock 또는 Derived FileBlock을 묶어서  
새로운 DataBlock을 만들 수도 있다.

즉 DataBlock도 시스템 생성 패키지일 수 있고,  
사용자 curated package일 수도 있다.

### 6.5 현재 잠정 기준선

- rule은 내부 구현
- canonical FB는 시스템 생성
- 사용자는 메타정보 기반으로 FB를 재구성 가능
- DataBlock은 사용자 선택/패키징 단위로 활용 가능
- 저장 시에는 generation 고정이 필요

---

## 7. Metadata 모델

### 7.1 문제의식

메타정보를 UI에서 조합해서 FB/DB를 만들려면, 메타가 어느 정도 일관되어야 한다.  
하지만 현실은:

- 시퀀싱 장비마다 파일명 다름
- 툴마다 산출 규칙 다름
- 샘플시트 형식 다름
- 어떤 데이터는 메타가 풍부하고, 어떤 것은 거의 없음

즉 모든 메타를 전역적으로 고정된 typed schema로 만드는 것은 비현실적이다.

### 7.2 현재 방향

따라서 메타 모델은 다음 구조가 적절하다.

#### Core Metadata
시스템이 공통적으로 이해하려는 최소 필드

예:
- dataType
- sampleId
- libraryLayout
- columnRoles
- generation
- recognizerType
- sourceRef
- confidence

#### Accepted Extension Metadata
규칙을 통과한 확장 메타

예:
- illumina.runId
- illumina.lane
- nanopore.flowcellId
- manifest.patientGroup

#### Raw Metadata
아직 공식적으로 수용되지 않았지만 보존해야 하는 메타

예:
- 원본 파일명 토큰
- 샘플시트 컬럼
- header 값
- lab custom tags

### 7.3 Metadata는 확장 가능해야 한다

새 장비/새 툴/새 실험형태가 계속 들어올 수 있으므로,  
메타는 반드시 확장 가능해야 한다.

따라서 구조는:

- 작은 Core
- namespace 있는 Extension
- Raw fallback

형태가 적절하다.

### 7.4 확장 규칙과 예외 수용 규칙

단지 “확장 가능”만으로는 부족하다.  
확장을 받아들이는 규칙과, 규칙 밖의 것을 수용하는 규정이 필요하다.

현재 추천 구조:

- Core: 시스템 공통 필드
- Accepted Extension: namespace, 타입, 출처, recognizer version, UI 노출 여부 등이 정의된 확장
- Raw Metadata: 아직 규칙에 맞지 않거나 승인되지 않은 메타

### 7.5 사용자 피드백 UX

사용자는 low-level rule을 배워야 하는 것이 아니라,  
시스템이 해석 결과를 사람말로 설명해줘야 한다.

예:

- “Paired FASTQ로 인식했습니다”
- “12개 sample row를 생성했습니다”
- “platform 정보는 확인하지 못했습니다”
- “새로운 장비 메타 5개는 raw metadata로 저장되었습니다”

즉 메타 확장 거버넌스는 내부 정책만이 아니라  
**사용자 피드백 UX까지 포함하는 문제**다.

---

## 8. Pipeline Spec을 어떻게 볼 것인가

### 8.1 K8s Pod/Job 기준으로 생각한다

현재 방향은 다음이다.

> 파이프라인 스펙은 K8s Pod/Job 기준으로 설계하고,  
> 싱글 머신은 그 의미를 따라가는 구조로 본다.

다만 중요한 점:

- 스펙이 곧바로 Pod YAML이 되면 안 된다
- 대신 Pod/Job으로 자연스럽게 번역 가능한 분석 스펙이어야 한다

### 8.2 Node는 무엇인가

Node는 곧바로 Pod YAML이 아니라,  
**Pod/Job으로 번역 가능한 containerized analysis step contract**로 보는 것이 적절하다.

Node에는 대체로 다음이 포함될 수 있다.

- id
- imageRef (digest)
- scriptRef 또는 inline script + digest
- inputs / outputs
- resourceClass
- standardized work contract (`/in`, `/work`, `/out`)
- env/input contract

---

## 9. Pipeline Spec의 4분리

현재까지의 논의를 반영한 추천 구조:

### 9.1 Pipeline Logic Spec
immutable한 분석 논리

포함:
- DAG
- nodes
- edges
- wiring
- node script / tool recipe
- input/output semantics
- fileblock binding semantics
- image/script version or digest
- resourceClass 이름

### 9.2 Execution Profile
mutable하지만 실행 시점에는 고정되어야 하는 실행/환경 설정

포함:
- cpu/memory
- timeout/retry
- runtime mode
- scheduling class
- materialization strategy
- local/k8s execution differences

### 9.3 Execution Request
이번 실행 요청

포함:
- logicRef
- profileRef
- dataBindings
- reference input
- params
- requester intent

### 9.4 Resolved Run Plan
내부 불변 실행 계획

포함:
- logic 고정
- profile 고정
- DataBlock / FileBlock generation 고정
- row fanout 결과
- execution units
- materialization 계획
- provenance anchor

---

## 10. Script Input Contract

### 10.1 중요한 기준선

shell script 작성자는 FileBlock 내부 구조를 알지 않아도 되어야 한다.

즉 script는:

- fileblock을 몰라도 되고
- row를 몰라도 되고
- CAS를 몰라도 되고
- K8s를 몰라도 된다

script는 오직 **시스템이 약속한 논리 입력 이름**만 안다.

예:
- READS1
- READS2
- REFERENCE
- OUT_SAM
- IN_SAM
- OUT_BAM

### 10.2 Binding은 script 바깥에서 일어난다

예를 들어:

- FileBlock 컬럼: R1, R2
- node input names: reads1, reads2

시스템이 내부적으로:

- reads1 <- R1
- reads2 <- R2

를 연결한다.

즉 binding은 script 안에 박히지 않고, 별도 단계에서 해석된다.

### 10.3 Runtime materialization

실행 직전 runtime은:

- row의 실제 파일을 준비하고
- `/in`, `/work`, `/out`를 구성하고
- env 또는 경로 계약을 제공한 뒤
- script를 실행한다

즉 script는 준비된 입력만 소비한다.

---

## 11. 사용자 UX 관점

### 11.1 파이프라인 작성 사용자

생명정보학 연구원 또는 기술 지원을 받는 의사는 UI에서 다음을 할 수 있다.

- DAG 구성
- node 생성
- shell script 작성
- node input/output 정의
- pipeline 저장

즉 파이프라인 작성은 반드시 개발자 전용이어야 할 필요는 없다.

### 11.2 실행 사용자

더 단순한 사용자는 다음만 할 수 있어야 한다.

- 안정화된 파이프라인 선택
- DataBlock / FileBlock 선택
- 실행
- 결과 보기

### 11.3 중요한 UX 원칙

최종 사용자는 다음을 직접 볼 필요가 없다.

- Pod/Job 세부
- Finalizer / Watcher / Repair
- ownerRef / GC
- low-level rule
- raw runtime materialization detail

즉 UI는 사람의 역할과 숙련도에 따라 다르게 단순화되어야 한다.

---

## 12. Paired FASTQ 시뮬레이션에서 확인한 기준선

우리는 Paired FASTQ -> BAM 예시로 하나의 concrete simulation을 돌렸다.  
그 결과 현재 유효해 보이는 기준선은 다음과 같다.

### 12.1 시뮬레이션 핵심
- tori가 paired FASTQ FileBlock 생성
- row마다 R1/R2 존재
- Pipeline Logic Spec은 align -> sort DAG
- script는 READS1 / READS2 / REFERENCE만 사용
- Execution Request는 fileBlockRef와 reference를 선택
- Resolved Run Plan에서 generation 고정 + row fanout
- K8s에서는 1 row = 1 execution unit = 1 Job
- 싱글 머신은 같은 Resolved Run Plan을 local runtime으로 수행

### 12.2 이 시뮬레이션으로 잠정 유효해 보인 것
- 사용자 binding 단위는 FileBlock
- 실행 fanout 단위는 Row
- script는 FileBlock을 모름
- runtime은 Resolved Run Plan만 소비
- K8s-first 설계와 single machine 추종 구조가 자연스럽다

---

## 13. Reproducibility, CAS, Provenance

### 13.1 CAS를 왜 도입했는가

CAS는 단순 저장 최적화가 아니라,  
재연성과 콘텐츠 기반 식별을 위해 중요하다.

즉 파일 경로나 이름이 아니라 **내용 기반 식별자(digest)** 로 입력과 결과를 붙잡을 수 있어야 한다.

### 13.2 Resolved Run Plan과 재연성

Resolved Run Plan은 단순 실행 계획이 아니라,  
사실상 **재연성 고정 문서** 역할을 한다.

즉 여기에 포함되어야 하는 것:

- logic digest
- profile digest
- image digest
- script digest
- datablock/fileblock generation
- row refs
- reference digest
- parameter snapshot
- materialization policy

### 13.3 Provenance / Graph 가능성

유전체 분석은 결과 생산 비용이 높고, 생산된 결과의 출처를 추적하는 것도 매우 중요하다.

따라서 장기적으로는 다음 관계를 추적하고 싶다.

- 입력 digest
- reference digest
- logic spec
- execution profile
- resolved run plan
- 결과 digest

이 관계는 그래프 구조에 가깝다.

초기 구현에서 graph DB를 바로 도입할 필요는 없지만,  
모델은 나중에 lineage graph로 확장 가능하도록 설계하는 것이 바람직하다.

### 13.4 현재 권장 해석

- CAS = 콘텐츠 식별과 재연성의 기준
- provenance model = 산출물 계보를 표현하는 논리 모델
- graph DB = 미래 확장 가능한 조회/추적 최적화 수단

---

## 14. K8s 적용 시 배경 제약

이 문서의 본문은 도메인 모델과 binding에 집중하지만,  
향후 K8s 적용에서는 아래 제약을 계속 의식해야 한다.

- control-plane churn
- image distribution traffic
- scheduling feasibility
- oversized resource request로 인한 unschedulable risk
- runtime backpressure
- pool vs pod routing
- large fanout에서의 실행 단위 폭증

이 항목들은 현재 문서의 중심 설명 대상은 아니지만,  
향후 runtime/profile 문서에서 반드시 회수해야 하는 제약이다.

---

## 15. 현재까지의 잠정 기준선 요약

### 기준선 1
tori는 watcher가 아니라 snapshot 기반 catalog/binding 계층이다.

### 기준선 2
재연성은 최상위 설계 요구사항이다.

### 기준선 3
파이프라인은 immutable한 Logic Spec과 mutable하지만 실행 시점에 고정되는 Execution Profile로 분리한다.

### 기준선 4
사용자는 FileBlock을 실행 입력으로 연결하고, 시스템은 Row 단위로 fanout 한다.

### 기준선 5
script는 FileBlock을 모르고, 약속된 논리 입력 이름만 사용한다.

### 기준선 6
Resolved Run Plan은 실행 계획이자 재연성 고정 문서다.

### 기준선 7
DataBlock은 dataset package, FileBlock은 typed view, Row는 실행 fanout 단위로 본다.

### 기준선 8
메타는 Core / Accepted Extension / Raw Metadata 구조로 관리하는 방향이 유력하다.

### 기준선 9
low-level rule은 내부 구현으로 두고, 사용자에게는 인식 결과 / preview / metadata 조합 UX를 제공한다.

### 기준선 10
K8s Pod/Job을 기준 런타임으로 보고, 싱글 머신은 그 의미를 따라간다.

## 15A. 현재 개발 계획과 단계 구조

이제부터는 문서에서 합의된 내용을 곧바로 전체 구현으로 옮기지 않고, 아래의 설계단위/개발단위 구조로 진행한다.

### 15A.1 설계단위

설계단위는 특정 주제의 책임 경계와 의미론을 고정하는 문서 단위다.

현재 즉시 착수할 설계단위:

- **FileBlock Rule Resolution Specification v0.1**

이 설계단위에서 다룰 범위:

- 현재 `rule.json` 및 resolver 구현의 의미론
- pair-end 예시가 가진 한계
- multi-role typed schema로의 일반화 방향
- role schema, validation, invalid, duplicate 처리 원칙
- materialization으로 넘어가기 전 resolver 책임 경계

이 설계단위에서 일부러 다루지 않을 범위:

- 최종 DataBlock publish 전략
- 전체 binding 문법 확정
- K8s runtime 세부 정책
- lineage graph 저장소 선택

### 15A.2 개발단위

개발단위는 설계단위를 코드/테스트/fixture 수준으로 제한해서 검증하는 구현 단위다.

현재 Track A 아래에서 예상하는 개발단위 예시는 다음과 같다.

- 현재 rule resolver 동작 fixture 정리
- pair-end current semantics snapshot test
- roles 기반 schema 구조체 초안
- count validator → schema validator 전환 실험
- duplicate collision 감지 추가
- invalid structured output 초안

### 15A.3 단계 종료 조건

설계단위 종료 조건:

- 범위와 비범위가 문서에 명시되었다.
- 현재(as-is) 의미론과 목표(to-be) 의미론이 구분되었다.
- 최소한의 MUST / SHOULD 수준 규칙이 작성되었다.
- 다음 단계가 이 문서를 입력으로 사용할 수 있다.
- 아직 열어둘 질문이 별도 섹션에 정리되었다.

개발단위 종료 조건:

- 코드/테스트/fixture가 현재 설계 문서 버전에 대응된다.
- 성공 기준이 충족되었다.
- 새롭게 발견된 이슈가 기록되었다.
- 롤백 시 영향 범위가 명확하다.
- 다음 개발단위로 넘어갈지, 설계 수정이 필요한지 평가가 남았다.

### 15A.4 롤백과 문서 고도화 원칙

- 롤백은 실패가 아니라 위험 제한 수단으로 본다.
- 문서 확정은 영구 고정이 아니라 “현재 단계 기준선 확정”으로 본다.
- 구현으로 인해 설계 충돌이 발견되면, 문서를 보정한 뒤 다시 진행한다.
- Track A 1차 진행 중 문서는 계속 고도화될 수 있다.
- 단, 문서 없는 코드 변경이나 문서와 무관한 즉흥적 확장은 지양한다.

---

## 16. 아직 미해결인 열린 질문

이 문서에서 일부러 확정하지 않은 것들이다.  
다음 제약 논의 전에 이 목록을 유지해야 한다.

### 16.1 DataBlock / FileBlock 관계
- DataBlock generation과 FileBlock generation을 1:1로 묶을 것인가
- DataBlock과 FileBlock의 N:M 관계를 공식 허용할 것인가
- canonical FB identity는 무엇으로 정의할 것인가

### 16.2 Row identity
- rowId는 generation 내부에서만 유일하면 되는가
- 세대 간 같은 논리 row를 이어볼 필요가 있는가
- sampleId와 rowId를 분리할 것인가

### 16.3 FileBlock 선택 UX
- 사용자는 DataBlock만 선택하는가
- FileBlock까지 직접 선택하는가
- 파이프라인 요구사항에 따라 시스템이 자동 선택하는가
- 사용자가 수정/확인하는 단계가 필요한가

### 16.4 Metadata Core
- Core metadata 최소 집합은 무엇인가
- sampleId / patientId / platform / batch 중 무엇까지 core로 둘 것인가
- 어떤 필드는 optional로 남겨도 되는가

### 16.5 Metadata feedback UX
- unrecognized metadata를 사용자에게 어떻게 보여줄 것인가
- warning / confirm / ignore 수준을 어떻게 나눌 것인가
- 고급 사용자와 일반 사용자에게 같은 정보를 보여줄 것인가

### 16.6 Node input contract
- env naming 표준을 어떻게 정할 것인가
- `/in`, `/work`, `/out` 표준을 고정할 것인가
- path + env 혼합 계약으로 갈 것인가

### 16.7 Execution unit default
- 초기 기본값을 정말 1 row = 1 execution unit = 1 Job으로 둘 것인가
- future batching 정책은 profile로만 제어할 것인가

---

## 17. 다음 주제로 넘어가기 전 점검 체크리스트

이 문서 검토 시 다음을 확인한다.

- 용어가 사람말로 충분히 풀려 있는가
- 현재 합의한 내용과 어긋난 해석이 없는가
- 너무 빨리 확정해버린 내용은 없는가
- 아직 열린 질문이 숨겨지지 않았는가
- 유전체 분석의 재연성 요구가 충분히 중심에 놓였는가
- DataBlock / FileBlock / Row / metadata / pipeline binding 사이 연결이 자연스러운가

---

## 18. 결론

현재 시점에서 가장 중요한 판단은 다음이다.

- 지금까지의 고민은 흩어진 실패가 아니라, 하나의 시스템을 다른 층에서 먼저 만져본 흔적이다.
- 지금 필요한 것은 새 아이디어를 계속 추가하는 것보다, 이미 나온 아이디어를 층별로 정리하고 기준선을 고정하는 것이다.
- 그리고 이제는 그 기준선을 실제 개발로 연결하기 위해, 상위 개발 프로그램 계획을 함께 가져가야 한다.

현재 기준으로 즉시 착수할 범위는 다음과 같다.

1. 본 문서를 **상위 기준 문서 v0.2** 로 사용한다.
2. 가장 먼저 **Track A. File/Data 구조화 계층 확정**을 1차 목표로 삼는다.
3. Track A 안에서 첫 상세 설계단위인 **FileBlock Rule Resolution Specification v0.1** 을 작성한다.
4. 그 문서를 바탕으로 제한된 개발단위를 정의하고, 구현/평가/문서 보정을 반복한다.
5. Track A가 1차 마무리되면 그 다음에 Track B 이후로 이동한다.

즉, 다음 단계의 목표는 “전체 시스템을 한 번에 구현”하는 것이 아니라,  
**Track A를 문서와 구현이 연결되는 수준까지 1차로 마무리하는 것**이다.

이 문서는 그 작업의 상위 기준 문서로 계속 업데이트된다.

---

## 19. 구현 현황 스냅샷 (2026-03-11 기준)

이 절은 문서 기준선과 실제 코드 상태를 연결하기 위한 as-is 기록이다.

### 19.1 현재 동작 중인 범위

- CLI 엔트리포인트는 `tori-admin` 기준으로 동작한다.
- `snapshot` 명령은 현재 디렉터리 스냅샷을 DB에 저장한다.
- `sync` 명령은 DB/실제 폴더 diff 후 변경 시 FileBlock 생성과 `datablock.pb` 생성을 수행한다.
- `dump` 명령은 `datablock.pb`를 텍스트로 변환 저장한다.
- Rule 해석은 `rule.json` 기반(`delimiter`, `rowRules.matchParts`, `columnRules.matchParts`)으로 파일명을 그룹화한다.
- valid/invalid 분리와 CSV/invalid 파일 출력 경로가 구현되어 있다.

### 19.2 아직 미구현 또는 임시 상태

- `serve` 명령의 gRPC 서버 실행부는 주석 처리 상태로 실제 서버 기동을 하지 않는다.
- `DataBlockServer`의 RPC 핸들러들(`SyncFolders`, `SaveFolders`, `GetDataBlock`)은 주석 처리 상태다.
- 문서에서 제시한 Binding/Resolved Run Plan/Runtime 계층은 현재 저장소에서 본격 구현 전 단계다.
- multi-role typed view 일반화는 아직 시작 전이며 pair-end 중심 semantics가 유지된다.

### 19.3 문서-코드 갭(현재 인지)

- 상위 문서의 3계층/Track 구조는 방향성 기준선이고, 코드 구현은 현재 Track A의 초기 as-is 보존에 집중되어 있다.
- 일부 TODO가 코드 주석에 남아 있어 명명/경계(예: `SaveFolders`의 의미, 파라미터 정리) 고정 작업이 필요하다.
- `header` 기반 표현과 실제 discovered column key 정렬 기반 CSV 출력 사이 의미 불일치 가능성이 남아 있다.

### 19.4 다음 문서 업데이트 원칙

- 이후 문서 갱신 시에는 반드시 "기준 날짜"를 절 제목에 남긴다.
- 설계 결정(should)과 구현 현황(as-is)을 같은 문단에 섞지 않고 분리한다.
- Track A 범위 변경 시 본 문서와 `fileblock_rule_resolution_spec_v0.1.md`를 동시에 갱신한다.
