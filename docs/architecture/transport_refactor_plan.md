# Transport Refactor Plan

## 목표

이번 단계의 목표는 최종 네트워크 구조를 확정하는 것이 아니라, transport 경계를 먼저 고정하는 것이다.

구체 목표:

- `service`를 transport-agnostic 하게 유지
- gRPC를 adapter로 분리
- local/in-process 호출을 정상 경로로 인정
- 향후 Kubernetes, Gateway API, Istio 변경이 core를 흔들지 않게 만들기

## 단계별 작업 계획

### 1. 현재 경계 기록

- 현재 `service`, `cmd`, `block`, `db`의 역할을 재정리
- gRPC/server 흔적과 local/in-process 호출 경로를 문서화

### 2. service 인터페이스 명시

- `cmd`와 gRPC adapter가 함께 사용할 최소 service interface를 도입
- concrete type 의존보다 interface 의존을 우선

### 3. gRPC adapter 분리

- `service` 패키지 안의 gRPC server adapter 흔적을 `transport/grpc`로 이동
- adapter는 request/response 변환과 service 호출만 담당

### 4. local/in-process 경로 유지

- `cmd`는 별도 transport 없이 service 인터페이스를 직접 호출
- local adapter는 현재 단계에서 “in-process direct call”을 기준선으로 둠

### 5. 후속 단계 준비

- proto 흡수 및 generated code 경로 정리는 별도 작은 단계로 분리
- `block`의 proto helper 직접 의존은 후속 경계 정리 대상으로 남김

## 작은 diff 기준의 실제 정리안

이번 단계에서 허용하는 실제 수정은 다음으로 제한한다.

- 문서 2개 추가
- `service`에 transport-agnostic interface 추가
- gRPC adapter를 `transport/grpc` 패키지로 분리
- `cmd`가 concrete type 대신 service interface에 붙도록 조정

이번 단계에서 하지 않는 것:

- proto 파일 재배치
- generated code 재생성
- syncfolders gRPC 구현 확장
- 대규모 패키지 이동
- transport/storage DTO 전면 재설계

## 위험요소

- `service` 테스트는 이미 오래된 transport contract 잔존물과 섞여 있어, 이번 단계만으로 전체 green은 되지 않을 수 있다.
- `block`이 외부 generated helper에 직접 붙어 있어 transport 경계가 완전히 닫히지는 않는다.
- 현재 `DataBlock` 반환 타입이 proto message이므로, service가 완전히 protobuf-neutral 하다고 말할 수는 없다.

## 비목표

- gRPC 제거
- Istio 전제 구조 도입
- Gateway API 전제 코드 도입
- production-grade server lifecycle 완성
- mTLS 정책 확정
- syncfolders/DataBlock 전체 API 재설계

## 이번 단계 성공 기준

- `service`가 `google.golang.org/grpc`를 import하지 않는다.
- gRPC adapter 코드가 `transport/grpc`에 분리된다.
- `cmd`가 service interface 경유 local/in-process 경로를 사용한다.
- 문서에 왜 이렇게 하는지와 아직 확정하지 않는 범위가 남는다.
