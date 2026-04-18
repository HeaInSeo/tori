# Proto Contract Ownership

## 목적

이 문서는 현재 tori에서 proto 관련 ownership을 작은 범위로 고정하기 위한 메모다.
이번 단계의 목적은 `api-protos`를 바로 흡수하는 것이 아니라, 그 전에 무엇이 source contract이고 무엇이 app/transport boundary인지 먼저 구분하는 것이다.

현재 상태 메모:

- `datablock` 계열 active code path 는 이미 local generated path 로 전환되었다.
- `syncfolders` source proto도 local canonical path 아래에 존재하지만, 현재는 local-only/deferred 의미로 읽는다.
- 현재 `tori` 의 local canonical source 는 `protos/ichthys/v1` 아래에 놓여 있다.
- 이전 `protos/apis.proto` 는 legacy duplicate 였고, 현재 저장소에서는 제거되었다.
- `tori` active code path 기준으로는 external `api-protos` generated import 가 제거되었다.

## 구분해야 할 4개 층

### 1. source `.proto`

- protobuf schema의 원본 정의다.
- 장기적으로 canonical ownership 후보가 될 수 있는 층이다.
- 현재 저장소 안의 `protos/apis.proto` 흔적과 외부 `api-protos` generated import가 함께 존재하므로, source ownership은 아직 최종 고정 상태가 아니다.

### 2. generated code

- `.proto`로부터 생성된 Go 타입과 service stub 계층이다.
- generated code는 source contract의 파생물이지, ownership 자체를 자동으로 결정하지는 않는다.
- 현재 tori는 외부 `api-protos` generated code에 의존할 수 있지만, 그 사실만으로 외부 저장소가 최종 canonical owner라고 확정되는 것은 아니다.

### 3. app contract

- 현재 `service` 계층이 local/in-process와 transport adapter가 함께 사용하는 app service contract다.
- 이 층은 transport lifecycle이나 ingress 정책을 소유하지 않는다.
- 현재 baseline에서는 일부 protobuf 타입 반환이 허용되어 있다. 이는 과도기적 app contract 허용 상태이지, 최종 protobuf-neutral 내부 DTO 목표가 완료되었다는 뜻은 아니다.

### 4. transport contract

- `transport/grpc`가 해석하는 RPC request/response shape다.
- transport contract는 network exposure와 adapter translation을 위한 층이다.
- 이 층은 app semantics를 호출하지만, app/service ownership을 대체하지 않는다.

## Current Canonical Candidate

현재 시점의 final decision은 아직 아니다.
다만 현재 문서 기준선에서는 다음을 **current working assumption**으로 둔다.

- canonical source `.proto`의 우선 후보는 generated Go package가 아니라 source `.proto` 자체다.
- 그리고 현재 저장소 안에 남아 있는 `protos/apis.proto`는 "무시된 흔적"이 아니라, source ownership을 다시 확인해야 할 local candidate로 취급한다.
- 반대로 현재 외부 `api-protos` generated import가 널리 사용되고 있다는 사실은, transport/app 구현이 그 generated package에 의존하고 있음을 보여줄 뿐, canonical source ownership을 자동으로 고정하지는 않는다.

즉 현재 preferred direction은 "`source .proto` ownership을 먼저 고정하고, generated package path 문제는 그 다음에 따라오게 둔다"는 것이다.

## Canonical Source Candidate Evaluation Criteria

canonical source `.proto` 후보를 볼 때는 아래 체크리스트를 먼저 적용한다.

- change ownership
  - 해당 `.proto`를 바꾸는 책임과 승인 경로가 어디에 있는지 본다.
  - 자주 import된 generated package인지보다, schema 변경을 실제로 소유하고 조정하는 곳이 어디인지가 더 중요하다.
- meaning ownership
  - 메시지/서비스 의미를 설명하고 안정화하는 책임이 어디에 있는지 본다.
  - 단순 배포 편의나 코드 재사용보다, contract 의미를 누가 정의하는지가 우선이다.
