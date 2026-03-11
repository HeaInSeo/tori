# Phase A-1 Current Semantics Freeze 작업 문서 v0.1.1
### 문서 버전: v0.1.1 (2026-03-11, 소규모 업데이트/patch)

## 1. 문서 목적

이 문서는 Track A(File/Data 구조화 계층 확정)의 첫 개발단위인 **Phase A-1 Current Semantics Freeze**를 실제 작업 지시가 가능한 수준으로 정의한다.

이 단계의 목적은 다음과 같다.

- 현재 `rule.json`과 구현 코드가 실제로 수행하는 의미론(as-is semantics)을 고정한다.
- 이후 multi-role generalization, schema validator, row identity 분리 같은 구조 변경 전에 **현재 동작을 기준선**으로 남긴다.
- 추후 리팩터링/일반화 시 회귀를 판정할 수 있는 fixture, snapshot test, 샘플 데이터, 보고 형식을 마련한다.

이 문서는 **구현 단계 문서**이지만, 설계 기준선을 어기지 않기 위해 현재 살아 있는 기술문서와 `FileBlock Rule Resolution Specification v0.1.1`의 범위를 따른다.

---

## 2. 배경과 상위 기준선

이 단계는 아래 기준선을 전제로 한다.

- tori는 snapshot 기반 data catalog / binding 계층이다.
- FileBlock은 typed view / grouping 결과다.
- Row는 FileBlock 내부의 실행 fanout 후보 단위다.
- low-level rule은 내부 구현으로 두고, 사용자에게는 인식 결과와 preview를 제공하는 방향이다.
- script는 FileBlock 내부 구조를 직접 알지 않고, 바깥에서 binding 된다.
- Resolved Run Plan은 generation 고정과 row fanout 결과를 포함하는 내부 불변 실행 계획이다. 

현재 살아 있는 기술문서도 FileBlock을 typed view, Row를 fanout 단위, low-level rule을 내부 구현으로 두는 기준을 이미 채택하고 있다. fileciteturn13file0turn13file1

또한 현재 `rule.json`은 아래와 같은 pair-end 예시 구조를 사용한다.

- `delimiter = ["_", "."]`
- `header = ["R1", "R2"]`
- `rowRules.matchParts = [0,1,2,4,5,6]`
- `columnRules.matchParts = [3]`
- `sizeRules.minSize/maxSize` 존재 

즉 현재 구현은 pair-end 중심의 초기 recognizer로 보는 것이 적절하다. fileciteturn13file3

---

## 3. 이번 단계의 정확한 목표

### 3.1 목표

이번 단계의 목표는 **현재 구현의 실제 의미론을 안전하게 얼리는 것**이다.

여기서 “얼린다”는 뜻은 다음과 같다.

1. 현재 구현이 파일명을 어떻게 tokenization 하는지 명시한다.
2. row grouping이 어떻게 되는지 명시한다.
3. column key가 어떻게 만들어지는지 명시한다.
4. valid/invalid 분리가 어떤 규칙으로 되는지 명시한다.
5. CSV export가 어떤 순서/형태로 출력되는지 명시한다.
6. 예외/충돌/한계가 무엇인지 문서와 테스트로 남긴다.

### 3.2 이번 단계의 비목표

이번 단계에서는 아래를 **일부러 하지 않는다**.

- multi-role schema를 실제 구현에 도입하지 않는다.
- `header`를 `roles`로 바꾸지 않는다.
- schema-based validator로 완전 전환하지 않는다.
- row identity / row ordinal 분리를 구현하지 않는다.
- generation 모델을 새로 도입하지 않는다.
- DataBlock/FileBlock canonical identity를 확정하지 않는다.
- binding 문법이나 Resolved Run Plan 구조를 변경하지 않는다.

즉 이 단계는 **일반화 단계가 아니라 현재 동작 보존 단계**다.

---

## 4. 현재 동작(as-is semantics) 고정 범위

현재 구현에서 동결해야 하는 의미론은 아래와 같다.

### 4.1 입력 규칙 로딩

- `LoadRuleSetFromFile(dirPath)`는 디렉토리 유효성을 검사한 뒤, `<dirPath>/rule.json`을 읽어 `RuleSet`으로 로드한다.
- `rule.json`이 없거나 JSON unmarshal에 실패하면 에러를 반환한다.

### 4.2 파일명 tokenization

