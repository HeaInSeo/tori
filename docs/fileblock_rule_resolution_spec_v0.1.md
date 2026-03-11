# tori FileBlock Rule Resolution Specification v0.1
## 부제: Track A / File-Data 구조화 계층의 첫 상세 설계 문서
### 상태: 상세 설계 초안 (개발 전 기준선)
### 상위 문서: tori 설계 정리 초안 v0.2 / 개발 프로그램 계획
### 목적: source snapshot으로부터 canonical FileBlock / Row 후보를 생성하는 rule resolution 계층의 의미론, 책임 경계, 단계적 개발 기준을 고정한다.

---

## 0. 문서 목적과 위치

이 문서는 tori 개발 프로그램 계획에서 **Track A. File/Data 구조화 계층 확정**의 첫 상세 설계 문서다.

이 문서가 다루는 주제는 다음 하나로 제한한다.

> 주어진 source snapshot과 rule specification을 이용해,
> 시스템 내부의 canonical FileBlock 후보와 Row 후보를 어떻게 생성하고 검증할 것인가.

즉 이 문서는 다음을 고정한다.

- 현재 `rule.json` 기반 구현의 실제 의미론(as-is)
- pair-end 예시를 넘어서 data type별 multi-role typed view로 일반화하는 방향(to-be)
- resolver의 책임 경계
- validation / invalid / duplicate / materialization 경계
- 이후 Materialized FileBlock / Row Identity 문서로 넘겨야 할 열린 질문

이 문서는 아직 다음을 최종 확정하지 않는다.

- DataBlock publish / packaging 전략 최종안
- Binding 문법 최종안
- Resolved Run Plan 내부 구조 상세
- K8s runtime translation 상세
- lineage / graph 저장 방식

---

## 1. 범위 / 비범위

### 1.1 범위

이 문서는 다음 범위를 포함한다.

1. source snapshot에서 resolver가 읽는 입력의 의미
2. `rule.json` 또는 rule spec이 담당하는 책임
3. 파일명 tokenization, grouping, role derivation, validation, invalid 처리의 의미론
4. FileBlock / Row / role schema의 관계
5. pair-end 예시의 현재 구조와 그 한계
6. multi-role typed view 일반화 방향
7. 개발 1차 구현에서 허용되는 단순화와 추후 승격 포인트

### 1.2 비범위

이 문서는 다음 범위를 일부러 제외한다.

1. DataBlock과 FileBlock의 최종 N:M 관계 확정
2. rowId의 세대 간 연속성 정책 확정
3. canonical FB identity의 최종 해시 구성 확정
4. metadata core 최소 필드 최종 확정
5. 사용자-facing preview 화면 구체 UI
6. node input contract의 최종 env/path naming
7. execution unit batching 정책

---

## 2. 배경과 설계 제약

현재 tori의 상위 기준선은 다음과 같다.

- tori는 watcher가 아니라 snapshot 기반 data catalog / binding 계층이다.
- 재연성(reproducibility)은 최상위 요구사항이다.
- FileBlock은 typed view, Row는 실행 fanout 단위, DataBlock은 dataset package로 본다.
- low-level rule은 시스템 내부 구현이며, 사용자에게는 인식 결과와 preview를 보여주는 방향이 적절하다.
- Resolved Run Plan은 단순 실행 계획이 아니라 재연성 고정 문서다.
- 최종 사용자는 low-level rule을 직접 이해하지 않아도 되어야 한다. fileciteturn11file1

이 전제 아래에서 rule resolution 계층은 단순 유틸이 아니라,
**재연성 가능한 데이터 구조화 엔진의 첫 단계**로 설계되어야 한다.

---

## 3. 용어 정의

### 3.1 Source Snapshot

어떤 시점의 파일시스템 입력 집합.

포함 가능한 것:
- 파일명
- 상대경로
- 파일 크기
- 수정 시각
- digest(있다면)
- 확장 메타 소스(샘플시트, sidecar, manifest)

v0.1 범위에서는 최소 입력을 **파일명 리스트**로 본다.

### 3.2 Rule Specification

