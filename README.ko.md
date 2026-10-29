> Local development branch: safe manual session transfer MVP. See [Cursor terminal guide](docs/SESSION-TRANSFER.ko.md). No new automatic limit-based switching.

# ccam — Claude Code Account Manager (Claude Code 다중 계정 관리)

[English](README.md) · **한국어** · 웹사이트: https://ccam.dev

- ccam은 컴퓨터 한 대에서 Claude Code 계정 여러 개를 쓰고 명령 하나로 바꿀 수 있게 해 주는 무료 오픈소스 명령줄 도구입니다.
- 계정마다 별도의 CLAUDE_CONFIG_DIR에 자기 로그인을 따로 두고, MCP 서버, 설정, CLAUDE.md, 확장 기능, 대화 내역은 계정끼리 공유할 수 있습니다. 그래서 한 계정이 사용량 한도에 도달하면 다른 계정에서 대화를 이어 갈 수 있습니다.
- Linux, macOS, Windows에서 명령 하나로 설치한 뒤, 계정마다 ccam add를 실행합니다.
- ccam은 비공식 프로젝트이며 Anthropic과 제휴하거나 Anthropic의 보증을 받은 것이 아닙니다.

`ccam`은 Linux, macOS, Windows 터미널에서 Claude Code 계정 여러 개를 추가하고 바꿔 쓰게 해 주는 단일 바이너리 도구입니다. 계정마다 `CLAUDE_CONFIG_DIR`을 따로 두어 로그인을 분리하고, MCP 서버·설정·전역 CLAUDE.md·확장 기능·대화 내역은 항목별로 공유할지 고릅니다.