- `splitFileName(fileName, delimiters)`는 delimiter 배열의 각 문자열을 공백으로 치환한 뒤 `strings.Fields()`로 token 배열을 만든다.
- 이 동작은 중복 구분자, 연속 구분자, 끝 구분자 등을 공백 정리 후 token화한다.

### 4.3 row grouping

- `GroupFiles(fileNames, ruleSet)`는 파일명마다 token 배열을 만들고, `rowRules.matchParts`에 해당하는 token들을 `_`로 join해 `rowKey`를 만든다.
- `rowKey`를 처음 본 순서대로 `rowIdx`를 배정한다.
- 결과 구조는 `map[int]map[string]string` 형태다.

### 4.4 column derivation

- `columnRules.matchParts`에 해당하는 token들을 `_`로 join해 `colKey`를 만든다.
- 한 row 안에서 `colKey -> fileName` 으로 저장된다.
- 같은 row/column key 충돌 시 현재 구조상 overwrite 가능성이 있다.

### 4.5 valid / invalid 분리

- `FilterGroups(resultMap, expectedColCount)`는 row별 column 개수가 `expectedColCount`와 같으면 valid, 아니면 invalid로 분리한다.
- valid 결과는 새 row index를 `0..N-1`로 다시 부여한다.
- invalid는 `[]map[string]string`로 반환된다.

### 4.6 invalid 파일 저장

- `SaveInvalidFiles(invalidRows, outputDir)`는 invalid row에 포함된 파일명을 `invalid_files_YYYYMMDDhhmmss.txt`에 기록한다.
- timestamp 기반 파일명이라 reproducibility anchor라기보다는 현재는 디버깅 산출물로 보는 것이 맞다.

### 4.7 결과 CSV 출력

- `ExportResultsCSV(resultMap, headers, outputDir)`는 `fileblock.csv`를 생성한다.
- header 행은 `Row + headers`로 작성한다.
- 실제 데이터 컬럼 순서는 `resultMap`에서 발견된 모든 `colKey`를 수집한 뒤 `sort.Strings`로 정렬한 값을 따른다.
- 따라서 현재 구현은 `headers`보다 `discovered allKeys` 정렬 결과에 더 강하게 의존한다.

### 4.8 룰 유효성 검사

- `IsValidRuleSet(ruleSet)`는 동일 index가 rowRules / columnRules 양쪽에서 중복 사용되는지 검사한다.
- conflict가 있으면 false를 반환하고 로그를 남긴다.

---

## 5. 이번 단계에서 반드시 드러내야 할 현재 한계

이번 단계는 단지 테스트를 쓰는 단계가 아니라, **현재 구현의 구조적 한계도 명시적으로 고정하는 단계**다.

반드시 기록해야 할 현재 한계는 아래와 같다.

1. `header=["R1","R2"]` 구조는 pair-end 예시에는 맞지만 일반 schema 표현에는 부족하다. fileciteturn13file3
2. row identity가 실제로는 logical identity가 아니라 encounter-order 기반 index다.
3. `FilterGroups()`가 valid row를 다시 재번호 매긴다.
4. `map[string]string` 구조는 role당 단일 파일만 표현 가능하다.
5. duplicate collision은 현재 명시 정책 없이 overwrite 가능성이 있다.
6. invalid 결과는 구조화된 산출물이 아니라 timestamp txt 파일 중심이다.
7. CSV export는 `headers`와 실제 discovered `colKey` 사이에 의미 불일치가 생길 수 있다.

이 한계 기록은 다음 단계의 일반화/리팩터 필요성을 정당화하는 근거가 된다.

---

## 6. 작업 산출물

이번 단계의 필수 산출물은 아래와 같다.

### 6.1 문서 산출물

1. **현재 의미론 정리 문서 보강**
   - tokenization
   - row grouping
   - column derivation
   - valid/invalid 분리
   - CSV export
   - 현재 한계

2. **fixture 설명 문서**
   - 어떤 입력 파일명을 어떤 의도로 구성했는지
   - expected row grouping / column mapping이 무엇인지
   - invalid 케이스가 무엇인지

3. **단계 평가 보고서**
   - 목표 달성 여부
   - 발견된 구조적 문제
   - 다음 Phase A-2로 넘길 이슈

### 6.2 코드 산출물

1. fixture/sample 디렉토리
2. snapshot 또는 table-driven test
3. 최소한의 구조체 승격 초안
   - 단, 실제 의미론 변경 없이 가독성과 이후 확장을 돕는 수준만 허용
4. invalid/CSV 출력 검증 테스트

---

## 7. 세부 작업 항목

### 7.1 작업 1 — 현재 입력 사례 정리