source snapshot을 FileBlock view로 해석하기 위한 선언형 규칙 집합.

포함 가능 요소:
- tokenizer
- grouping rule
- role derivation rule
- validation rule
- materialization hint

### 3.3 Resolver

Rule Specification을 해석해 FileBlock 후보와 Row 후보를 생성하는 내부 엔진.

### 3.4 FileBlock View

파일 집합을 특정 방식으로 바라본 typed view.

예:
- paired FASTQ
- BAM + BAI
- sample 기준 lane bundle
- somatic tumor-normal bundle
- QC input bundle

### 3.5 Row

FileBlock 내부의 구조화된 입력 레코드.

현재 기준에서 Row는 fanout의 기본 후보이며,
추후 Binding / Runtime 단계에서 execution unit으로 전개될 가능성이 높다. fileciteturn11file1

### 3.6 Role

Row 내부에서 특정 의미를 가진 입력 슬롯.

예:
- R1
- R2
- BAM
- BAI
- TUMOR_R1
- TUMOR_R2
- NORMAL_R1
- NORMAL_R2
- REFERENCE
- KNOWN_SITES

본 문서에서는 가능하면 `column`보다 `role`을 주 용어로 사용한다.
단, CSV/표현 맥락에서는 `column`이라는 표현을 부가적으로 사용할 수 있다.

### 3.7 Materialized FileBlock

특정 source snapshot에 특정 rule spec을 적용해 얻은 고정 결과물.

v0.1에서는 개념적으로만 도입하고, 상세 구조는 다음 문서에서 더 고정한다.

---

## 4. 현재 구현(as-is) 의미론

현재 업로드된 `rule.json`은 아래와 같은 구조다.

- `delimiter`: `["_", "."]`
- `header`: `["R1", "R2"]`
- `rowRules.matchParts`: `[0,1,2,4,5,6]`
- `columnRules.matchParts`: `[3]`
- `sizeRules`: `minSize=100`, `maxSize=1048576` fileciteturn11file3

현재 구현 코드 기준 의미론은 다음과 같다.

### 4.1 입력

resolver 입력은 사실상 **파일명 리스트**다.

### 4.2 Tokenization

각 파일명은 `delimiter` 목록의 모든 값을 공백으로 치환한 뒤 `strings.Fields()` 방식으로 token화된다.

예:
- `sample1_L001_R1_001.fastq.gz`
- delimiters `_`, `.` 적용 후
- `sample1 L001 R1 001 fastq gz`
- tokens = `[sample1, L001, R1, 001, fastq, gz]`

### 4.3 Row Key 생성

`rowRules.matchParts`에 해당하는 token을 추출해 `_`로 join한 값을 row key로 사용한다.

### 4.4 Column Key 생성

`columnRules.matchParts`에 해당하는 token을 추출해 `_`로 join한 값을 column key로 사용한다.

### 4.5 Grouping

동일 row key를 가진 파일들은 하나의 row 후보로 묶인다.

현재 자료구조는 개념적으로 다음과 같다.

- `rowKey -> rowIndex`
- `rowIndex -> { columnKey -> filename }`

### 4.6 Validation

현재 valid/invalid 판정은 role schema가 아니라 **기대 column 수(expectedColCount)** 기반이다.

즉:
- 어떤 row에 column 개수가 expected와 같으면 valid
- 아니면 invalid

### 4.7 Export

valid rows는 CSV로 내보낼 수 있다.
invalid rows는 timestamp 이름의 텍스트 파일로 내보낼 수 있다.

### 4.8 현재 구조의 성격

현재 구현은 pair-end 예제를 처리하는 **초기 recognizer/resolver**로 해석하는 것이 적절하다.

즉 현재 구현은 다음에는 유용하다.

- 파일명 기반 pair grouping 실험
- rule driven grouping의 feasibility 검증
- prototype 수준의 canonical FB 후보 생성

하지만 다음 한계를 가진다.

1. `header=[R1,R2]`가 2-role 예시에 강하게 묶여 있다.
2. validation이 count 기반이라 optional / duplicate / many cardinality를 표현하지 못한다.
3. row index가 encounter order에 의존할 수 있다.
4. 같은 row/column에 여러 파일이 들어오면 overwrite 위험이 있다.
5. invalid 산출물이 구조화되어 있지 않다.
6. materialized 결과와 identity 개념이 아직 약하다.

