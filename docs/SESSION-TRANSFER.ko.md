# Cursor 터미널에서 대화 수동 이전하기

이 브랜치는 ccam 원본(MIT, Copyright 2026 jhyunwoo)의 독립 로그인 기능에 안전한 수동 세션 복사를 추가합니다. 원본 LICENSE를 유지합니다. 새 명령은 사용량을 조회하거나 제한 도달 시 계정을 자동으로 바꾸지 않습니다. 원본에 있던 `auto_switch`, `--fallback` 등은 이 작업에서 추가·수정하지 않았습니다.

## 시작

Go 1.26 이상에서 저장소 루트에서 빌드합니다.

```sh
go build -o ./ccam ./cmd/ccam
./ccam add personal --no-group
./ccam add work --no-group
```

각 계정의 로그인은 사용자가 별도로 완료합니다. 기존 계정은 다시 만들 필요가 없습니다. 독립적인 history를 사용하세요. history 공유 그룹의 심볼릭 링크는 이번 MVP에서 거부합니다. 원본 자동 전환 설정을 활성화하지 마세요.

Cursor의 통합 터미널에서 원래 프로젝트 폴더로 이동해 `ccam personal`로 작업합니다. 대화를 이전하려면 **양쪽 프로필에서 실행 중인 Claude 세션을 모두 종료**한 뒤 다음 순서로 진행합니다. 아래 `SESSION_UUID`와 `PROJECT_FOLDER`는 list 출력으로 바꾸세요.

```sh
ccam session list personal
ccam --dry-run session transfer personal work SESSION_UUID --project-folder PROJECT_FOLDER
ccam session transfer personal work SESSION_UUID --project-folder PROJECT_FOLDER
ccam work --resume SESSION_UUID
```

빌드한 바이너리가 PATH에 없으면 `ccam` 대신 그 파일의 절대 경로를 사용합니다. 프로젝트 폴더를 옮기거나 프로젝트 이름을 추측하지 말고 list에 표시된 정확한 폴더명을 사용합니다. 목록의 `--project-folder "..."`를 그대로 붙여 넣을 수 있습니다.

## 보존 및 검증

- 원본 세션을 삭제하거나 수정하지 않습니다.
- `<UUID>.jsonl` 및 `<UUID>/` 하위 파일만 복사합니다. `.credentials.json`, `.claude.json`, Keychain, 설정, 전체 history 인덱스는 복사하지 않습니다.
- 백업은 ccam 상태 폴더의 `session-backups/session-...`에 보관하고 경로를 출력합니다. 디렉터리 0700, 파일 0600으로 생성합니다.
- JSONL 구문과 기록된 sessionId를 검사합니다. SHA-256 manifest로 백업 및 대상 파일의 일치를 확인합니다. 이 검증은 파일 무결성 검사이며 Claude 서버의 재개 허용을 보장하지 않습니다.
- 대상 세션이 이미 있으면 덮어쓰지 않습니다. 복사 도중 오류가 나면 이번 호출이 만든 대상 파일을 정리하고 백업을 남깁니다. 메인 transcript는 동기화된 임시 파일에서 하드 링크로 마지막에 공개합니다.
- `--dry-run`은 세션/백업을 쓰지 않지만 동시 명령을 막는 잠금 파일을 만들 수 있습니다.

```sh
ccam session verify /절대/백업/경로
```

백업에서 수동 복구하려면 먼저 verify를 실행하고 Claude를 종료한 상태에서 백업의 `projects/PROJECT_FOLDER/UUID.jsonl`과 `projects/PROJECT_FOLDER/UUID/`(있을 때)를 원하는 프로필의 동일한 상대 경로로 복사합니다. 기존 대상 파일을 덮어쓰지 마세요. 자동 복구 명령은 이번 MVP에 포함하지 않았습니다.

## 범위와 제한

파일 합계 최대 128 MiB입니다. 프로필 루트는 실제 경로로 해석하며 그 아래의 심볼릭 링크, 특수 파일, 잘못된 UUID/경로, 손상된 JSONL은 거부합니다. 일반 파일 시스템의 하드 링크 지원이 필요합니다. 실행 중 여부 검사와 ccam 잠금은 다른 프로그램이 새로 Claude를 시작하는 것을 막지 못합니다. 이전이 끝날 때까지 두 프로필을 사용하지 마세요. 프로세스 강제 종료나 전원 장애 후 남은 부분 복사본은 자동으로 정리하지 않습니다. 백업을 확인하고 수동으로 정리해야 합니다.

대화만 이전합니다. 프로젝트 소스 코드, 외부 이미지/첨부, 전역 file-history, 메모리, MCP 설정·인증, 도구 권한을 이식하지 않습니다. 파일 되돌리기 기능이나 외부 첨부 접근까지 동일하게 복구된다고 보장하지 않습니다. 다른 조직의 서명된 thinking 등으로 Claude가 재개를 거부하면 원래 계정에서 계속하세요. 서명이나 기록을 제거해 제한을 우회하지 않습니다.

공식 문서: [프로필별 로그인](https://code.claude.com/docs/en/authentication#log-in-with-multiple-accounts), [세션 재개](https://code.claude.com/docs/en/sessions). 실제 계정 간 재개는 계정·Claude 버전에 따라 별도 확인이 필요합니다.