최소 아래 fixture 세트를 준비한다.

#### Fixture Set A — 정상 pair-end 기본 사례

예상 목적:
- `R1`, `R2`가 모두 존재하는 정상 row 생성 검증
- row grouping / column derivation / CSV export 검증

#### Fixture Set B — invalid row 사례

예상 목적:
- `R1`만 있고 `R2`가 없는 경우
- 불완전 row가 invalid로 분리되는지 검증

#### Fixture Set C — tokenization 경계 사례

예상 목적:
- `_`, `.` delimiter 조합
- token 수가 기대보다 적은 경우
- 연속 delimiter 또는 예상 외 파일명 패턴

#### Fixture Set D — duplicate collision 사례

예상 목적:
- 같은 row/column key를 가지는 파일 2개 이상 입력
- 현재 overwrite되는지, 어떤 결과가 남는지 문서와 테스트로 고정

#### Fixture Set E — export ordering 사례

예상 목적:
- `headers`와 실제 discovered key 정렬의 차이 확인
- row 순서/column 순서를 고정

### 7.1.1 현재 진행 상태 (Phase A-1 1차, 2026-03-11 기준)

- [x] Fixture Set A 추가/검증 완료 (정상 pair-end)
- [x] Fixture Set B 추가/검증 완료 (invalid row)
- [x] Fixture Set C 추가/검증 완료 (연속 delimiter tokenization 경계)
- [x] Fixture Set D 추가/검증 완료 (duplicate collision overwrite current behavior 기록)
- [x] Fixture Set E 추가/검증 완료 (ExportResultsCSV current serialization ordering 기록)

주의:
- Set D는 final duplicate policy를 확정한 것이 아니다.
- Set E는 final canonical column policy를 확정한 것이 아니다.
- 두 항목은 모두 known as-is behavior freeze 범위다.

### 7.2 작업 2 — snapshot/table-driven test 작성

최소 아래 테스트 범주를 포함한다.

- `LoadRuleSetFromFile` 정상/오류 테스트
- `splitFileName` tokenization 테스트
- `GroupFiles` grouping 테스트
- `FilterGroups` valid/invalid 분리 테스트
- `SaveInvalidFiles` 출력 테스트
- `ExportResultsCSV` CSV snapshot 테스트
- `IsValidRuleSet` conflict 테스트

테스트는 가능한 한 “현재 의미론을 얼린다”는 관점으로 작성한다.

즉 이상적인 동작을 기대값으로 쓰지 말고, **현재 실제 동작을 기대값으로 고정**해야 한다.

### 7.3 작업 3 — 샘플 데이터와 기대 결과 문서화

각 fixture에 대해 아래를 문서로 남긴다.

- 입력 파일명 목록
- 적용 rule.json
- 예상 rowKey
- 예상 row/column 매핑
- valid / invalid 판정 이유
- CSV 출력 예시
- 현재 구조적 문제 여부

### 7.4 작업 4 — 구조체 승격(의미론 변경 없이)

이번 단계에서 허용되는 구조체 승격은 다음 수준까지다.

- fixture/expected result를 표현하는 테스트 전용 구조체 도입
- 내부 `map[int]map[string]string` 해석을 돕는 보조 타입 도입
- 함수/테스트 이름 정리

이번 단계에서 허용되지 않는 것:

- `header -> roles` 개명
- `map[string]string -> map[string][]FileRef` 의미 변경
- validator semantics 변경
- rowId 도입

즉 구조체 승격은 **코드 이해도와 다음 단계 준비용**이어야 하며, 현재 동작을 바꾸면 안 된다.

### 7.5 작업 5 — 현재 한계 보고서 작성

최종 보고서에는 반드시 아래를 포함한다.

- pair-end 전용 해석의 한계
- multi-role generalization 필요성
- schema validator 필요성
- duplicate collision 정책 부재
- row identity 불안정성
- invalid report 구조화 필요성

---

## 8. 권장 디렉토리 / 파일 구조

아래는 권장안이며, 실제 repo 구조에 맞게 조정 가능하다.

```text
rules/
  fixtures/
    basic_pair/
      rule.json
      input_files.txt
      expected.json
    invalid_missing_r2/
      rule.json
      input_files.txt
      expected.json
    duplicate_collision/
      rule.json
      input_files.txt
      expected.json
    export_ordering/
      rule.json
      input_files.txt
      expected.json
  current_semantics_test.go
  snapshot_test.go
  docs/
    phase_a1_current_semantics_notes.md
    phase_a1_evaluation_report.md
```

