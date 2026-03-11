# tori

## 최소 온보딩 (Phase 0 기준)

### 현재 단계
- 현재 단계: **Phase 0 baseline stabilization**
- 현재 우선순위: **Track A (File/Data structuring) 착수 직전**

### 실행 기준
- 코어 Green baseline 범위: `config`, `db`, `rules`, `block`, `cmd`
- `service` 패키지는 보류된 transport/runtime 영역이므로 현재 코어 baseline에서 제외

### 기본 명령
- `make test-core`: Phase 0 / Track A 기준 코어 Green baseline 확인 명령
- `make lint`: 코어+cmd 범위 fail gate
- `make lint-security`: 코어 보안 점검(report-only)
- `make vuln`: 코어 취약점 점검(report-only)
- `make test`: 전체 상태 확인용 명령 (`service` 보류 영역 포함)

### 설계 문서
- [docs/tori_living_technical_draft_v0.2.md](docs/tori_living_technical_draft_v0.2.md)
- [docs/fileblock_rule_resolution_spec_v0.1.md](docs/fileblock_rule_resolution_spec_v0.1.md)
- [docs/phase_a1_current_semantics_freeze_workplan_v0.1.md](docs/phase_a1_current_semantics_freeze_workplan_v0.1.md)
- [docs/tori_phase0_environment_setup_checklist_v0.1.md](docs/tori_phase0_environment_setup_checklist_v0.1.md)

---

## 레거시 작업 메모

## 설치 필요.
- https://github.com/mattn/go-sqlite3
- DDL 설정 필요. notion 참고.

## dependencies
~~- fsnotify (사용안함.)~~ backup 참조
~~- install : go get -u github.com/fsnotify/fsnotify~~
~~- golang.org/x/sys 이것도 자동으로 설치됨.~~
~~- fsnotify v1.8.0, x/sys v0.26.0~~

## TODO (빨리 정리하고 마무리 하자.)
~~- main 에서 부터 이제 어떻게 다시 시나리오를 만들어 갈지 구상 해야함.~~
- 검색 기능 넣고, grpc 연동 진행.
- 기초 grpc 넣어두고 grpc 프로젝트 만들고 고도화 함. 시작.
- config 에 grpc 관련 부수정보 넣을 것.
- sql 구문 관련해서 보안이나 여러 문제 들에 대해서 한번 체크하고 가자.  
- db 관련해서 테스트 코드 작성해서 최적으로 만들어야 함.  
- 파일명을 읽어드리고 rule 을 읽어드려서 검증하는 루틴 만들어줘야 함.  
- https://github.com/golang/sync/tree/master/singleflight 이거 적용해볼 것을 생각해보자.
- golang.org/x/sync/singleflight
  ~~- grpc 컨테이너 오류 수정해줘야 함.~~  
  ~~- Dockerfile 만들었으며 테스트 진행해야함.~~  
  ~~- 사용자 편의성 생각할 것. exit 을 넣으면 종료 되는데 이게 로그가 올라오면 사라짐.(필요없음)~~
  ~~- Makefile 현재 설정이 내 노트북으로 되어 있는데 이거 확장가능하도록 하자. 우선 순위 낮음. 컨테이너로 하는 방향으로 해서 볼륨 연결해서 pb.go 파일들만 얻는 방식으로 진행하자.~~