---

## 5. 문제 진단

현재 pair-end 예시는 유용하지만, 설계의 본질은 pair-end가 아니다.

실제 도메인에서는 다음이 가능해야 한다.

- 2-role paired FASTQ
- 2-role BAM+BAI
- 4-role tumor/normal paired FASTQ
- 5-role variant calling input bundle
- role 하나에 여러 파일이 연결되는 bundle
- optional role이 있는 데이터 타입

따라서 현재 `header=[R1,R2]`는 설계의 본질이 아니라,
**현재 사례 하나를 표현한 초기 스키마**로 봐야 한다.

핵심 전환은 다음이다.

> “컬럼 수를 일반화”하는 것이 아니라,
> “data type별 role schema를 일반화”해야 한다.

즉 시스템 전체는 다양한 role 집합을 가진 여러 FileBlock type을 지원해야 하지만,
**하나의 FileBlock view 내부 schema는 고정**되어야 한다.

---

## 6. 목표(to-be) 의미론

### 6.1 최상위 목표

resolver는 source snapshot을 입력받아,
특정 data type / view type에 대한 **canonical multi-role FileBlock 후보**를 생성해야 한다.

### 6.2 핵심 원칙

1. 시스템 전체는 여러 FileBlock type을 허용해야 한다.
2. FileBlock 하나는 고정된 role schema를 가져야 한다.
3. Row validation은 count가 아니라 schema 만족 여부로 수행해야 한다.
4. rule은 사용자-facing 개념이 아니라 내부 recognizer/resolver spec이어야 한다.
5. 사용자에게는 low-level rule이 아니라 인식 결과와 preview를 보여줘야 한다. fileciteturn11file1
6. resolver 결과는 이후 generation/identity/provenance 문서로 연결될 수 있어야 한다.

### 6.3 예시

#### 예시 A — paired-fastq

roles:
- R1 (required, one)
- R2 (required, one)

#### 예시 B — bam-indexed

roles:
- BAM (required, one)
- BAI (required, one)

#### 예시 C — somatic-dual-fastq

roles:
- TUMOR_R1 (required, one)
- TUMOR_R2 (required, one)
- NORMAL_R1 (required, one)
- NORMAL_R2 (required, one)

#### 예시 D — variant-input-bundle

roles:
- BAM (required, one)
- BAI (required, one)
- REFERENCE (required, one)
- KNOWN_SITES (optional, many)
- TARGET_BED (optional, one)

---

## 7. Rule Resolution 계층의 책임 경계

### 7.1 Resolver가 반드시 책임져야 하는 것

1. source snapshot 입력 정규화
2. tokenization
3. row grouping
4. role derivation
5. schema 기반 validation
6. duplicate / unknown / missing role 판정
7. preview 가능한 구조화 결과 생성
8. materialized 결과로 승격 가능한 중간 결과 생성

### 7.2 Resolver가 아직 직접 책임지지 않는 것

1. 최종 DataBlock packaging
2. 사용자 curated derived FB 구성
3. node input binding 해석
4. runtime materialization (`/in`, `/work`, `/out`)
5. actual execution fanout scheduling
6. lineage graph 저장

### 7.3 Rule의 위치

Rule은 사용자-facing 데이터 모델이 아니라,
시스템 내부 recognizer / resolver가 사용하는 선언형 규칙이다.

사용자는 rule 자체를 직접 다루기보다,
“무슨 타입으로 인식되었는가”, “어떤 row가 생성되었는가”, “어떤 role이 채워졌는가”를 보게 된다. fileciteturn11file1

---

## 8. 권장 개념 모델

### 8.1 현재 `RuleSet`의 개념적 분해

현재 `RuleSet`은 실질적으로 여러 책임을 하나에 담고 있다.

- tokenizer rule
- row grouping rule
- column/role derivation rule
- validation hint
- export hint

v0.1 문서 기준으로는 이를 아래처럼 분해해 이해한다.

