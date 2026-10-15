# GitHub 공개 및 기여 안내

[English](../en-US/RELEASING.md) · [简体中文](../zh-CN/GITHUB.md) · [日本語](../ja-JP/GITHUB.md) · **한국어** · [繁體中文](../zh-TW/GITHUB.md) · [Documentation](../README.md)

## 현재 결정 사항

공개 소스 저장소는 [lorvanto/monitor-go](https://github.com/lorvanto/monitor-go), 표시 이름은 Monitor Go입니다. 릴리스 태그 및 대상 플랫폼은 추후 확정합니다. 애플리케이션 `0.4.0-dev`과 SQLite 스키마 `6`는 개발 기준이며 이미 공개된 릴리스를 의미하지 않습니다. 소스 push와 태그 및 배포 파일을 포함하는 Release 공개는 별도로 관리합니다.

프로젝트 코드와 문서는 [Apache-2.0](../../LICENSE)을 적용합니다. [NOTICE](../../NOTICE)와 [제삼자 라이선스 목록](../../THIRD_PARTY_NOTICES.md)도 확인하세요. 의존성, 글꼴 및 아이콘은 각각의 라이선스를 유지합니다. 라이선스 본문은 공식 영어 원문을 보존합니다.

## 문서 찾기

| 목적 | 문서 |
|---|---|
| 설치 및 운영 | [사용 안내](README.md) |
| 개발 및 PR | [Contributing](../../CONTRIBUTING.md) |
| 문의 및 문제 해결 | [Support](../../SUPPORT.md) |
| 비공개 취약점 신고 | [Security](../../SECURITY.md) |
| 변경 이력과 향후 후보 | [Changelog](../../CHANGELOG.md) · [Roadmap](../../ROADMAP.md) |
| 공개 절차 | [Release preparation](../en-US/RELEASING.md) |
| 번역 변경 | [번역 유지 관리](INTERNATIONALIZATION.md) |

## 개발 및 검증

Go 1.26.9 이상의 호환 버전, Node.js 22.12+ / 24.x, npm, Make가 필요합니다. 저장소 루트의 POSIX 셸에서 명령을 실행합니다. Windows에서는 WSL을 사용할 수 있습니다.

```sh
make build
./bin/monitor -data tmp/development -listen 127.0.0.1:5093
# 두 번째 터미널:
MONITOR_GO_API_TARGET=http://127.0.0.1:5093 npm --prefix web run dev
```

로컬의 `tmp/development/setup-token`을 확인하고 `http://127.0.0.1:5188`에서 초기화합니다. Go가 화면 자산을 포함하므로 Go 빌드와 테스트 전에 `web/dist`를 생성해야 합니다. 백엔드 변경 후에는 다시 빌드하고 재시작합니다. 운영 데이터는 사용하지 않습니다.

```sh
make test
npm --prefix web run format:check
# 문서만 변경한 경우:
npm --prefix web run check:docs
```

Go 변경에는 `gofmt`와 `go test ./...`도 실행합니다. API 변경은 독립된 서비스에 실제 HTTP 요청을 보내 확인하고 PR에 포트를 기록합니다. 화면 변경은 실제 브라우저의 데스크톱 및 좁은 화면, 관련 권한과 5개 언어를 확인합니다. 문서 검사는 Node.js만 있으면 실행할 수 있습니다.

SQLite만 저장소로 사용하며 리소스는 설치 인스턴스 전체에서 공유합니다. 생성자는 감사 정보입니다. 쓰기 API에 권한 검사를 추가하고 메뉴 숨김에 의존하지 마세요. 잠금 파일을 유지하고 새 의존성의 버전, 용도, 라이선스를 기록합니다.

PR에는 문제, 최종 동작, 수행한 검증, 호환성과 마이그레이션 영향을 설명하세요. 실행하지 않은 검사를 통과했다고 표시하지 않습니다. 화면 변경에는 가상 데이터로 만든 이미지를 첨부하고 사용자에게 영향을 주는 변경을 Unreleased에 기록합니다. 큰 변경은 먼저 범위를 논의합니다. 번역할 때는 5개 언어의 키와 치환 변수를 맞추며 API 식별자와 저장된 사용자 데이터를 번역하지 않습니다.

제출하는 기여에는 Apache-2.0의 기여 조항이 적용됩니다. 제삼자 표시를 보존하고 제공 권한이 있는 자료만 제출하세요. 별도 CLA나 DCO 절차는 설정되어 있지 않습니다. 서로 존중하며 사람이 아닌 문제와 근거에 대해 논의합니다.

## 문제 및 보안 신고

일반 Issue에는 버전이나 커밋, OS, 설치 방식, 브라우저와 언어, 재현 단계, 예상 결과와 실제 결과를 적습니다. 데이터베이스, 비밀 키, 쿠키, API 키, 등록 토큰 또는 Agent 설정을 첨부하지 않습니다.

이 저장소는 비공개 취약점 신고가 활성화되어 있습니다. [Report a vulnerability](https://github.com/lorvanto/monitor-go/security/advisories/new) 또는 **Security → Advisories → Report a vulnerability**를 사용합니다. 버튼이 없으면 취약점 세부 내용을 공개하지 않고 비공개 연락 방법만 Issue로 문의하세요. `SECURITY.md`만 추가해도 기능이 활성화되는 것은 아닙니다. 현재 별도 보안 이메일, 채팅 채널 또는 응답 시간 보장은 없습니다.

## 공개 전 확인

1. 공개 저장소는 `lorvanto/monitor-go`입니다. Go module과 내부 import는 `github.com/lorvanto/monitor-go`로 통일했습니다.
2. 스테이징된 파일 목록과 실제 내용을 검토합니다. `.gitignore`는 이미 추적된 파일이나 다른 경로의 비밀 정보를 제거하지 않습니다. 운영 데이터, 백업, 자격 증명과 개인 화면 이미지를 공개하지 않습니다.
3. 인접 저장소 없이 깨끗한 체크아웃에서 빌드하고 테스트합니다. Issues와 비공개 신고는 활성화되어 있습니다. 브랜치 보호와 CI는 아직 설정하지 않았으며 필요에 따라 구성하고 확인합니다. 템플릿이 이를 자동 설정하지는 않습니다.
4. 공개 저장소는 이미 생성되어 있습니다. 이후 동기화할 때는 변경 내용을 검토하고 remote가 `https://github.com/lorvanto/monitor-go.git`인지 확인한 후 승인된 브랜치를 push합니다. 새 저장소 생성 절차는 별도 저장소를 만들 때만 적용합니다. 태그, 배포 파일 및 다운로드 링크는 실제 공개하고 검증한 후 추가합니다.

바이너리 배포에는 대상 기기에서 실행, Agent 재연결, 권한, 파일, 알림, 종료 및 복구를 검증해야 합니다. 교차 컴파일 성공만으로 실행이 검증되지는 않습니다. 서비스를 중지한 뒤 전체 데이터 디렉터리를 백업하고 테스트 DB로 업그레이드와 롤백을 연습합니다.

릴리스 버전은 프런트엔드 package/lockfile, `internal/app/server.go` / `internal/app/settings.go`, 변경 이력에 함께 반영합니다. 현재 `-version` 옵션이나 링크 시 버전 주입은 없습니다. 배포 파일에는 바이너리, 실행 안내, LICENSE/NOTICE와 실제 간접 의존성까지 포함한 해당 라이선스 자료를 넣고 SHA-256 및 검증 기록을 만듭니다. `make build`는 압축 파일, 서명 또는 컨테이너를 자동 생성하지 않습니다.

[Release 템플릿](../../.github/RELEASE_TEMPLATE.md)으로 초안을 작성한 후 실제 첨부 파일, 체크섬, 호환성과 알려진 제한을 확인하고 공개합니다. 문서와 과거 검증 기록은 새 후보 버전의 실제 검증을 대신하지 않습니다.

컨테이너 변경 시 `docker compose config --quiet`와 `node scripts/test-docker.mjs`를 실행하세요. 데이터 보존, 호스트 지표 범위 및 로컬 빌드는 [Docker 배포](DOCKER.md)를 참고하세요. 레지스트리 게시는 별도 공개 작업으로 검증해야 합니다.