> **이용약관을 먼저 확인하세요.** 계정을 여러 개 쓰는 것이 허용되는지는 각 계정의 Anthropic 이용약관(Pro·Max는 [Consumer Terms](https://www.anthropic.com/legal/consumer-terms), Team·Enterprise는 [Commercial Terms](https://www.anthropic.com/legal/commercial-terms), 모두에게 [Usage Policy](https://www.anthropic.com/legal/aup))과 소속 조직의 정책에 따릅니다. ccam은 계정의 한도를 늘리거나 합치거나 숨기지 않으며, 모든 계정은 자기 한도에서 멈춥니다. ccam은 요청할 때, 또는 `auto_switch`를 켜 두면(Linux, macOS) 한도에서 스스로, 작업을 다른 계정으로 이어 갈 뿐입니다. 그것이 내 계정들에 허용되는지는 사용자가 판단할 일입니다.

> **Team·Enterprise 시트.** 자동 선택(`ccam next`, `--fallback`, `ccam pick`, 대시보드의 `n`, 그리고 켜 두었다면 [`auto_switch`](https://ccam.dev/ko/features/next/#auto-switch))은 같은 Claude Team이나 Enterprise 조직의 시트 사이에서도 계정을 바꿉니다. Anthropic 고객센터는 "팀 플랜의 각 시트는 한 명의 지정된 개인에게 할당되며, 그 사람만 사용할 수 있습니다", "사용자는 다른 사용자의 계정을 통해 작업을 라우팅하여 해당 사용자의 남은 사용량을 사용할 수 없습니다"라고 안내합니다([Team 플랜](https://support.claude.com/ko/articles/12004354-purchase-and-manage-seats-on-team-plans#h_b8a7d07129), [Enterprise 플랜](https://support.claude.com/ko/articles/13393991-purchase-and-manage-seats-on-enterprise-plans#h_fa6b9cade0)도 마찬가지). 따라서 시트 사이에서 자동 선택을 쓰면 Anthropic 이용약관을 위반할 수 있습니다. 시트 사이에서 쓸지는 사용자가 판단할 일이니, 이 기능에 기대기 전에 소속 조직과 Anthropic에 확인하세요. 개인 요금제(Pro, Max)는 위의 안내를 따릅니다.

프로그램 출력(메시지, 질문, 도움말)은 모두 영어입니다.

## ccam을 쓰는 이유

- **로그인을 복사하지 않습니다.** 계정마다 자기 폴더에서 한 번 로그인하고, 토큰 갱신은 Claude Code가 그 폴더에서 직접 합니다. 계정을 바꿔도 다른 계정이 로그아웃되지 않고, 여러 계정을 동시에 실행할 수 있습니다.
- **기본으로는 Claude Code 옆에서 돌아가는 프로세스가 없습니다.** ccam은 `exec`로 `claude`가 됩니다. 래퍼·프록시·데몬이 없고, 실행할 때 더해지는 시간은 약 4 ms입니다. (`exec`이 없는 Windows에서는 Claude Code가 끝날 때까지 기다렸다가 종료 코드를 그대로 돌려줍니다.) `config.toml`에 `relay = true`(Linux·macOS에서는 `auto_switch = true`도)를 두면, 대화를 다른 계정으로 이어 갈 수 있도록 ccam이 대화형 세션마다 옆에서 기다립니다.
- **그룹이 공유할 것을 정합니다.** 그룹은 공유할 항목(MCP 서버·설정·CLAUDE.md·확장 기능·대화 내역)과 계정을 정하고, 계정은 여러 그룹에 들 수 있습니다. MCP 서버는 `--mcp-config`로 넘기므로 실행할 때 `.claude.json`을 고치지 않습니다. (계정이 MCP 공유를 그만두거나 다시 시작할 때, `ccam mcp import`, Remote Control에서만 백업을 남기고 고칩니다.)
- **바꾸기 전에 계획을 보여 주고 백업합니다.** 파일을 옮기기 전의 계획 출력(폴더 안에서 서로 다른 파일도 알려 줌), 백업, `--dry-run`, 원자적 쓰기, 그리고 중간에 멈춘 변경까지 마무리하는 `ccam doctor --fix`.
- **스크립트도 한도에 대응합니다.** `ccam --fallback work -p "…"`는 `work`가 한도에 걸리면 여유가 가장 많은 계정으로 같은 명령을 다시 실행하고, `ccam pick`은 병렬 작업자마다 다른 계정을 줍니다. 여유 있는 계정이 없으면 둘 다 종료 코드 75로 끝내거나 `--wait`로 기다립니다. 대화형 세션은 상태줄이 `ccam next --auto --continue`를 알려 주고, `auto_switch = true`(Linux, macOS. 기본은 꺼짐, 실험적 기능)면 Claude Code가 한도에서 멈춘 세션을 ccam이 여유 있는 다른 계정으로 스스로 옮깁니다. 같은 Team이나 Enterprise 조직 안에서는 `auto_switch`를 포함한 이 선택이 작업을 다른 시트로 옮기므로 Anthropic 이용약관 위반이 될 수 있습니다(위의 "Team·Enterprise 시트" 참고).
- **프로젝트마다 맞는 계정.** `ccam folder work ~/code/company`로 그 폴더에서 이름 없이 실행하면 `work`가 열립니다.
- **정적 바이너리 하나입니다.** Linux, macOS, Windows를 지원하고, 설치할 런타임이 없고, 설치와 업데이트는 체크섬을 확인하며, 텔레메트리가 없습니다.

claude-swap, CCS, clauth, CAAM, cux 등과의 비교: [ccam.dev/ko/compare/account-switchers](https://ccam.dev/ko/compare/account-switchers/).

## 설치

```bash
curl -fsSL https://ccam.dev/install.sh | sh
```

리눅스(우분투·데비안·페도라·아치·알파인 등)와 macOS를 지원합니다. `~/.local/bin/ccam`이 생깁니다. 체크섬을 확인한 뒤 설치하고, `~/.local/bin`이 `PATH`에 없으면 알려 줍니다. 그 밖의 방법:

- Windows 10·11: PowerShell에서 `irm https://ccam.dev/install.ps1 | iex`. 체크섬을 확인한 뒤 `%USERPROFILE%\.local\bin\ccam.exe`를 설치하고 그 폴더를 사용자 `PATH`에 더합니다. 폴더 공유는 그대로 되고, 파일 하나를 공유하려면(`settings.json`, `CLAUDE.md`, `history.jsonl`) 개발자 모드가 필요합니다. Windows는 그 모드(또는 관리자)에서만 파일 링크를 만들게 해 주기 때문입니다. 그래서 개발자 모드가 아니면 첫 `ccam add`가 MCP 서버와 확장 기능만 공유하는 `default` 그룹을 만듭니다. 스크립트를 다시 실행하면 세션이 옛 바이너리로 돌고 있어도 업데이트되고, 하드 링크로 만든 단축 명령도 새 버전으로 바꿉니다.
- `.deb`: [릴리스](https://github.com/jhyunwoo/ccam/releases)에서 받아 `sudo apt install ./ccam_*.deb`
- 소스: `go install github.com/jhyunwoo/ccam/cmd/ccam@latest` (Go 1.26 이상. 릴리스 태그로 빌드하면 그 버전을 알려 주므로 `ccam update`도 됩니다)

Claude Code(`claude`)가 먼저 설치되어 있어야 합니다.

Claude Code 플러그인(Claude Code 2.1.275 이상): `/plugin install ccam --marketplace jhyunwoo/ccam`을 설치하면 Claude Code가 사용량을 확인하고, `ccam pick`으로 다른 계정에서 작업자를 띄우고, `ccam ext`로 빠진 스킬의 원인을 설명합니다.

## 업데이트

```bash
ccam update            # 최신 릴리스 설치
ccam update --check    # 새 릴리스가 있는지 알려 주기만 함
```

`ccam update`는 내 시스템용 릴리스를 받아 릴리스의 `checksums.txt`(SHA-256)로 확인한 뒤, `ccam` 바이너리를 파일 이름 바꾸기 한 번으로 교체합니다. 단축 명령은 이 파일을 가리키는 링크라 함께 새 버전이 됩니다. `--version v0.2.0`은 그 릴리스를 설치하고(이전 버전도 가능), `ccam --dry-run update`는 받을 파일과 바꿀 파일만 보여줍니다. 대시보드(터미널에서 `ccam`)는 열 때마다 GitHub에 최신 릴리스를 묻고, 새 버전이 있으면 알려 줍니다. `U`를 누르고 확인하면 업데이트합니다. 다른 명령은 확인하지 않고, 묻지 않고 업데이트하는 일은 없습니다. `.deb`로 설치했다면 apt에 맡깁니다. [릴리스](https://github.com/jhyunwoo/ccam/releases)에서 새 `.deb`를 받아 `sudo apt install ./ccam_<버전>_linux_<아키텍처>.deb`를 실행하세요. 설치 스크립트를 다시 실행해도 `~/.local/bin`의 ccam이 업데이트됩니다.

## 빠른 시작

```bash
ccam add             # 이름 → 로그인 방식 → 그룹 → 단축 명령? → 요약 → 브라우저 로그인
ccam add work --shortcut ccwork    # 두 번째 계정, 단축 명령 ccwork

ccwork               # 단축 명령으로 work 계정의 Claude Code
ccam work --resume   # 이름으로 열기, 뒤의 인자는 그대로 claude에 전달
ccam                 # 대시보드: 계정·에이전트·그룹·폴더·확장 기능·점검
ccam folder work ~/code/company   # 그 폴더에서 이름 없이 실행하면 work
ccam usage           # 모든 계정의 5시간·주간·모델별(Fable) 사용률
ccam next --continue # 한도에 걸리면 다음 계정에서 같은 대화 이어가기
ccam --fallback work -p "fix the tests"   # 스크립트: 한도에 걸리면 다른 계정으로 다시 실행
```

이미 쓰던 `~/.claude`가 있으면 `ccam import`로 제자리에 등록합니다 (옮기거나 복사하지 않음).

쓰던 설정을 공유 설정의 출발점으로 삼으려면 `ccam migrate`를 실행합니다. `~/.claude`의 settings.json, CLAUDE.md, 확장 기능, MCP 서버를 공유 그룹으로 복사합니다. 원래 폴더는 읽기만 하고, 대화 내역은 `--history`를 줄 때만 복사합니다. 터미널에서는 먼저 복사할 항목을 체크 목록에서 고르고, 그다음 복사할 내용을 보여준 뒤 확인을 받습니다 (`ccam --dry-run migrate`는 보여주기만 함). MCP 서버의 저장된 로그인(OAuth)은 복사하지 않습니다. 복사한 로그인은 다른 쪽을 로그아웃시키기 때문입니다. 대신 `/mcp`로 직접 로그인할 계정을 계획에 알려 줍니다.

SSH로 접속한 원격 서버에서 로그인할 때는 내 컴퓨터의 브라우저가 열리지 않는 `http://localhost:<포트>/callback?code=…` 페이지에서 멈춥니다. 그 주소를 `ccam add`나 `ccam login`이 실행 중인 터미널에 붙여 넣으면 ccam이 서버의 Claude Code에 전달해 로그인을 마칩니다.

## 명령

| 명령 | 하는 일 |
| --- | --- |
| `ccam` | 전체 화면 대시보드 (터미널이 아니면 목록 출력) |
| `ccam <이름\|번호> [claude 인자]` | 그 계정으로 `claude` 실행. 인자는 해석하지 않고 그대로 전달 |
| `<단축 명령> [claude 인자]` | 계정에 단축 명령이 있으면 그 이름으로 바로 실행 |
| `ccam add [이름]` | 계정 추가. 터미널에서는 마법사, `--no-login`·`--group <그룹>`·`--no-group`·`--shortcut <명령>`도 가능. `--api --key-from env:NAME\|command:CMD`는 API 키 계정 |
| `ccam shortcut <이름> [명령] [--remove]` | 계정의 단축 명령 보기·만들기·이름 바꾸기·지우기 |
| `ccam list [--json]` | 계정 목록 |
| `ccam use <이름>` | 기본 계정 (`ccam -p "…"`처럼 이름 없이 실행할 때) |
| `ccam folder [이름] [폴더] [--remove]` | 그 폴더와 하위 폴더에서 이름 없이 실행할 때 쓸 계정. 인자가 없으면 목록 |
| `ccam --fallback [이름] -p …` | 계정이 사용량 한도에 걸리면 여유가 가장 많은 계정으로 다시 실행 (`-p` 실행만) |
| `ccam next [--auto] [--continue [--brief]] [-- claude 인자]` | 다음 계정으로. `--auto`는 사용량으로 고르고 이유를 알려 줌 (`--max-usage`, `--strategy headroom\|expiring\|balance`, `--wait`). `--continue --brief`는 마지막 대화의 요약으로 새 대화를 시작함. claude 옵션은 `--` 뒤에: `ccam next -c --auto -- --dangerously-skip-permissions` |
| `ccam pick [--json] [--model m] [--wait]` | 스크립트용으로 여유가 가장 많은 계정 이름 출력. 함께 시작한 작업자는 서로 다른 계정, 없으면 종료 코드 75 |
| `ccam ext [이름] [--json]` | 계정마다 로드되는 스킬·명령·플러그인·MCP 서버·훅과 그 출처, 문제 |
| `ccam usage [--watch] [--json] [-v]` | 사용량 (줄임 `ccam u`) |
| `ccam ccusage [-a 이름[,이름…]] [ccusage 인자]` | 모든 계정(또는 `-a`로 고른 계정)의 대화로 [ccusage](https://ccusage.com)의 토큰·비용 보고서. 그룹으로 대화 기록을 공유하는 계정은 그 그룹의 대화가 모두 들어감. ccusage 인자는 그대로 전달되고, 도구를 지정하지 않으면 찾은 다른 코딩 도구(Codex 등)도 함께 셈: `ccam ccusage claude daily`. `blocks`는 5시간 구간이 계정마다 따로라 계정 하나를 지정: `ccam ccusage -a work blocks --active`. 설치된 ccusage, 없으면 `bun x ccusage`, 그다음 `npx ccusage@latest`로 실행 |
| `ccam group [add\|set\|join\|leave\|show\|rename\|remove]` | 그룹: 계정 여럿이 공유할 것. 바꿀 때는 계획을 보여 주고 확인 후 실행 |
| `ccam share <이름>` | 계정이 무엇을 어느 그룹으로 공유하는지 보기 |
| `ccam mcp add\|add-json\|list\|remove\|import` | 공유 MCP 서버 |
| `ccam login\|logout <이름>` | 다시 로그인 / 로그아웃 (로그인과 MCP 서버 로그인을 지움. 함께 사라지는 것이 있으면 먼저 물어봄) |
| `ccam rename` / `ccam remove` | 이름 변경 / 삭제 (먼저 로그아웃하고, 폴더는 휴지통으로) |
| `ccam import [폴더]` | 기존 폴더 등록 |
| `ccam migrate [폴더]` | 기존 설정(기본 `~/.claude`)을 공유 그룹으로 복사. 원본은 바꾸지 않음. `--group`, 항목 옵션, `--history`, `--yes` |
| `ccam doctor [--fix]` | 링크·권한·로그인·버전·사용량 형식 점검과 수리 |
| `ccam statusline install` | Claude Code 상태줄에 `[account] · model · 5h N%` 표시 |
| `ccam update [--check] [--version vX.Y.Z]` | 최신(또는 지정한) 릴리스로 업데이트, 체크섬 확인 |
| `ccam uninstall` | 모든 계정(먼저 로그아웃), 단축 명령, ccam 폴더, ccam 자신을 지움. 가져온 폴더는 그대로 |
| `ccam completion bash\|zsh\|powershell` | 자동완성 |

앞에 `--dry-run`을 붙이면 실행하지 않고 할 일만 보여줍니다 (`ccam --dry-run work`). 실행(`ccam <이름>`, `run`, `next`), `group`, `migrate`, `shortcut`, `update`, `uninstall`, `doctor --fix`에서 쓸 수 있고, 무언가를 바꾸는 다른 명령은 실제로 실행하는 대신 거절합니다.

## 공유

그룹은 정한 항목을 그 그룹의 계정끼리 공유합니다: `ccam group add team --items history --members work,side`. 계정은 여러 그룹에 들 수 있지만 한 항목은 한 그룹에서만 받습니다. 처음 추가한 계정이 `default`(MCP 서버·설정·CLAUDE.md·확장 기능)를 만들고 다음 계정들은 거기에 들어갑니다.

| 항목 | 방식 |
| --- | --- |
| MCP 서버 | 그룹의 `mcp.json`을 실행할 때 `--mcp-config`로 전달 (실행할 때 `.claude.json`은 건드리지 않음. 공유를 그만두는 계정은 그룹 서버를 자기 `.claude.json`에 받음) |
| settings.json, 전역 CLAUDE.md | 그룹 파일로 심볼릭 링크 |
| 에이전트·명령·스킬·플러그인 | `agents/ commands/ skills/ plugins/ output-styles/ rules/` 폴더 링크 |
| 대화 내역 | `projects/`와 `history.jsonl` 링크 → 다른 계정에서 `--continue`/`--resume` |

로그인 파일(`.credentials.json`, `.claude.json`)은 어떤 경우에도 공유되지 않습니다. 그룹을 바꾸면 영향받는 모든 계정을 계획 하나로 옮기고, 원본을 `.ccam-backup/<시각>/`에 남기고, 파일이 서로 다르면 "keep group / use mine / merge JSON" 중에서 고르게 합니다. 그룹 이전의 `config.toml`은 그룹으로 읽고 다음 저장 때 새 형식으로 쓰며, 옛 파일은 `config.toml.bak`으로 남깁니다.

세션 안에서 `claude mcp add`로 넣은 서버는 그 계정에만, `-s user`가 없으면 그 폴더에서만 쓰이게 저장됩니다. 다음 실행 때 ccam이 폴더와 함께 알려 주면 `ccam mcp import <계정> <서버>`로 그룹에 옮기세요. `.claude.json`을 직접 고치는 방식이 필요하면 `config.toml` 맨 위(첫 `[[group]]`, `[[profile]]`, `[[folder]]`보다 위)에 `mcp_strategy = "sync"`를 둡니다. 계정이 따로 가진 같은 이름의 서버는 그대로 둡니다.

대화 내역을 공유하는 두 계정이 같은 폴더에서 동시에 돌면 경고합니다.

## 사용량

```
$ ccam usage
 1 personal  just now
   5-hour   ██████████████░░░░░░  72%  resets in 1h 40m
   Weekly   ████████░░░░░░░░░░░░  41%  resets in 3d 6h
   Fable    ██████████████████░░  88%  resets in 1d 6h
 2 work      just now
   5-hour   ██░░░░░░░░░░░░░░░░░░   8%  resets in 4h 12m
   Weekly   ████░░░░░░░░░░░░░░░░  19%  resets in 5d 2h
 3 side      12m ago (cached)
   5-hour   ████████████████████ 100%  resets in 22m
   Weekly   ██████████████░░░░░░  68%  resets in 1d 9h
→ Most headroom: work   (open it with ccam next --auto)
```

각 계정의 토큰으로 사용량 API를 동시에 조회합니다 (계정당 3초 제한, 60초 캐시). 이 API는 공개 문서가 없어 바뀔 수 있습니다. 실패하면 마지막 값이나 상태줄이 모아 둔 값을 보여주고, `ccam doctor`가 형식 변경을 알려 줍니다. Claude Code가 한도에 걸렸다고 알린 구간은(플러그인 훅이 기록) 초기화될 때까지 100%로 봅니다. 만료된 토큰은 직접 갱신하지 않습니다 (`--refresh`는 Claude Code를 어떻게 시작했든 그 계정이 실행 중이 아닐 때만, 갱신에는 30초까지 걸릴 수 있음).

## 파일 위치

```
~/.config/ccam/config.toml           계정 목록과 공유 설정
~/.local/share/ccam/profiles/<이름>/  계정별 CLAUDE_CONFIG_DIR
~/.local/share/ccam/groups/<그룹>/    공유 저장소
~/.local/state/ccam/                 사용량 캐시, 한도 기록, 마지막 사용 시각, 메모
~/.local/state/ccam/locks/           실행 표시와 잠금 파일
~/.local/bin/<단축 명령>              ccam을 가리키는 링크 (만든 계정만)
```

`CCAM_HOME`을 지정하면 모든 경로가 그 아래로 바뀝니다 (테스트용).

## 성능

`ccam <이름>`은 `config.toml` 하나만 읽고 `lstat`으로 링크를 확인한 뒤 `exec`로 `claude`가 됩니다. 일반 노트북에서 ccam이 더하는 시간은 p95 약 4 ms, 메모리는 8 MB입니다. `exec`이 없는 Windows에서는 같은 콘솔로 `claude.exe`를 띄우고 끝날 때까지 기다리고, `relay = true`나 `auto_switch = true`면 Linux·macOS에서도 대화형 세션을 자식 프로세스로 띄우고 옆에서 기다립니다. `CCAM_TRACE=1`이면 단계별 시간을, `CCAM_PPROF=<파일>`이면 CPU 프로파일을 남깁니다.

## 문제 해결

- 무엇이든 이상하면 `ccam doctor`, 고칠 수 있는 것은 `ccam doctor --fix` (바뀌는 파일은 백업).
- "공유 링크에 문제가 있어 실행을 멈췄습니다" → 공유 항목 자리에 실제 파일이 생긴 경우입니다. `doctor --fix`가 다시 그룹에 링크합니다. 그룹에 사본이 없으면 그 파일이 그룹 사본이 되고, 있으면 그룹 사본을 두고 원본을 백업합니다.
- 로그인 만료 → `ccam login <이름>`. 액세스 토큰만 만료됐다면(`access token expired`) 그 계정을 한 번 실행하면 Claude Code가 갱신합니다.

## 개발

```bash
go build ./cmd/ccam
go test ./...
golangci-lint run
scripts/perf-check.sh            # hyperfine으로 실행 경로 목표 확인
scripts/bench-compare.sh main    # benchstat으로 20% 넘는 회귀 검사
```

설계와 결정 기록은 [docs/PLAN.md](docs/PLAN.md), Claude Code 폴더 조사는 [docs/claude-layout.md](docs/claude-layout.md), 수동 점검표는 [docs/manual-qa.md](docs/manual-qa.md)에 있습니다.

## 라이선스

[MIT](LICENSE). ccam은 비공식 프로젝트이며 Anthropic과 제휴하거나 Anthropic의 보증을 받은 것이 아닙니다.