- 소스 코드 업데이트 때문에 테스트 코드 다시 작성해야함.
- 데이터 변경이 없는데도 db 가 변하는지 파악해야힘.
- 소스코드 메서드 등 정리 필요. 좀 이름들이 헷갈림.
- lint 처리
- context 정리 해야함. 지금 좀 엉망인 상태임. (중요)
~~- BDD 추가 해주고, ci/cd 정립하자. 장기적으로 고려해야 할듯.~~
- cli 적용하고, grpc 코드 넣고 끝내자.  
- context 이슈 있음. 이거 통일성이 지금 없음. 있는 메서드도 있고 없는 메서드도 있음.  
- 이슈 세밀하게 정검해야함. header 이슈있는데 이거 확인해야함.   pb 파일들이 제대로 만들어지는 철저히 검증해야함. 오류 있을 수 있음.
- DB 지우고 테스트 철저히 진행해야함.  
- 테스트 메서드에 대한 부분 검증하자. 지금 mock 테스트를 해버려서 오류 없이 끝났는데, 실제로 하면 에러나는 것이었다.  
## 수정할 것들.
~~- 일단 굴러가게만 하자.~~  
~~- invalid 해줘야 함.~~  
~~- 특정파일에 동일 규칙의 파일들이 있는지 검사해야 할까?~~  
~~- 하나의 디렉토리에 하나의 블럭만 존재하게 해야 하는가? 복수개도 존재할 수 있도록 해야 하지 않은가?~~  
~~- 디렉토리안에서 여러개의 블럭이 존재할 수 있다고 보는데. 이건 정신이 맑아지면 컨디션 좋을대 살펴보자.~~  
~~- 디렉토리는 이름이 unique 함 따라서 이것은 파일명을 잡는데 중요한 기준점임.~~  
~~- 정신이 없어서 테스트 코드 살펴봐야 한다.~~  
~~- 필터링하는 거 해줘야 함.~~
~~- sql 구문 에러 표시되는거 확인하자.~~
- 폴더에 파일이 없으면 에러 남. 이건 에러로 처리할지 생각할지 아니면 수정할지 생각해봐야 할듯. (일단 이거 빨리 처리하자)
- 서버에는 datablock 만전송한다. 해당 클라이언트가 (이녀석은 서버이면서 클라이언트) ui 클라이언트에는 datablock 에서 필요한 것만 전달.(ui 구성에 필요한것)
- 검색은 서버이면서-클라이언트가 해야 하지 않을까? 검색의 경우는 FileBlockData 에서 block_id 와 column_headers 를 알면될듯하다.
- 해당 정보를 가져와서 user_script 에서 사용자에게 입력된 정보를 매칭시키셔 filepath 를 연결시켜주면 끝.
- 이 녀석은 버전관리만 하고, 데이터 업데이트만 관리하는.
~~- FileBlock 은 간단한 정보이고, 보다 풍부한 정보 path 정보등이 필요한 것이 필요할듯하다.~~ 보안상 가려줄것은 사용자 ui 에 넘기면 안될듯. 이건 생각해보자.  
- 서버이면서-클라이언트는 user_script 를 만들어줄때, tori 한테 물어서 path 가 추가된 user_script 를 만들어주고 이걸 컨테이너에 넣어주면 될듯.
- db 는 스냅샷 역활을 하는 것을 잊으면 안됨. 중요.  
- grpc 에서 인터셉트 알아보기. block_id 같은 경우는 암호화 해야 할거 같은데 일단 생각해보자.  
~~- 로그 정리하기.~~    
~~- apis.go -> api.go 로 이동~~ api.go 에서 grpc 메서드 구현체를 만든다.
- ~~cobra 사용하자.~~ grpc streaming 넣어두자. push 기능 구현 해야함. cli 로 간단히 구현하고 명령을 grpc 로 한다.  
- - 사용자 시나리오 만들어두자. 
~~- 버그는 해결했는데, 메서드 최적화를 좀 해야 할거 같다.~~
- 폴더에 여러 파일들을 넣어보는 테스트를 진행해야 한다.  
- 테스트 파일 수정할 것.  

## 생각해봐야 할 것
~~- 조금더 사용자 친화적으로 할 수 있는 방법이 없는지 생각해보자.(필요없음.)~~ 사용자 명령은 감추어 둘 것임. 어드민으로)    
~~- 디렉토리 관련해서 개발을 시작해야 할 것 같다.~~
- 사용자에게 직접 입력 받는 것은 최소화 하고, 부모 app 에게 명령을 받는 형태로 하는게 좋을 것 같다.
- 최초 사용과 그 이후 사용에 대해서 구별을 자동으로 해주는 방향으로 가자.
- github action 에 대해서 스터디 진행하자.
- 시간날때 lint 최적화 하자.

## update
- 로그 관련 표준 정하자. 생각은 어느정도 정리 됨. 문서화 하자.   


### 참고 하기
https://github.com/charmbracelet/bubbletea/tree/main

### 지우기
https://v.daum.net/v/20250222170330596

###
- Golang 버전 최신 버전으로 맞출 것
- protoc version v26.1
- protoc-gen-go v1.33.0
- protoc-gen-go-grpc v1.3.0

### 확인사항
- gogoproto(https://github.com/cosmos/gogoproto) 사용 안함.
- gogoproto 사용했을때는 ~.pb.go 파일 하나만 사용하면 되었지만 표준방식으로 사용하면 두개를 만들어야 한다.
- 일단 성능적으로 낫다고 하지만, 안정적으로 standard proto 를 사용하기로 함.  
  ~~- pb 파일 생성은 window/linux 둘다 작성 함.~~ (Makefile 만들었음.)

### 설치사항
- protoc 설치
- protoc 의 설치는 https://github.com/protocolbuffers/protobuf/releases/tag/v26.1 이 링크에서 다운
- protoc 의 경우 윈도우는 설치했고, 리눅스 설치해야 함. 버전에 약간 혼동이 있었음.  일단 윈도우 버전하고 비교해보자. 이슈 발생할 듯.
- protoc-gen-go 설치
- 리눅스의 경우(ubutnu) /usr/local/bin/ 에 넣어둠.
- https://github.com/protocolbuffers/protobuf-go/releases/tag/v1.33.0 이 링크에서 설치
- 아래 grpc 설치
- go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
- 일단 go install 을 하면 gopath 기준으로 gopath/bin 에 설치가 된다. 나는 여기서 설치된것을 가져와서 해당 디렉토리에 넣어 두었다.
- 물론, 별도로 위에처럼 링크를 찾아서 해도 된다. https://github.com/grpc/grpc-go/tree/master/cmd/protoc-gen-go-grpc
- 이건 윈도우 리눅스 버전에 대한 부분을 명확히 해야 할듯하다.
- exe 확장자는 윈도우 버전이다.
- 윈도우와 리눅스를 동일하게 작성하게 만들었다.
- grpcurl 은 1.9.1 최신 버전을 설치 하였다.(https://github.com/fullstorydev/grpcurl/releases)
- grpc 다른 proto 파일들도 추가, 리눅스 버전도 해줘야 함.