### 8.2 권장 상위 구조

#### RuleSpec
- version
- recognizerType
- viewType
- tokenizer
- grouping
- roleDerivation
- validation
- materializationHints

#### TokenizerSpec
- delimiters
- caseNormalization
- trimEmpty
- pathUsage 여부

#### GroupingSpec
- row match parts
- canonical join rule
- optional extra grouping source

#### RoleSchema
- role list
- required 여부
- cardinality (`one`, `many`)
- 설명 텍스트

#### RoleDerivationSpec
- 어떤 token / metadata / extension을 보고 role을 결정하는가
- alias mapping
- unknown role 처리 정책

#### ValidationSpec
- required role 누락 처리
- duplicate collision 정책
- optional role 허용 여부
- size policy
- path/file count policy

#### MaterializationHint
- row ordering rule
- role ordering rule
- invalid output policy
- preview summary 생성 정책

---

## 9. 규범 문장 (Normative Rules)

이 절의 문장은 v0.1 기준의 MUST / SHOULD / MAY 규칙이다.

### 9.1 일반 규칙

1. resolver는 source snapshot과 rule spec을 입력으로 받아야 한다.
2. resolver는 deterministic한 결과를 만들어야 한다.
3. 같은 snapshot과 같은 rule spec이면, 같은 구조적 결과를 생성해야 한다.
4. resolver는 FileBlock 내부 schema를 고정된 role 집합으로 해석해야 한다.
5. resolver는 row를 execution fanout 후보 단위로 유지해야 한다.

### 9.2 Rule 관련 규칙

1. rule은 pair-end 전용으로 설계되어서는 안 된다.
2. rule은 data type별 multi-role schema를 표현할 수 있어야 한다.
3. rule은 사용자가 직접 이해해야 하는 UX 핵심 개념으로 노출되어서는 안 된다.
4. rule은 preview와 explanation을 생성할 수 있을 만큼 설명 가능해야 한다.

### 9.3 Schema 관련 규칙

1. FileBlock 하나는 고정된 role schema를 가져야 한다.
2. role은 의미적 슬롯이어야 하며 단순 column count로만 해석되어서는 안 된다.
3. role에는 최소 `name`, `required`, `cardinality` 개념이 있어야 한다.
4. required role이 누락되면 해당 row는 valid로 판정되어서는 안 된다.
5. optional role 누락은 정책에 따라 허용될 수 있다.

### 9.4 Validation 관련 규칙

1. validation은 단순 expected column count 비교로만 끝나서는 안 된다.
2. duplicate collision 정책은 명시적이어야 한다.
3. unknown role의 처리 정책은 명시적이어야 한다.
4. invalid 결과는 구조화된 이유(reason)와 함께 남겨야 한다.
5. silent overwrite는 기본 정책이 되어서는 안 된다.

### 9.5 Output 관련 규칙

1. resolver 결과는 preview 가능해야 한다.
2. resolver 결과는 이후 materialized FileBlock으로 승격 가능해야 한다.
3. row 표시 순번과 row identity는 장기적으로 분리 가능한 구조여야 한다.
4. invalid 결과는 단순 timestamp 로그 파일만으로 끝나서는 안 된다.

### 9.6 UX 관련 규칙

1. 사용자는 low-level token index 규칙보다 인식 결과를 보게 되어야 한다.
2. 시스템은 “어떤 view로 인식했는지”, “몇 row가 생성되었는지”, “무엇이 누락/충돌했는지”를 설명할 수 있어야 한다.
3. preview 계층은 metadata / recognizer explanation 계층과 자연스럽게 연결되어야 한다.

---

## 10. 역할 스키마(Role Schema)

### 10.1 핵심 관점

현재 `header=[R1,R2]`는 pair-end 예시의 한 표현이다.
앞으로는 이를 보다 일반적인 role schema로 승격한다.

### 10.2 권장 RoleSpec

```json
{
  "name": "R1",
  "required": true,
  "cardinality": "one",
  "description": "forward read of paired FASTQ"
}
```

### 10.3 cardinality

v0.1 기준 권장 cardinality:

