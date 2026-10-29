# ccam 베타 설치·사용 가이드 — AI에게 그대로 전달하기

작성: 2026-10-29 · 대상: macOS + Cursor 통합 터미널 · 배포 상태: 소스 빌드 베타

## 1. 이 문서를 받은 AI에게

사용자는 이 문서에 따라 자신의 Mac에 ccam 베타를 설치하고 실제 사용할 수 있게 설정하려고 합니다. 설명만 하지 말고 아래 순서로 환경 확인, 설치, 검증을 진행하세요. 이미 끝난 단계는 확인 후 건너뛰고, 사용자 변경사항을 보존하세요.

### 작업 범위

- 원본: https://github.com/jhyunwoo/ccam (MIT)
- 사용할 포크: https://github.com/juseok2/ccam
- 기능 브랜치: `feat/safe-session-transfer`
- 이 문서의 재현 기준 커밋: `7c424b4e03c722866fef480ffd9659ad831a1c3f`
- 계정별 로그인 분리, 수동 대화 이전·백업·검증, 선택적인 원본 자동 전환 설정을 제공합니다.
- 계정 별명 기본값은 `account-a`, `account-b`입니다. 사용자가 원하면 `work`, `personal` 등으로 바꾸되 모든 명령에 일관되게 적용하세요.
- 기존 `claude`, 기존 `ccam`, 실행 중인 작업을 변경하지 않고 별도 `ccam-beta` 명령을 설치합니다.

### AI가 지킬 실행 원칙

1. 실제 운영 프로젝트 대신 새 테스트 폴더에서 먼저 확인하세요. 다른 터미널의 Claude를 강제로 종료하지 마세요.
2. 기존 파일을 발견하면 내용을 확인하고 병합하세요. 설치 폴더 삭제, 설정 전체 교체, Git 강제 초기화를 하지 마세요.
3. 계정 로그인은 각 사용자가 브라우저에서 직접 합니다. 비밀번호·인증 코드·토큰을 채팅에 요구하거나 출력하지 마세요. 코드는 해당 로그인 대기 터미널에 사용자가 입력합니다.
4. 로그인 파일/Keychain을 다른 프로필로 복사하지 마세요. 각자 사용 권한이 있는 계정으로만 등록합니다. 팀원의 인증 정보를 모아 공유하는 도구로 설정하지 마세요.
5. 실제 실행 출력으로 성공을 판정하세요. 실패하면 다음 단계로 넘어가지 말고 원인을 해결하거나 정확히 보고하세요.
6. 자동 전환은 별도 선택 사항입니다. 사용자가 자동 전환까지 요청한 경우 8절을 적용하고, 요청하지 않았다면 수동 방식까지 완료한 뒤 켤지 확인하세요.
7. 실제 사용량을 일부러 소진하거나 실제 transcript에 가짜 제한 이벤트를 삽입하지 마세요. 모의 테스트와 실사용 관찰을 구분하세요.
8. 로그인 대기처럼 사용자의 행동이 필요한 단계에서만 안내하고 기다리세요. 로그인 완료 후 남은 검증을 이어가세요.

## 2. 무엇을 설치하는가

`ccam-beta account-a`를 입력하면 A 계정으로 일반 Claude Code 대화 화면이 열립니다. 이후에는 그냥 질문합니다. 매 질문마다 명령어를 붙이지 않습니다.

`account-a`는 모델이나 상품명이 아니라 사용자가 정한 계정 별명입니다. 이메일과 달라도 됩니다. 계정마다 로그인 상태와 대화 폴더가 독립적입니다.

수동 이전은 대화를 종료하고 백업·복사한 다음 다른 계정으로 재개합니다. 자동 전환은 원본 ccam의 실험적 기능이며, 설정과 조건이 맞는 대화형 세션에서만 동작합니다.

### 검증된 것과 아직 확인하지 못한 것

- macOS Apple Silicon에서 빌드 및 신규 기능 테스트 통과.
- 실제로 독립 로그인한 두 계정에서 짧은 텍스트 대화 수동 이전과 기억 유지 확인.
- 백업 SHA-256 무결성 검사 확인.
- 원본 자동 전환의 CLI·launcher·E2E 모의 테스트 통과.
- 실제 계정의 한도 소진으로 자동 전환되는 실사용 상황은 아직 미확인.
- 긴 코딩 작업, 첨부파일, 모든 도구 상태 복원, Intel Mac 실기기 사용은 별도 검증이 필요.
- 전체 Go 테스트에서 기존 `migrate` 테스트 3개가 실패했고, 동일 원본 커밋에서도 재현됨. 전체 테스트가 모두 통과했다고 안내하지 말 것.