- import reality != ownership
  - 현재 어떤 generated package가 많이 import되고 있는지는 dependency reality일 뿐이다.
  - import 빈도나 현재 결합도만으로 canonical ownership을 확정하지 않는다.
- generated convenience != source authority
  - generated package가 편리하거나 이미 널리 쓰인다는 사실은 source authority의 증거가 아니다.
  - source authority는 generated 산출물 바깥의 `.proto` ownership에서 판단한다.
- boundary preservation
  - 어떤 후보가 app contract, transport contract, file I/O boundary를 덜 흔들고 유지하게 하는지 본다.
  - canonical 판단은 편의보다 경계 보존에 유리한 쪽을 우선한다.
- ownership != migration cost
  - 나중에 옮기기 쉬운지, import 교체 비용이 작은지는 migration cost 문제다.
  - migration cost는 참고사항일 뿐이고, ownership 자체를 대신 결정하지 않는다.

## Current Application Of The Criteria

현재 저장소에 위 기준을 적용하면, final decision이 아니라 다음 해석을 **working application note**로 둔다.

- `protos/apis.proto`는 local canonical candidate로 재검토 대상이다.
  - 이유: local source candidate로서 change ownership과 meaning ownership을 다시 점검할 수 있는 위치이기 때문이다.
  - 다만 이것만으로 이미 canonical source로 최종 확정되었다고 말하지는 않는다.
- 외부 `api-protos` generated import는 dependency reality일 뿐 canonical ownership 자동 증거는 아니다.
  - 현재 `service`, `block`, `transport/grpc`, `protoio`가 그 generated package를 사용하고 있어도, 이는 import reality와 generated convenience를 보여줄 뿐이다.
  - 따라서 external generated path의 현재 사용량만으로 source authority를 판정하지 않는다.
- canonical 판단의 1차 기준은 generated package가 아니라 source `.proto`다.
  - 즉 현재 단계에서는 "`어느 generated path가 더 널리 쓰이는가`"보다 "`어느 source `.proto`가 change/meaning ownership을 더 설명하는가`"를 먼저 본다.

## Evidence Checklist For Canonical Source Ownership

아래 항목은 canonical source ownership 판정을 보조하기 위한 evidence checklist다.
이 체크리스트는 final decision을 자동으로 내리기 위한 것이 아니라, 어떤 근거가 더 필요한지 좁혀 보기 위한 evaluation aid다.

- change approval path
  - 해당 `.proto` 변경이 실제로 어떤 저장소와 어떤 승인 경로를 통해 검토/승인되는지 확인한다.
  - 변경 승인 경로가 불명확하면 canonical ownership evidence도 약한 것으로 본다.
- meaning stewardship
  - 메시지와 서비스 의미를 누가 설명하고, 누가 semantic drift를 막고, 누가 contract 변경의 의미를 책임지는지 확인한다.
  - 단순 유지보수 편의보다 meaning stewardship evidence를 우선 본다.
- boundary preservation impact
  - 어떤 후보를 canonical source로 볼 때 app contract, transport contract, file I/O boundary가 덜 흔들리는지 확인한다.
  - 경계를 불필요하게 섞는 후보는 evidence가 더 강해야 한다.
- cross-repo dependency shape
  - 실제 의존이 단일 저장소 중심인지, 다중 저장소에 걸친 shared contract인지, 혹은 한쪽이 파생 소비자인지 확인한다.
  - 이 항목은 dependency shape를 이해하기 위한 것이지, import 수만 세어 ownership을 확정하기 위한 것이 아니다.
- source-first consistency
  - 판단이 generated package 경로가 아니라 source `.proto` 기준으로 일관되게 설명되는지 확인한다.
  - generated convenience가 source authority를 가리는 경우에는 evidence를 다시 분리해서 본다.
