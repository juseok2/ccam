# Cursor 터미널에서 대화 수동 이전하기

이 브랜치는 ccam 원본(MIT, Copyright 2026 jhyunwoo)의 독립 로그인 기능에 안전한 수동 세션 복사를 추가합니다. 원본 LICENSE를 유지합니다. 새 명령은 사용량을 조회하거나 제한 도달 시 계정을 자동으로 바꾸지 않습니다. 원본에 있던 `auto_switch`, `--fallback` 등은 이 작업에서 추가·수정하지 않았습니다.

## 시작

Go 1.26 이상에서 저장소 루트에서 빌드합니다.

```sh
go build -o ./ccam ./cmd/ccam
./ccam add personal --no-group
./ccam add work --no-group
```

각 계정의 로그인은 사용자가 별도로 완료합니다. 기존 계정은 다시 만들 필요가 없습니다. 두 계정이 history 공유 그룹에 함께 있으면(`projects/` 가 같은 그룹 폴더 링크) `session transfer` 는 복사 없이 재개 명령만 알려 줍니다. 한쪽만 공유 그룹이면 그 그룹 폴더로 복사합니다(0.2.0). 수동 이전만 사용할 때는 자동 전환 설정을 끈 상태로 사용합니다. 자동 전환은 아래 별도 절을 참고하세요.

Cursor의 통합 터미널에서 원래 프로젝트 폴더로 이동해 `ccam personal`로 작업합니다. 대화를 이전하려면 **그 대화를 쓰는 Claude 세션을 종료**한 뒤 다음 순서로 진행합니다(0.2.0부터 다른 대화의 세션은 그대로 둬도 됩니다. 확인: `ccam session running personal --session SESSION_UUID`). 아래 `SESSION_UUID`와 `PROJECT_FOLDER`는 list 출력으로 바꾸세요.

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

파일 합계 최대 128 MiB입니다. 프로필 루트는 실제 경로로 해석하고 `projects/` 링크(공유 그룹)는 한 번 따라가며, 그 아래의 심볼릭 링크, 특수 파일, 잘못된 UUID/경로, 손상된 JSONL은 거부합니다. 일반 파일 시스템의 하드 링크 지원이 필요합니다. 실행 중 여부 검사와 ccam 잠금은 다른 프로그램이 새로 Claude를 시작하는 것을 막지 못합니다. 이전이 끝날 때까지 그 대화를 두 프로필 어디에서도 열지 마세요. 프로세스 강제 종료나 전원 장애 후 남은 부분 복사본은 자동으로 정리하지 않습니다. 백업을 확인하고 수동으로 정리해야 합니다.

대화만 이전합니다. 프로젝트 소스 코드, 외부 이미지/첨부, 전역 file-history, 메모리, MCP 설정·인증, 도구 권한을 이식하지 않습니다. 파일 되돌리기 기능이나 외부 첨부 접근까지 동일하게 복구된다고 보장하지 않습니다. 다른 조직의 서명된 thinking 등으로 Claude가 재개를 거부하면 원래 계정에서 계속하세요. 서명이나 기록을 제거해 제한을 우회하지 않습니다.

공식 문서: [프로필별 로그인](https://code.claude.com/docs/en/authentication#log-in-with-multiple-accounts), [세션 재개](https://code.claude.com/docs/en/sessions). 실제 계정 간 재개는 계정·Claude 버전에 따라 별도 확인이 필요합니다.


## 선택 사항: 원본의 자동 전환 기능

사용자 요청으로 기존 `auto_switch` 기능을 테스트 프로필에서 활성화했습니다. 별도의 자동 전환 코드를 새로 작성한 것은 아닙니다. macOS/Linux의 대화형 `ccam 계정명` 실행에 적용되며 `-p` 실행이나 이미 실행 중인 일반 `claude`에는 적용되지 않습니다.

설정 파일 최상위(첫 `[profile]`/`[[profile]]` 이전)에 `auto_switch = true`를 둡니다. 각 프로필의 `settings.json`에는 기존 설정을 보존하면서 아래 hook을 추가하거나, 원본 ccam 플러그인을 사용합니다. 두 방법을 중복 설치하지 마세요. 명령의 경로는 실제 설치한 ccam의 절대 경로로 바꿉니다.

```json
{
  "hooks": {
    "StopFailure": [{
      "matcher": "rate_limit",
      "hooks": [{
        "type": "command",
        "command": "'/absolute/path/to/ccam' hook stop-failure",
        "timeout": 10
      }]
    }]
  }
}
```

두 프로필 모두에서 대상 프로젝트의 초기 설정과 폴더 신뢰 확인을 완료해야 합니다. 그다음 새로 `ccam 계정명`으로 시작합니다. Claude가 계정 요금제의 사용량 제한을 기록하면 원본 hook이 조건을 확인하고, 여유 있는 계정으로 대화를 재개하며 계속 진행하라는 프롬프트를 전달합니다. 다음 계정은 등록 순서가 아니라 원본의 사용량 선택 정책으로 결정됩니다.

여유 있는 계정이 없거나, 인증/사용량을 확인하지 못하거나, 대상 프로젝트가 신뢰되지 않거나, 조직 간 전환 제한 등 조건을 충족하지 못하면 전환하지 않습니다. 단순 서버 속도 제한이나 API 지출 제한에 모두 반응하는 기능은 아닙니다. 사용자의 Ctrl+C 취소도 존중합니다.

이 자동 경로는 위의 `session transfer` 명령과 다릅니다. 독립 history에서는 원본 transcript 경로를 `--resume`과 `--fork-session`으로 Claude에 전달하므로, 수동 이전의 SHA-256 백업 생성 단계를 실행하지 않습니다. 새 기능의 백업 보장이 필요하면 수동 이전을 사용하세요.

검증: 원본 자동 전환 관련 CLI/launcher/E2E 테스트를 모의 Claude로 실행해 통과했습니다. 실제 두 계정의 대화 이전은 별도 확인했지만, 실제 사용량 소진 이벤트를 발생시켜 자동 전환한 것은 아닙니다. 사용량을 일부러 소진하지 않습니다. 끄려면 `auto_switch = false`로 바꾸고 세션을 다시 시작합니다.