- `one`: 정확히 하나 필요 또는 허용
- `many`: 0개 이상 여러 개 허용

추후 필요 시 `one-or-more`, `zero-or-one` 등으로 세분화할 수 있다.

### 10.4 current `header`와의 관계

현재 `header`는 완전히 버리기보다,
초기 구현에서는 `roles[].name`의 간략 표기로 해석할 수 있다.

즉 초기 migration 방향:

- `header: ["R1","R2"]`
- → `roles: [{name:R1,...},{name:R2,...}]`

---

## 11. Row Grouping

### 11.1 현재 방식

현재는 파일명 token 중 일부를 추출해 row key를 만든다.

### 11.2 권장 의미

row grouping은 다음 성질을 가져야 한다.

1. 같은 논리 입력 세트를 묶는 규칙이어야 한다.
2. role 판단과 혼동되지 않아야 한다.
3. deterministic해야 한다.
4. display ordinal과 identity를 분리할 수 있는 기반이 되어야 한다.

### 11.3 row key의 의미

row key는 v0.1에서 **논리 grouping key**다.
이는 장차 row identity의 입력이 될 수 있으나, 아직 최종 rowId 자체로 고정하지는 않는다.

### 11.4 주의점

다음은 이후 검토 포인트다.

- sampleId와 row key의 관계
- lane merge 여부
- technical replicate 처리
- paired 파일 외의 shared reference 파일을 row에 직접 둘지 바깥 binding으로 둘지

---

## 12. Role Derivation

### 12.1 현재 방식

현재는 token index 일부를 추출해 column key를 만든다.

### 12.2 문제

이 방식은 간단하지만 아래 한계를 가진다.

- role alias를 흡수하기 어렵다.
- multi-token role을 다루기 불편하다.
- metadata 기반 role 결정으로 확장하기 어렵다.

### 12.3 권장 방향

role derivation은 장기적으로 다음 source를 사용할 수 있어야 한다.

1. 파일명 token
2. 경로 토큰
3. 샘플시트 / manifest metadata
4. accepted extension metadata
5. recognizer-specific rule

다만 v0.1 개발 범위에서는 우선 **파일명 token 기반 derivation**만 고정한다.

### 12.4 alias 예시

예:
- `R1`, `read1`, `1` → `R1`
- `R2`, `read2`, `2` → `R2`
- `bam` → `BAM`
- `bai` → `BAI`

이 alias 흡수는 초기 구현에서 바로 필요하지 않을 수 있으나,
문서 수준에서는 허용 가능한 확장으로 열어둔다.

---

## 13. Validation Semantics

### 13.1 현재 방식의 한계

현재 valid 판정은 column 개수가 expected와 같은지만 본다.
이 방식은 다음 상황을 구분하지 못한다.

- required role 누락
- optional role 누락
- duplicate role 충돌
- unknown role 존재
- many cardinality 허용

### 13.2 권장 row validation 결과

각 row는 최소 다음 상태 중 하나를 가져야 한다.

- `valid`
- `invalid_missing_required`
- `invalid_duplicate_role`
- `invalid_unknown_role`
- `invalid_cardinality`
- `partial_optional_missing` (정책상 허용 가능)

### 13.3 duplicate collision policy

권장 후보:

- `error`
- `keep-first`
- `keep-last`
- `collect-many`
- `require-merge-step`

v0.1 권장 기본값은 `error`다.

이유:
- 유전체 입력은 silent overwrite 위험이 크다.
- duplicate를 조용히 덮어쓰면 재연성과 디버깅이 무너진다.

### 13.4 sizeRules의 위치

현재 `sizeRules`는 rule 안에 존재하지만 의미가 약하다. fileciteturn11file3

v0.1 해석:
- `sizeRules`는 validation spec의 일부로 본다.
- 다만 초기 구현에서는 파일명만 다루므로 enforcement가 제한될 수 있다.
- 추후 snapshot 입력이 file stat를 포함할 때 실제 적용 가능하다.

---

## 14. Invalid / Preview / Structured Output

### 14.1 Invalid 처리 원칙