- migration separation
  - ownership 판단과 migration 계획을 분리해서 서술할 수 있는지 확인한다.
  - "옮기기 어려우니 현재 owner로 본다" 같은 식의 논리를 피하고, ownership evidence와 migration cost를 따로 기록한다.

## How To Use This Checklist

- 먼저 candidate마다 위 evidence 항목을 한 줄씩 채운다.
- 그 다음 change ownership, meaning stewardship, source-first consistency가 가장 선명한 후보가 있는지 본다.
- boundary preservation impact와 cross-repo dependency shape는 보강 evidence로 사용하되, import usage reality를 ownership evidence와 같은 층으로 취급하지 않는다.
- migration separation 항목은 "지금 당장 옮길 수 있는가"가 아니라 "ownership 판단이 migration 비용 논리에 오염되지 않았는가"를 확인하는 데 사용한다.
- evidence가 비어 있거나 약한 경우에는 canonical owner를 확정하지 않고, working assumption 상태를 유지하는 편이 맞다.

## Candidate Comparison Note

이 비교 메모는 final decision이 아니라 현재 evidence 상태를 나란히 놓아 보는 working comparison note다.

### Candidate A: local `tori` source

- change approval path
  - local source path로서 저장소 내부 변경 경로를 직접 점검할 수 있다는 점은 보인다.
  - 다만 semantic owner evidence를 더 넓은 서비스 경계 기준으로 계속 보강할 필요는 남아 있다.
- meaning stewardship
  - local source 후보이므로 의미 소유권을 local 문서/설계와 연결해 재검토할 여지가 있다.
  - 다만 현재 이 파일이 실제 semantic owner라는 근거는 아직 비어 있다.
- boundary preservation impact
  - local source 기준으로 ownership을 정리하면 app/transport/file I/O 경계를 repo 안에서 설명하기 쉬워진다.
  - `datablock` 경로는 이미 local generated path 로 정리되었고, 남은 경계는 같은 원칙으로 후속 정리 대상이다.
- cross-repo dependency shape
  - local source candidate로 두면 외부 generated dependency와 local source ownership이 분리된 형태가 된다.
  - 이 분리가 실제 shared contract 구조와 맞는지는 추가 evidence가 필요하다.
- source-first consistency
  - source `.proto`를 기준으로 ownership을 본다는 현재 원칙과는 잘 맞는다.
  - 다만 local source가 canonical이라고 말할 만큼의 추가 근거는 아직 없다.
- migration separation
  - ownership 판단과 import migration을 분리해서 생각하기 쉽다.
  - 다만 실제 migration 부담은 아직 계산하지 않았고, 그것을 ownership 증거로 쓰지도 않는다.

### Candidate B: external `api-protos` source

- change approval path
  - 현재 external generated package 사용 현실은 보이지만, external source 자체의 승인 경로가 tori 기준에서 어떻게 연결되는지는 아직 문서상 비어 있다.
  - 따라서 change approval path evidence는 현재로서는 충분히 드러나지 않는다.
- meaning stewardship
  - external source가 의미 소유권을 실제로 책임지는지, 아니면 generated distribution 역할에 가까운지는 아직 분명하지 않다.
  - 현재 import usage만으로 meaning stewardship을 강하게 읽을 수는 없다.
- boundary preservation impact
  - external source를 canonical로 볼 경우 cross-repo contract 설명은 단순해질 수 있다.
  - 하지만 tori 내부 app/transport/file I/O boundary 설명이 더 선명해지는지는 별도 검토가 필요하다.
- cross-repo dependency shape
  - 현재 dependency reality상 external generated package가 실제 소비 지점에 널리 연결돼 있다는 점은 보인다.
  - 그러나 이 사실은 cross-repo dependency shape evidence일 뿐, ownership 결론으로 바로 점프할 수는 없다.
- source-first consistency
  - external generated import usage가 많다는 사실만으로는 source-first consistency evidence가 되지 않는다.
  - external source 자체가 canonical source인지 설명하려면 generated path가 아니라 source ownership 근거가 더 필요하다.