핵심은 fixture와 expected result가 코드 바깥에서도 사람이 읽을 수 있게 유지되는 것이다.

---

## 9. 종료 조건

이번 단계는 아래 조건을 만족하면 종료로 본다.

### 9.1 필수 종료 조건

1. 현재 `rule.json` pair-end 예시를 기준으로 정상 fixture가 통과한다. fileciteturn13file3
2. invalid fixture가 통과한다.
3. duplicate collision 현재 동작이 테스트와 문서에 고정된다.
4. CSV export 결과가 snapshot 또는 golden file로 고정된다.
5. 현재 한계 목록이 문서로 정리된다.
6. 구조체 승격이 있었다면, 의미론을 바꾸지 않았음이 확인된다.
7. 다음 단계에서 바꿀 영역과 이번 단계에서 동결한 영역이 분리되어 기록된다.

### 9.2 보너스 종료 조건

- 현재 fixture를 바탕으로 Phase A-2 generalization을 위한 샘플 multi-role 예시(구현 없이 문서만)까지 남기면 좋다.

### 9.3 종료 평가 (Phase A-1 1차)

- 판정: **Phase A-1 1차 완료**
- 근거:
1. A/B/C/D/E fixture+freeze test가 모두 추가되어 현재 의미론의 핵심 경계가 고정되었다.
2. duplicate collision / export ordering에 대해 current behavior와 final policy를 분리해 기록했다.
3. 의미론 변경 없이 테스트/fixture 중심으로 기준선을 확보했다.

- 아직 열린 항목(Phase A-2 입력):
1. duplicate collision final policy 정의
2. multi-role schema generalization
3. canonical column policy(헤더/role/serialization 일관성) 정의

---

## 10. 평가 질문

단계 종료 시 아래 질문에 답한다.

1. 현재 구현 의미론이 충분히 보존되었는가?
2. 테스트가 이상적인 설계가 아니라 실제 현재 동작을 고정하고 있는가?
3. 어떤 동작이 의도였고, 어떤 동작이 우연한 현재 구현 결과인가?
4. 다음 단계에서 가장 먼저 바꿔야 할 구조는 무엇인가?
5. 이번 단계에서 도입한 보조 구조체가 다음 리팩터를 방해하지 않는가?

---

## 11. 다음 단계로 넘길 입력

Phase A-1 완료 후, 다음 Phase A-2로 넘길 입력은 아래와 같다.

- frozen current semantics test set
- fixture corpus
- current limitations report
- candidate generalized role schema examples
- duplicate/invalid/export ordering 관찰 결과

Phase A-2는 이 입력을 바탕으로 **pair-end 전용 해석을 multi-role typed schema로 일반화하는 설계/구현 준비 단계**로 넘어간다.

---

## 12. 에이전트 작업 지시용 요약

아래는 다른 에이전트에게 그대로 넘길 수 있는 요약이다.

### 작업 목표
현재 rule resolver의 동작을 바꾸지 말고, 현재 의미론을 fixture/test/sample/doc으로 고정하라.

### 절대 하지 말 것
- 의미론 변경
- role schema 도입
- validator 일반화
- row identity redesign
- 구조 리팩터를 핑계로 결과 변화 유발

### 반드시 할 것
- 정상/invalid/duplicate/export-ordering fixture 작성
- snapshot 또는 golden test 작성
- 현재 동작과 현재 한계 문서화
- 다음 단계로 넘길 이슈 목록 정리

### 최종 보고에 포함할 것
- 무엇을 고정했는가
- 어떤 현재 한계를 확인했는가
- 어떤 부분은 일부러 안 바꿨는가
- 다음 단계에서 무엇을 다룰 것인가

---

## 13. 현재 시점 결론

Phase A-1 Current Semantics Freeze는 작아 보이지만, 이후 Track A 전체의 안전장치 역할을 한다.

이 단계를 건너뛰고 일반화로 바로 가면, 아래 위험이 커진다.

- 현재 pair-end 사례 회귀를 놓칠 수 있다.
- 우연한 현재 동작과 의도된 동작을 구분하지 못한다.
- multi-role generalization 후 어떤 변화가 실제 개선인지 판단하기 어렵다.
- row identity, invalid handling, export ordering 같은 세부 동작이 나중에 더 큰 혼란으로 번질 수 있다.

따라서 이 단계의 목표는 “멋지게 개선”이 아니라, **현재를 정확히 붙잡는 것**이다.