invalid는 단순 부수 로그가 아니라,
resolver 산출물의 일부로 구조화되어야 한다.

### 14.2 권장 InvalidRow 구조

- rowLogicalKey
- attemptedViewType
- observedRoles
- missingRoles
- duplicateRoles
- unknownRoles
- sourceFiles
- reasonCode
- humanMessage

### 14.3 Preview 구조

resolver는 사용자-facing preview를 위해 최소 다음 요약을 제공할 수 있어야 한다.

- 어떤 view로 인식했는가
- 총 source file 수
- 생성된 row 수
- valid row 수
- invalid row 수
- role schema
- 대표 invalid reason top N

### 14.4 사용자 설명 예시

- “Paired FASTQ로 인식했습니다.”
- “12개의 row를 생성했습니다.”
- “3개의 row는 R2가 누락되어 invalid 처리되었습니다.”
- “1개의 row는 BAM role이 중복되어 확인이 필요합니다.”

이 방향은 상위 문서의 metadata / preview UX 기준선과 연결된다. fileciteturn11file1

---

## 15. Materialization 경계

### 15.1 왜 별도 경계가 필요한가

resolver 결과는 아직 “해석 결과”이고,
materialized FileBlock은 “고정된 구조화 결과물”이다.

두 개를 구분해야 하는 이유:

1. preview와 persisted result를 분리하기 위해
2. generation과 identity를 안정적으로 다루기 위해
3. Resolved Run Plan에서 참조 가능한 anchor를 만들기 위해

### 15.2 v0.1 기준

본 문서는 materialization의 존재를 전제로 하지만,
다음 상세 문서에서 더 엄밀하게 다룬다.

현재 문서에서 고정하는 최소 원칙:

1. resolver 결과는 materialized 결과로 승격 가능해야 한다.
2. row display ordinal과 logical grouping key는 보존되어야 한다.
3. invalid 결과도 materialized 구조 일부가 되어야 한다.
4. ordering은 deterministic해야 한다.

---

## 16. 현재 `rule.json`에서 권장하는 진화 방향

현재 파일:

```json
{
  "version": "1.0.1",
  "delimiter": ["_", "."],
  "header": ["R1", "R2"],
  "rowRules": { "matchParts": [0,1,2,4,5,6] },
  "columnRules": { "matchParts": [3] },
  "sizeRules": { "minSize": 100, "maxSize": 1048576 }
}
```

권장 개념적 진화:

```json
{
  "version": "1.1",
  "recognizerType": "filename-token",
  "viewType": "paired-fastq",
  "tokenizer": {
    "delimiters": ["_", "."],
    "trimEmpty": true
  },
  "grouping": {
    "matchParts": [0,1,2,4,5,6],
    "joiner": "_"
  },
  "roles": [
    { "name": "R1", "required": true, "cardinality": "one" },
    { "name": "R2", "required": true, "cardinality": "one" }
  ],
  "roleDerivation": {
    "matchParts": [3]
  },
  "validation": {
    "duplicatePolicy": "error",
    "unknownRolePolicy": "invalid",
    "sizeRules": {
      "minSize": 100,
      "maxSize": 1048576
    }
  },
  "materializationHints": {
    "rowOrdering": "lexical-group-key",
    "invalidPolicy": "structured"
  }
}
```

중요한 점은 필드 개수보다도,
**책임이 드러나는 구조**로 바뀌는 것이다.

---

## 17. Go 구조체 및 구현 방향 제안

### 17.1 단계적 리팩터 원칙

초기 구현을 한 번에 갈아엎지 않는다.
현재 pair-end 동작을 유지하면서 점진적으로 일반화한다.

### 17.2 1차 자료구조 승격 방향

현재:
- `Header []string`
- `map[int]map[string]string`

권장 다음 단계:

```go
// 개념 예시

type RoleSpec struct {
    Name        string `json:"name"`
    Required    bool   `json:"required"`
    Cardinality string `json:"cardinality"`
}

type RuleSpec struct {
    Version   string     `json:"version"`
    ViewType  string     `json:"viewType"`
    Delimiter []string   `json:"delimiter"`
    Roles     []RoleSpec `json:"roles"`
    RowRules  RowRules   `json:"rowRules"`
    RoleRules RoleRules  `json:"roleRules"`
    // Validation / materialization hint 등은 후속 추가
}
```