- migration separation
  - 현 상태를 유지하는 것이 migration cost 측면에서 편해 보일 수는 있다.
  - 그러나 그 편의는 ownership evidence가 아니라 migration consideration으로만 남겨 두어야 한다.

## Interim Reading

- 현재 working comparison 기준으로는 local `tori` source path 가 canonical candidate로 우세하다.
- 동시에 external `api-protos` generated usage가 많다는 사실만으로 canonical ownership을 확정할 수는 없다.
- Candidate A는 source-first consistency 측면에서 설명하기 쉬운 부분이 있고, Candidate B는 dependency reality가 더 눈에 띈다.
- 하지만 두 후보 모두 change approval path와 meaning stewardship evidence는 아직 충분히 문서화되어 있지 않다.
- 따라서 현재 단계의 읽기는 "external usage가 많으니 외부 owner로 확정"도 아니고, "`protos/apis.proto`가 있으니 local owner로 확정"도 아니다.

## Open Evidence Gaps

canonical source ownership 판정 전에 아직 확인이 비어 있는 근거는 아래 네 가지다.

- source change approval path
  - 어느 저장소/경로에서 source `.proto` 변경이 실제로 승인되는지 확인이 필요하다.
- semantic owner evidence
  - 메시지와 서비스 의미를 누가 설명하고 drift를 막는지 확인이 필요하다.
- shared-by-design vs reused-in-practice
  - 이 contract가 원래부터 cross-repo shared source인지, 아니면 현재는 외부 산출물을 재사용하는 상태인지 구분이 필요하다.
- boundary preservation evidence
  - 어느 후보가 app/transport/file I/O 경계를 덜 흔드는지에 대한 근거가 더 필요하다.

## Current Use Of These Gaps

- 현재 비교 메모는 위 gap이 남아 있다는 전제 아래에서만 읽어야 한다.
- 즉 candidate 비교는 "지금 보이는 evidence"를 나란히 놓은 것이고, 위 gap을 메우기 전에는 ownership final decision으로 넘어가지 않는다.
- 특히 external generated usage 현실은 shared-by-design evidence와 같지 않고, local source 존재만으로 semantic owner evidence가 채워진 것도 아니다.

## Working Candidate Judgment

- 현재 evidence로 local `tori` source 쪽에 유리한 점은, source-first consistency와 local boundary 설명 측면에서 더 자연스럽다는 것이다.
- current dependency reality로는 external `api-protos` source 쪽에 실제 generated usage가 넓게 보이지만, 이것은 ownership evidence라기보다 소비 현실에 가깝다.
- final decision을 막는 핵심 evidence는 아직 change approval path, semantic owner evidence, shared-by-design 여부, boundary preservation evidence 쪽에 남아 있다.
- 따라서 현재 working conclusion은 `local candidate preferred for now` 이지만, 이는 final ownership 확정이 아니라 현재 evidence만 놓고 본 좁은 judgment note다.

## Generated Code Handling Rule

- generated Go code는 source of truth가 아니다.
- generated Go code는 `.proto`에서 파생되는 build/toolchain 산출물로 취급한다.
- 따라서 "어디의 generated package를 import하고 있는가"와 "canonical source `.proto` owner가 어디인가"는 같은 질문으로 다루지 않는다.
- source `.proto` ownership이 아직 흔들리는 상태에서 generated package path만 보고 canonical owner를 확정했다고 해석하지 않는다.

실무 규칙:

- source contract 판단은 `.proto` 기준으로 한다.
- generated code 경로 판단은 build/toolchain 및 import migration 기준으로 따로 본다.
- 두 문제를 한 번에 묶어 결론 내리지 않는다.

## 현재 허용 상태