## 3. 설치 전 확인

Cursor에서 새 터미널을 열어 확인합니다. 아래 명령을 기존 Claude 대화 입력창에 넣지 말고, 셸 프롬프트에서 실행합니다.

```sh
uname -s
uname -m
command -v git
command -v go
go version
command -v claude
claude --version
```

- 이 가이드는 macOS용입니다. `arm64`는 Apple Silicon, `x86_64`는 Intel입니다. 다른 OS면 그대로 실행하지 말고 별도 안내합니다.
- Go는 1.26 이상이 필요합니다. 없으면 공식 Go 배포판(https://go.dev/dl/)으로 준비하거나, 이미 사용하는 패키지 관리자로 설치합니다. 기존 버전을 임의로 제거하지 마세요.
- Claude Code가 없으면 공식 설치 안내(https://code.claude.com/docs/en/setup)에 따라 먼저 설치합니다. 이 문서의 실사용 확인 버전은 2.1.312입니다. 다른 버전은 별도로 검증합니다.
- `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`, `CLAUDE_CONFIG_DIR` 등 인증·경로 환경변수가 이미 있는지 값 없이 유무만 확인합니다. 의도한 계정 대신 다른 인증이 쓰이는지 확인하되 전역 설정을 임의로 지우지 않습니다.
- 필요한 실행 권한이나 도구가 없다면 설치가 끝난 것처럼 말하지 마세요.

## 4. 포크에서 빌드하고 독립 명령 설치

다음은 **신규 설치 예시**입니다. 같은 경로에 파일이 있으면 AI가 먼저 확인하고, 다른 설치라면 별도 경로를 정하세요.

### 4-1. 소스 받기 및 빌드

```sh
mkdir -p "$HOME/.local/share/ccam-beta"
git clone --branch feat/safe-session-transfer \
  https://github.com/juseok2/ccam.git \
  "$HOME/.local/share/ccam-beta/src"
cd "$HOME/.local/share/ccam-beta/src"
git checkout --detach 7c424b4e03c722866fef480ffd9659ad831a1c3f
mkdir -p "$HOME/.local/lib/ccam-beta"
go build -o "$HOME/.local/lib/ccam-beta/ccam" ./cmd/ccam
```

커밋을 고정하는 이유는 팀원마다 다른 코드를 설치하지 않기 위해서입니다. 업데이트할 때는 검토한 새 커밋으로 명시적으로 변경합니다. 빌드 실패 시 후속 설치를 중단하세요.

### 4-2. 매번 설정하지 않아도 되는 실행 명령

아래 래퍼는 호출할 때마다 같은 전용 데이터 폴더를 사용합니다. 바이너리 파일 이름은 `ccam`을 유지해야 원본의 계정 shortcut 감지와 충돌하지 않습니다.

```sh
mkdir -p "$HOME/.local/bin"
cat > "$HOME/.local/bin/ccam-beta" <<'SH'
#!/bin/sh
export CCAM_HOME="$HOME/.local/share/ccam-beta/runtime"
exec "$HOME/.local/lib/ccam-beta/ccam" "$@"
SH
chmod 755 "$HOME/.local/bin/ccam-beta"
```

기존 `ccam-beta` 파일이 있다면 위 명령으로 덮어쓰지 말고 확인·백업 후 필요한 부분만 수정하세요.

### 4-3. 새 터미널에서도 명령 찾기

현재 터미널에는 다음을 적용합니다.

```sh
export PATH="$HOME/.local/bin:$PATH"
ccam-beta session --help
```

기본 zsh를 쓰는 경우 AI는 `~/.zshrc`를 백업하고, 같은 설정이 없는 경우에만 아래 한 줄을 추가합니다. 다른 셸이면 해당 셸의 설정 파일을 사용합니다.

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Cursor에서 **새 터미널을 열어** 다시 확인합니다.

```sh
command -v ccam-beta
ccam-beta session --help
```

`list`, `transfer`, `verify`가 보이면 이 포크의 기능이 설치된 것입니다. 단순 `ccam --version`만으로 확인하지 마세요. 소스 빌드는 `dev`라고 나올 수 있습니다.

주의: 원본 README의 설치 스크립트, `go install github.com/jhyunwoo/ccam/...@latest`, `ccam-beta update`는 원본 배포판을 설치할 수 있습니다. 이 베타에서는 사용하지 않습니다.

## 5. 두 계정 등록 — 로그인은 최초 한 번

```sh
ccam-beta add account-a --no-group --no-login
ccam-beta login account-a
```

브라우저에서 첫 번째 계정으로 로그인합니다. 코드가 나오면 실행 중인 터미널의 `Paste code here if prompted >`에 직접 입력합니다. `logged in` 성공 메시지가 확인된 뒤 다음으로 넘어갑니다.

```sh
ccam-beta add account-b --no-group --no-login
ccam-beta login account-b
```

브라우저가 이전 계정을 자동으로 선택하면 계정 선택을 바꿔 **두 번째 계정**으로 로그인합니다. 동일 계정 두 번 등록은 계정 간 이전 검증이 아닙니다.

```sh
ccam-beta list
```

AI는 두 프로필의 로그인이 서로 다른지 사용자와 확인합니다. 공유할 결과 문서에서는 이메일을 가리세요. 계정이 이미 있으면 `add`를 반복하지 말고 필요할 때만 `login`을 실행합니다.

`--no-group`은 설정과 대화 기록의 자동 공유를 끕니다. 이번 테스트에서는 그대로 유지합니다. 기존 `~/.claude` 전체를 복사하거나 기존 계정을 로그아웃시키지 마세요.

## 6. 평소 사용하는 방법

작업할 프로젝트 폴더에서 한 번 실행합니다.

```sh
cd /내/프로젝트/폴더
ccam-beta account-a
```

그다음은 평소처럼 Claude에게 질문하면 됩니다. 종료는 Claude 입력창에서 `/exit`입니다.

- 다른 계정으로 새 대화: `ccam-beta account-b`
- 해당 계정의 대화 선택: `ccam-beta account-a --resume`
- 특정 대화 재개: `ccam-beta account-a --resume 세션UUID`
- 해당 계정의 현재 프로젝트 최신 대화: `ccam-beta account-a --continue`
- 로그인 만료 시: 해당 계정 작업을 정상 종료하고 `ccam-beta login account-a`

`-p`는 한 번 질문하고 끝내는 자동 테스트용 옵션입니다. 평소 대화에서는 붙이지 않습니다. 일반 `claude` 명령은 별도 기본 환경이므로 이 베타 관리자를 거치지 않습니다.

첫 대화형 실행에는 테마, 초기 안내, 폴더 신뢰 화면이 나올 수 있습니다. 사용자가 만든 테스트 폴더인지 확인하고 진행합니다. 불필요한 터미널 설정 변경이나 권한 우회 옵션은 활성화하지 않습니다.

## 7. 첫 실사용 테스트: 수동 대화 이전

### 7-1. 작은 테스트 대화 만들기

```sh
mkdir -p "$HOME/ccam-beta-test"
cd "$HOME/ccam-beta-test"
ccam-beta account-a
```

Claude 입력창에서 다음처럼 말합니다. 실제 비밀번호가 아닌 임의의 테스트 문구를 사용하세요.

> 대화 이전 테스트야. 테스트 문구는 초록우산-5831이야. 도구를 사용하지 말고 기억했다고만 답해.

답변을 받은 뒤 `/exit`로 종료합니다. B 계정도 실행 중이면 정상 종료합니다. 이전 중에는 양쪽 프로필을 새로 실행하지 마세요.

### 7-2. 세션 찾기와 사전 검사

```sh
ccam-beta session list account-a
```

출력의 UUID와 `--project-folder "..."`를 복사합니다. 아래 `SESSION_UUID`, `PROJECT_FOLDER`는 실제 출력으로 바꿉니다. AI가 폴더명을 추측하거나 다른 사람의 UUID를 쓰면 안 됩니다.

```sh
ccam-beta --dry-run session transfer account-a account-b SESSION_UUID \
  --project-folder "PROJECT_FOLDER"
```

`Validated; no session files written.`를 확인합니다. 잠금 파일은 만들어질 수 있지만 대화와 백업은 쓰지 않습니다.

### 7-3. 백업과 복사

```sh
ccam-beta session transfer account-a account-b SESSION_UUID \
  --project-folder "PROJECT_FOLDER"
```

`Copied and verified`와 출력된 `Backup:` 경로를 확인합니다. 원본 세션과 로그인은 보존됩니다. 같은 세션이 대상에 있으면 덮어쓰지 않고 중단합니다.

### 7-4. 다른 계정에서 기억 유지 확인

같은 테스트 프로젝트 폴더에서 실행합니다.

```sh
ccam-beta account-b --resume SESSION_UUID
```

Claude에게 **테스트 문구를 다시 알려주지 말고** 질문합니다.

> 아까 내가 알려준 테스트 문구가 뭐였지? 도구를 사용하지 말고 문구만 답해.

정확한 문구가 나오면 이 사례에서 대화 유지가 확인된 것입니다. 답이 틀리거나 재개가 거부되면 성공으로 처리하지 마세요.

백업도 확인합니다. Claude를 `/exit`로 종료하고:

```sh
ccam-beta session verify "/출력된/백업/절대경로"
```

`Backup verified.`를 확인합니다. 대화만 이전하므로 소스 코드 변경, 외부 첨부, 전역 file-history, MCP 인증까지 이식된다고 간주하지 않습니다.

## 8. 선택 사항: 사용량 제한 시 자동 전환

### 8-1. 기능의 정확한 의미

이 기능은 원본 ccam의 `auto_switch`를 사용합니다. 새로 만든 `session transfer`와 별도 경로입니다.

- macOS/Linux에서 `ccam-beta 계정명`으로 새로 시작한 **대화형 세션**이 대상입니다.
- Claude가 계정 요금제의 제한을 기록하고 조건이 맞으면, ccam이 다음 계정에서 대화를 재개하고 이어서 진행하라는 프롬프트를 전달합니다.
- 등록 순서대로 무조건 A→B→C가 아니라 원본의 사용량 정책으로 적합한 계정을 선택합니다.
- 독립 history에서는 `--resume <transcript 경로> --fork-session`을 사용합니다. 대상의 새 대화 UUID는 달라질 수 있습니다.
- **수동 이전 명령의 SHA-256 백업을 자동으로 만들어 주지는 않습니다.** 중요한 대화에 그 보장이 필요하면 수동 이전을 사용합니다.
- 다른 사용자에게 할당된 조직 계정이나 정책상 허용되지 않는 계정으로 작업을 넘기는 설정을 하지 마세요. 조직 제한을 우회하도록 설정을 바꾸지 않습니다.

### 8-2. AI가 적용할 설정

수정 전 해당 베타 프로필의 세션을 정상 종료합니다. 다음 실제 경로를 사용합니다. 이 문서의 래퍼를 설치했다면 XDG 기본 경로가 아니라 아래 경로입니다.

- 설정: `~/.local/share/ccam-beta/runtime/config/config.toml`
- A 프로필: `~/.local/share/ccam-beta/runtime/data/profiles/account-a`
- B 프로필: `~/.local/share/ccam-beta/runtime/data/profiles/account-b`

각 파일을 권한 0600의 타임스탬프 백업으로 저장합니다. TOML을 파싱해 최상위 `auto_switch`를 `true`로 갱신합니다. 기존 키를 중복 추가하거나 `[[profile]]` 내부에 넣지 마세요. 나머지 키와 프로필을 보존하고 다시 파싱해 확인합니다.

```toml
auto_switch = true
```

각 프로필의 `settings.json`을 JSON으로 읽고 기존 설정에 다음 hook을 **병합**합니다. `command`는 그 Mac의 `ccam-beta` 래퍼 절대 경로로 치환하세요. `~`나 예시 사용자 이름을 그대로 저장하지 않습니다. 경로에 공백이 있어도 작동하도록 셸 인용 처리합니다.

```json
{
  "hooks": {
    "StopFailure": [
      {
        "matcher": "rate_limit",
        "hooks": [
          {
            "type": "command",
            "command": "'/Users/실제사용자/.local/bin/ccam-beta' hook stop-failure",
            "timeout": 10
          }
        ]
      }
    ]
  }
}
```

다른 hook을 지우지 마세요. 같은 ccam hook이나 ccam 플러그인이 이미 있으면 중복 설치하지 않습니다. 플러그인 설치와 위 직접 설정 중 하나만 사용합니다. `disableAllHooks` 같은 설정이 있으면 임의로 해제하지 말고 작동 불가 사유를 알립니다. 설정이 정책으로 관리되는 환경은 정책을 우회하지 않습니다.

### 8-3. 두 계정에서 같은 프로젝트의 첫 실행 완료

```sh
cd "$HOME/ccam-beta-test"
ccam-beta account-a
```

초기 안내와 해당 폴더 신뢰 확인을 끝내고 `/exit`합니다. 다음도 동일하게 진행합니다.

```sh
ccam-beta account-b
```

둘 다 완료한 뒤 `/exit`하고 원하는 계정으로 새로 시작합니다.

```sh
ccam-beta account-a
```

이제 평소처럼 질문합니다. 설정 이전에 이미 실행 중이던 세션은 다시 시작해야 합니다. 자동 전환을 다른 프로젝트에서 사용할 때도 대상 계정에서 해당 프로젝트의 초기 신뢰 확인이 필요합니다.

### 8-4. 전환되지 않아도 정상인 경우

- 모든 후보 계정의 사용 여유가 부족함.
- 로그인이나 요금제·사용량 정보 확인에 실패함.
- 한도 초기화까지 15분 이하로 남음, 또는 초기화 시점을 확인하지 못함.
- Remote Control 연결 상태이거나, `-p`/백그라운드 등 지원하지 않는 실행 형태.
- 조직 구분, 대상 폴더 신뢰, 추가 사용 크레딧 등 원본의 조건을 충족하지 못함.
- 일반 서버 속도 제한, API 지출 제한 등 지원하는 요금제 제한 이벤트가 아님.
- 사용자가 Ctrl+C로 전환을 취소함.

전환을 억지로 만들려고 실제 세션 로그를 변조하거나 제한 정보를 조작하지 마세요. 실제 한도 도달 때 전환 대상, 대화 유지, 도구 중복 실행 여부를 관찰합니다. 중간에 실행하던 외부 작업이 정확히 한 번만 처리된다고 보장하지 않습니다.

### 8-5. 자동 전환 끄기

설정 최상위 `auto_switch = false`로 바꾸고 해당 세션을 재시작합니다. 세션을 강제 종료하거나 토큰을 삭제할 필요는 없습니다. 필요하면 AI가 자신이 추가한 hook만 제거하고 나머지 설정은 보존합니다.

## 9. 자주 겪는 문제

### `command not found: ccam-beta`

`~/.local/bin/ccam-beta`가 있는지와 실행 권한을 확인합니다. PATH 설정 후 새 터미널에서 다시 실행합니다. 임시 확인은 `"$HOME/.local/bin/ccam-beta" session --help`로 할 수 있습니다. alias 정의와 호출을 한 번에 붙여 넣기보다 이 가이드의 실행 파일 래퍼를 사용하세요.

### 계정이 없다고 나오거나 다른 계정 목록이 보임

`ccam`과 `ccam-beta`를 혼용하지 않았는지 확인합니다. `ccam-beta`는 전용 `CCAM_HOME`을 사용합니다. 개발자의 기존 `demo-a/demo-b`는 팀원 PC에 자동으로 생기지 않습니다.

### 브라우저를 닫았거나 로그인 링크가 오래됨

해당 로그인 대기 명령만 Ctrl+C로 취소하고 `ccam-beta login account-a`를 다시 실행합니다. 새로 열린 링크를 사용합니다. 코드가 있다면 그 실행의 터미널에 넣고 채팅에는 보내지 않습니다.

### `is running; quit ... first`

해당 프로필의 대화를 저장하고 `/exit`로 정상 종료합니다. `ccam-beta agents`로 확인할 수 있습니다. 어떤 프로세스인지 확인하지 않고 `killall claude`를 실행하지 마세요.

### `target session already exists`

이미 복사된 대화일 수 있습니다. 대상에서 해당 UUID를 재개해 확인하세요. 덮어쓰기를 위해 임의 삭제하지 않습니다. 두 계정에서 각각 이어 쓴 대화는 서로 갈라진 기록이며 자동 병합하지 않습니다.

### `symlink not supported`

history 공유 또는 연결된 프로젝트 폴더일 수 있습니다. 수동 이전 MVP는 프로필 루트 아래의 심볼릭 링크를 거부합니다. 링크를 삭제해 우회하지 말고 공유 구조를 확인하세요.

### JSONL 오류, session ID 불일치, 128 MiB 초과

복사를 중단합니다. 기록을 수정해서 통과시키지 않습니다. 원본과 백업을 보존하고 오류 문구를 보고합니다. 파일 합계 128 MiB는 현재 수동 이전 MVP 제한입니다.

### 다른 계정에서 재개 실패 / thinking 서명 오류

원래 계정에서 원본 세션을 재개합니다. 계정·조직·Claude 버전에 따라 재개가 거부될 수 있습니다. 서명이나 thinking 기록을 삭제해서 통과시키지 않습니다.

### 자동 전환이 일어나지 않음

8-4 조건을 먼저 확인합니다. `ccam-beta` 대화형 실행인지, 양쪽 hook이 한 번씩만 있는지, 설정 후 재시작했는지 확인합니다. 필요하면 `ccam-beta usage`로 상태를 확인하되 토큰/전체 설정을 공유하지 않습니다. `doctor --fix` 같은 변경 명령을 원인 확인 없이 실행하지 마세요.

## 10. 중단·복구·업데이트

- 베타 사용 중단: `/exit` 후 평소 쓰던 `claude`로 돌아갑니다. 베타 로그인과 대화는 별도 폴더에 남습니다.
- 실행 파일만 비활성화: AI는 설치 위치를 확인하고 `ccam-beta` 래퍼를 별도 보관할 수 있습니다. runtime 폴더나 Keychain은 자동 삭제하지 않습니다.
- 설정 되돌리기: 최근 사용자 변경을 확인한 뒤 백업과 비교해 이번 변경만 되돌립니다. 무조건 오래된 백업 전체를 덮어쓰지 않습니다.
- 수동 백업 복구: 먼저 `session verify`를 통과시킨 뒤, Claude를 종료하고 백업의 `projects/PROJECT_FOLDER/UUID.jsonl` 및 `UUID/`를 대상 프로필의 같은 상대 경로로 복사합니다. 기존 파일은 덮어쓰지 않습니다. 자동 복구 명령은 없습니다.
- 업데이트: 별도 안내된 커밋을 받아 테스트·빌드 후 기존 바이너리를 백업하고 교체합니다. 원본 자동 업데이트 명령은 쓰지 않습니다. 대화·인증 데이터는 이동하거나 초기화하지 않습니다.

## 11. AI의 완료 보고 형식

다음 항목을 실제 확인 결과로 작성하세요. 확인하지 못한 항목은 ‘미확인’으로 표시합니다.

- OS/CPU, Go 버전, Claude Code 버전, 설치한 Git 커밋
- 실행 명령과 바이너리/설정/데이터 경로
- 새 Cursor 터미널에서 `ccam-beta session --help` 성공 여부
- 등록된 별명과 서로 다른 계정 로그인 확인 여부(이메일은 마스킹)
- 수동 이전 사전 검사, 복사, 문구 기억, 백업 검증 결과
- 자동 전환 설정 여부와 양쪽 초기 실행/프로젝트 신뢰 확인 여부
- 자동 전환 검증 수준: 설정 확인 / 모의 테스트 / 실제 한도 상황 관찰 중 무엇인지
- 남은 오류와 다음에 사용자가 할 일

설정 파일, 토큰, 인증 URL, 로그인 코드, 실제 업무 대화 전문을 보고서에 넣지 마세요.

## 12. 팀원용 버그 제보 양식

```text
OS/CPU:
Claude Code 버전:
설치 커밋:
문제 유형: 설치 / 로그인 / 수동 이전 / 자동 전환 / 기타
수행한 명령(개인 경로와 민감한 인자는 가림):
기대한 동작:
실제 동작:
오류 문구(토큰·인증코드·이메일 등 제거):
자동 전환이면 실제 한도 도달 여부와 전환 대상:
대화 유지 여부 / 작업 중복 여부:
백업 검증 결과:
재현 가능 여부:
```

중요한 운영 작업 대신 작은 프로젝트부터 사용하세요. 소스와 실행 파일만 공유하며 계정 로그인 자료와 대화 백업은 공유하지 않습니다.