### 17.3 Row cell 승격 방향

현재 `map[string]string`은 one-role-one-file 가정에 강하게 묶여 있다.
장기적으로는 다음 방향이 더 적절하다.

```go
// 개념 예시

type FileRef struct {
    Name string
    Path string
}

type RoleCell struct {
    Files []FileRef
}

type RowCandidate struct {
    LogicalKey string
    Roles      map[string]RoleCell
}
```

이 구조는 `cardinality=many`로 확장 가능하다.

---

## 18. 단계별 개발 계획 (Track A / 본 문서 범위)

이 문서는 실제 개발을 한 번에 다 밀지 않고,
작은 개발단위로 나누기 위한 기준선도 함께 제공한다.

### Phase A-1. Current Semantics Freeze

목표:
- 현재 pair-end 예시 구현의 의미론을 테스트와 문서로 고정

포함:
- tokenizer 규칙 확인
- row key / column key grouping 확인
- valid/invalid count 기반 판정 확인
- CSV export 형태 확인

비포함:
- 구조 일반화
- row identity 안정화
- metadata 연계

성공 기준:
- 현재 동작을 재현하는 fixture / snapshot test 확보
- 현재 한계가 문서에 명시됨

### Phase A-2. Role Schema Introduction

목표:
- `header`를 `roles` 개념으로 승격
- pair-end 외 schema를 문서와 구조체에서 표현 가능하게 함

포함:
- `RoleSpec` 도입
- `viewType` 도입
- count 기반 validator를 schema 기반으로 바꿀 준비

성공 기준:
- 2-role / 4-role / 5-role 예시를 같은 틀로 표현 가능

### Phase A-3. Schema-based Validation

목표:
- expected column count 방식에서 role schema 기반 validation으로 전환

포함:
- missing required
- duplicate collision
- unknown role
- optional role 처리

성공 기준:
- invalid reason이 구조화되어 산출됨

### Phase A-4. Structured Resolver Output

목표:
- preview / invalid / deterministic ordering을 가진 resolver output 도입

포함:
- row candidate 구조화
- invalid structured report
- preview summary

성공 기준:
- 이후 Materialized FileBlock 문서로 자연스럽게 승격 가능

---

## 19. 테스트 전략

### 19.1 최소 fixture 세트

1. 정상 paired FASTQ
2. R2 누락 paired FASTQ
3. duplicate R1 paired FASTQ
4. unknown role token 포함 예시
5. 4-role tumor/normal 예시
6. 5-role bundle 예시

### 19.2 테스트 레벨

- tokenizer unit test
- grouping unit test
- role derivation unit test
- schema validation unit test
- structured invalid output test
- deterministic ordering test

### 19.3 꼭 검증해야 할 것

1. 입력 파일 순서가 달라도 구조적 결과가 동일한가
2. duplicate가 조용히 덮어써지지 않는가
3. optional role이 valid/partial 정책에 맞게 처리되는가
4. role schema가 pair-end에 고정되지 않는가

---

## 20. 예시

### 20.1 현재 pair-end 예시

입력 파일 예시:

- `P001_S1_L001_R1_001.fastq.gz`
- `P001_S1_L001_R2_001.fastq.gz`
- `P002_S1_L001_R1_001.fastq.gz`
- `P002_S1_L001_R2_001.fastq.gz`

해석:
- viewType = paired-fastq
- roles = R1, R2
- rows = P001, P002 각각 1 row

### 20.2 somatic 4-role 예시

입력 파일 예시:

- `Case01_T_R1.fastq.gz`
- `Case01_T_R2.fastq.gz`
- `Case01_N_R1.fastq.gz`
- `Case01_N_R2.fastq.gz`

해석:
- viewType = somatic-dual-fastq
- roles = TUMOR_R1, TUMOR_R2, NORMAL_R1, NORMAL_R2
- rows = Case01 1 row

### 20.3 5-role bundle 예시

입력 파일 예시:

