# Proto Canonicalization Phase 1 Note

## Purpose

이 문서는 Phase 1을 canonical source 선언 단계가 아니라 canonicalization 준비 단계로 고정하기 위한 짧은 note다.
이번 단계의 목적은 local source candidate를 더 보수적으로 다룰 기준을 세우는 것이지, generated code 재생성이나 import rewrite를 시작하는 것이 아니다.

## Current Baseline

- `service`는 transport-agnostic app service boundary다.
- `transport/grpc`는 adapter다.
- `protoio`는 protobuf file I/O boundary다.
- `service`의 protobuf 반환은 현재 허용된 과도기적 app contract다.
- remote RPC surface는 deliberately narrow 하다.
- generated Go code는 source of truth가 아니다.
- current working judgment는 `local candidate preferred for now` 이다.

## Phase 1 Scope

- local source candidate를 working canonical candidate로 다룰 최소 조건을 고정한다.
- source `.proto` ownership과 generated/import 문제를 분리해 둔다.
- external `api-protos` 의존은 기존 boundary 안에서만 유지하고 신규 확산을 막는다.

## Working Canonical Candidate

- 현재 기준에서 [protos/apis.proto](/opt/go/src/github.com/HeaInSeo/tori/protos/apis.proto)는 local source candidate로 가장 자연스럽다.
- 다만 이것은 final ownership 확정이 아니라, Phase 1에서 준비 대상으로 다루는 working canonical candidate다.

## Promotion Conditions

local source candidate를 working canonical candidate로 더 밀어 올리려면 최소 아래 조건이 필요하다.

- local source 변경 승인 경로를 설명할 수 있어야 한다.
- local source가 semantic owner 후보라는 근거를 더 확보해야 한다.
- external usage reality와 ownership evidence를 계속 분리해서 설명할 수 있어야 한다.
- local source 기준이 app/transport/file I/O 경계를 더 악화시키지 않는다는 설명이 가능해야 한다.

## Generated Code Handling In Phase 1

- generated code path 결정, import rewrite, generated code 재생성은 아직 Phase 2 범위다.
- Phase 1에서는 source `.proto`와 generated code를 다른 층으로 유지한다.
- 즉 generated package 사용 현실을 ownership 선언으로 읽지 않는다.

## External Dependency Policy In Phase 1

- external `api-protos` 의존은 즉시 제거하지 않는다.
- 다만 기존 boundary(`service`, `transport/grpc`, `block`, `protoio`) 안에서만 유지한다.
- 신규 external proto 의존 확산은 막고, 새 의존이 필요하면 먼저 local boundary를 통해 흡수할 수 있는지 본다.

## Exit Criteria

- `protos/apis.proto`를 working canonical candidate로 다루는 이유를 문서상 더 일관되게 설명할 수 있어야 한다.
- source ownership 설명과 generated/import 현실 설명이 분리되어 있어야 한다.
- Phase 2가 다룰 일과 아직 다루지 않을 일이 구분되어 있어야 한다.

## Current Working Conclusion

- Phase 1은 canonical source 선언 단계가 아니라 canonicalization 준비 단계다.
- `protos/apis.proto`는 가장 자연스러운 local source candidate이지만 final ownership 확정은 아니다.
- generated code path 정리, import rewrite, 재생성은 아직 Phase 2로 둔다.
- `service` protobuf 반환 제거, remote surface 확장, bootstrap/ingress/Gateway API 문제는 이번 note 범위 밖이다.