- 외부 `api-protos` generated code 의존은 현재 baseline에서 허용된다.
- `service`는 app boundary로 유지되며, 현재는 일부 protobuf 타입을 직접 다룰 수 있다.
- `transport/grpc`는 protobuf RPC surface를 해석하는 adapter로 남는다.
- `protoio`는 protobuf file I/O boundary를 담당한다.

## Transitional Import Policy

현재는 전환기이므로 외부 `api-protos` import를 즉시 제거하지 않는다.

기존 import 허용 조건:

- 이미 현재 app/transport/file I/O 경계에서 사용 중인 generated package import는 당분간 유지 가능하다.
- 단, 이 import가 canonical ownership을 이미 결정한 것처럼 문서나 코드 주석에서 해석되면 안 된다.

신규 proto 관련 의존 추가 시 우선 방향:

- 신규 코드는 먼저 "이 의존이 source `.proto` ownership 문제인지, generated package 사용 문제인지"를 분리해서 판단해야 한다.
- 가능하면 기존 app boundary와 transport boundary를 더 흐리지 않는 방향을 우선한다.
- 같은 의미의 새로운 generated package 경로를 병렬로 더 늘리는 식의 추가 의존은 피한다.

기존 import를 언제/왜 바꾸는가:

- 기존 import는 `api-protos` 흡수 구현이 필요해서가 아니라, ownership 기준선이 먼저 고정된 뒤 그 결정에 맞춰 정리할 때 바꾼다.
- 즉 import 교체의 최소 원칙은 "ownership 결정의 후속 작업"이지, ownership 결정을 import 교체로 대신하지 않는다는 점이다.

## Phase 0 Import Guardrail

- 현재 external `api-protos` import는 이미 사용 중인 경계 안에서만 유지한다.
- 당분간 신규 proto 관련 external import는 기존 사용 패키지(`service`, `transport/grpc`, `block`, `protoio`) 바깥으로 확산시키지 않는 방향을 기본으로 둔다.
- 새 의존이 필요해 보일 때는 먼저 local boundary package를 통해 흡수할 수 있는지 검토한다.
- 이 가드레일의 목적은 즉시 import 제거가 아니라, ownership 결정 전 dependency surface가 더 넓어지는 것을 막는 데 있다.

## 중요한 정리

- 외부 `api-protos` 의존은 현재 허용 상태다.
- 그러나 이것이 최종 canonical ownership을 자동으로 의미하지는 않는다.
- canonical ownership은 source `.proto`, generated code 경로, app contract, transport contract를 분리해서 판단해야 한다.

## App Contract vs Transport Contract Boundary

- 현재 `service`의 protobuf 반환은 허용된 과도기적 app contract다.
- 이 상태는 `service` 책임이 transport adapter 책임으로 재분류되었다는 뜻이 아니다.
- 즉 protobuf 타입을 사용하더라도, 현재 `service`는 여전히 app/service boundary로 본다.
- 동시에 이 상태가 최종형이라는 뜻도 아니다.
- 이후 더 엄격한 contract 분리나 protobuf-neutral 내부 DTO 전환은 후속 선택지로 남아 있다.

## Explicit Non-Decisions

이번 문서는 아직 아래를 결정하지 않는다.

- 최종 canonical `.proto` 위치 확정
- generated code 최종 디렉터리/패키지 경로 확정
- `api-protos` 실제 흡수 방식
- protobuf-neutral 내부 DTO 전환 여부
- generated code 재생성 방식과 toolchain 절차 확정
- 기존 import 교체의 실제 실행 순서

## 다음 단계

다음 가장 작은 단계는 `api-protos` 흡수 구현이 아니다.
그 전에 먼저 아래를 문서 기준선으로 고정해야 한다.

- source `.proto`의 canonical owner가 어디인지
- generated code를 어떤 저장소/경로에서 관리할지
- 현재 `service`의 app contract와 `transport/grpc`의 transport contract를 어디까지 분리된 층으로 볼지

즉, 다음 단계는 흡수 설계 전에 ownership을 먼저 고정하는 것이다.