- `Case01.bam`
- `Case01.bai`
- `GRCh38.fa`
- `dbsnp.vcf.gz`
- `targets.bed`

해석:
- viewType = variant-input-bundle
- roles = BAM, BAI, REFERENCE, KNOWN_SITES, TARGET_BED
- rows = Case01 1 row 또는 context bundle 1 row

이 예시는 설계가 `R1/R2`에 고정되면 안 된다는 점을 보여준다.

---

## 21. 오류 / 이벤트 기록 정책 (초안)

본 문서 범위에서 resolver는 최소 다음 수준의 오류/이벤트를 남길 수 있어야 한다.

- rule parse error
- tokenizer config error
- role schema invalid error
- duplicate collision detected
- unknown role detected
- required role missing
- invalid row emitted
- preview summary emitted

이 로그는 운영 로그와 사용자 설명 로그를 나중에 분리할 수 있도록,
reason code 중심으로 설계하는 것이 바람직하다.

---

## 22. 운영 체크리스트 (Track A / 현재 단계)

현재 단계에서 확인할 것:

- pair-end 예시 현재 동작이 문서와 테스트로 고정되었는가
- `header`를 장기적으로 `roles`로 승격해야 한다는 점에 팀 합의가 있는가
- role schema가 2개, 4개, 5개 예시를 모두 담을 수 있는가
- duplicate 기본 정책을 silent overwrite가 아닌 error로 둘 것인가
- invalid를 단순 텍스트 로그가 아니라 구조화 결과로 남길 것인가
- deterministic ordering을 명시할 것인가

---

## 23. 한계와 후속 문서

### 23.1 현재 문서의 한계

이 문서는 resolver의 의미론과 책임 경계를 고정하지만,
아직 아래는 일부러 완전히 확정하지 않는다.

- row identity의 최종 공식
- materialized FileBlock persisted shape
- canonical FB identity
- DataBlock packaging 최종안
- metadata projection 세부
- binding / Resolved Run Plan 연결 상세

### 23.2 다음 문서

다음 상세 문서는 아래가 적절하다.

1. **Materialized FileBlock and Row Identity Specification v0.1**
2. DataBlock Packaging Specification v0.1
3. Binding and Resolved Run Plan Specification v0.1

이 순서가 적절한 이유는,
현재 resolver가 만든 결과를 먼저 “고정된 결과물”로 승격할 구조를 정의해야 이후 binding과 generation을 단단하게 연결할 수 있기 때문이다.

---

## 24. 빠른 시작

현재 바로 할 일은 다음과 같다.

1. 현재 `rule.json` + current code 의미론을 fixture 테스트로 고정한다.
2. `header`를 `roles` 개념으로 승격하는 초안 구조체를 만든다.
3. pair-end 외 4-role / 5-role 예시 rule spec JSON을 샘플로 만든다.
4. validator를 `expectedColCount` 기반에서 schema 기반으로 바꾸는 실험 브랜치를 만든다.
5. invalid structured output 초안을 만든다.

이 다섯 가지는 Track A 1차 마무리를 위한 직접적인 개발 준비 작업이다.

---

## 25. 결론

현재 `rule.json` 기반 구현은 pair-end 파일 묶기 실험으로서는 유효하지만,
이제 tori의 FileBlock resolver는 **pair-end recognizer**가 아니라
**data type별 multi-role typed view resolver**로 승격되어야 한다.

이 문서가 고정하는 핵심은 다음이다.

- FileBlock은 typed view다.
- Row는 fanout 후보 단위다.
- role schema는 pair-end를 넘어서 일반화되어야 한다.
- validation은 count가 아니라 schema 만족 여부로 승격되어야 한다.
- rule은 사용자-facing 개념이 아니라 내부 recognizer/resolver spec이다.
- resolver 결과는 preview 가능하고, 이후 materialized 결과로 승격 가능해야 한다.

즉 Track A의 첫 상세 설계 목표는,
현재 구현을 부정하는 것이 아니라 **현재 구현의 의미를 고정한 뒤, 더 넓은 일반화 방향으로 안전하게 승격시키는 것**이다.

