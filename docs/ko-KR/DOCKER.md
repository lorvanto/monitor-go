# Docker 배포

[English](../en-US/DOCKER.md) · [简体中文](../zh-CN/DOCKER.md) · [日本語](../ja-JP/DOCKER.md) · **한국어** · [繁體中文](../zh-TW/DOCKER.md) · [Documentation](../README.md)

Docker Engine / Docker Desktop의 Linux 컨테이너 모드, BuildKit 및 Docker Compose v2.20 이상을 사용하세요. 현재 소스에서 빌드하며 `monitor-go:local`은 로컬 태그입니다. 게시된 레지스트리 이미지가 아닙니다. Docker로 빌드하고 실행할 때 호스트에 Go나 Node.js를 설치할 필요는 없습니다.


`make docker-build`는 Git 기반 버전을 주입합니다. Compose를 직접 쓸 때는 `MONITOR_VERSION`을 지정하며 생략하면 `0.4.0-dev`를 표시합니다. 직접 빌드할 때는 `docker build --build-arg VERSION="$(git describe --tags --always --dirty 2>/dev/null || printf 0.4.0-dev)" -t monitor-go:local .`을 사용할 수 있습니다. Git 이력은 Docker 빌드 컨텍스트에 포함하지 않습니다.

## 빌드 및 초기화

```sh
git clone https://github.com/lorvanto/monitor-go.git
cd monitor-go
docker compose up -d --build
docker compose exec monitor cat /data/setup-token
```

[로컬 서비스](http://127.0.0.1:5080/)를 열고 출력된 초기화 코드로 첫 관리자를 만드세요. 처음에는 영어로 표시되며 초기화 후 코드 파일을 삭제합니다. 기본 바인딩은 `127.0.0.1`뿐입니다. 5080을 사용 중이면 호스트 포트를 바꾸세요.

```sh
MONITOR_PORT=5081 docker compose up -d --build
```

## 데이터와 실행 범위

이름 있는 볼륨 `monitor-data`는 SQLite와 관련 파일, `master.key`, 초기화 상태 및 업로드를 포함한 `/data` 전체를 보존합니다. 소스의 `data/`와 별개입니다. 볼륨 하나에 서비스 프로세스 하나만 실행하세요. `docker compose down`은 볼륨을 보존하며 `docker compose down -v`는 삭제합니다. 컨테이너 재생성과 이미지 업데이트는 같은 볼륨을 사용합니다.

UID/GID는 `10001:10001`입니다. Compose는 읽기 전용 루트 파일 시스템, 쓰기 가능한 `/data` 및 임시 `/tmp`, `/readyz` 상태 검사를 사용합니다. 새 볼륨은 준비된 디렉터리 소유권을 받습니다. bind mount를 쓰면 UID/GID 10001이 쓸 수 있는 0700 전용 디렉터리를 준비하고 실행 중인 네이티브 인스턴스와 공유하지 마세요.

컨테이너 내부 호스트 수집은 기본적으로 꺼져 있습니다. Docker 호스트를 포함한 실제 호스트에 네이티브 Agent를 설치하고 등록하세요. `MONITOR_COLLECT_HOST=true`는 컨테이너/VM 환경을 관찰하며 물리 호스트와 같다고 보장하지 않습니다. 점검의 `127.0.0.1`도 컨테이너 자체입니다. 접근 가능한 서비스 DNS/IP를 사용하고 Docker Desktop에서는 `host.docker.internal`로 호스트 서비스에 연결할 수 있습니다. Docker socket, 호스트 PID 네임스페이스 또는 privileged 모드는 필요하지 않습니다.

## 설정 및 HTTPS

Compose는 shell 환경 또는 추적하지 않는 `.env`에서 `MONITOR_IMAGE`(`monitor-go:local`), `MONITOR_PORT`(5080), `MONITOR_BIND_ADDRESS`(127.0.0.1), `MONITOR_PUBLIC_URL`(비어 있음), `MONITOR_SECURE_COOKIE`(false), `MONITOR_COLLECT_HOST`(false), `MONITOR_TRUSTED_PROXY`(비어 있음)를 읽습니다. 이후 명령에도 같은 값을 유지하세요. 상태 검사를 위해 컨테이너 내부 포트는 5080을 유지합니다.

원격 브라우저 및 Agent 접근에는 호스트 포트 앞에 신뢰할 수 있는 HTTPS 프록시를 놓고 끝에 슬래시가 없는 외부 오리진을 지정하세요.

```sh
MONITOR_PUBLIC_URL=https://monitor.example.com MONITOR_SECURE_COOKIE=true \
  docker compose up -d --build
```

이 예시는 호스트에서 실행하는 프록시를 가정합니다. 다른 기기가 포트에 접근해야 하면 `MONITOR_BIND_ADDRESS`를 명시하세요. 사설 CA는 컨테이너 신뢰 저장소에도 설치해야 하며 호스트 인증서 설정은 자동으로 상속되지 않습니다.

## Compose 없이 실행

```sh
docker build -t monitor-go:local .
docker volume create monitor-go-data
docker run -d --name monitor-go --restart unless-stopped \
  -p 127.0.0.1:5080:5080 --mount source=monitor-go-data,target=/data \
  --read-only --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  --cap-drop ALL --security-opt no-new-privileges:true \
  --stop-timeout 20 monitor-go:local
docker exec monitor-go cat /data/setup-token
```

## 중지 후 백업과 복원

쓰기를 중지한 뒤 전체 볼륨을 보관하세요. 임시 컨테이너도 동일한 비root 실행 사용자로 중지된 데이터를 읽습니다. 백업은 자격 증명과 업무 데이터로 취급하여 비공개로 보관하세요.

```sh
mkdir -p backups
docker compose stop monitor
docker compose run --rm --no-deps -T --entrypoint tar monitor \
  -C /data -czf - . > backups/monitor-go-data.tar.gz
docker compose start monitor
```

새 Compose 프로젝트의 빈 볼륨에 복원하세요. 명령은 기존 데이터 덮어쓰기를 거부하고 실행 사용자(UID/GID 10001)로 복원 파일을 생성합니다. 새 Docker 호스트에서는 호환되는 로컬 이미지를 먼저 빌드하세요. 원본과 복원 인스턴스를 하나의 설치로 동시에 운영하지 마세요.

```sh
docker compose -p monitor-go-restore run --rm --no-deps -T \
  --entrypoint sh monitor -c \
  'test -z "$(ls -A /data)" && tar -xzf - -C /data' \
  < backups/monitor-go-data.tar.gz
MONITOR_PORT=5081 docker compose -p monitor-go-restore up -d --no-build
```

## 업그레이드와 롤백

업데이트 전에 중지 후 전체 백업 및 복원을 확인하고 검토된 소스를 갱신한 뒤 다시 빌드하세요.

```sh
git pull --ff-only
docker compose up -d --build
docker compose ps
docker compose logs --tail=100 monitor
```

Compose 프로젝트 이름과 볼륨을 유지하고 업데이트 중 삭제하지 마세요. 시작 시 SQLite 마이그레이션을 실행합니다. 롤백에는 이전 이미지/소스와 업데이트 전 전체 백업이 필요하며 이전 바이너리가 새 스키마를 읽지 못할 수 있습니다.

## 검증과 이미지 유지 관리

스모크 테스트는 테스트 호스트에 Node.js 22.12+ / 24.x도 필요합니다. 임시 Compose 프로젝트/볼륨 및 자동 선택된 루프백 포트를 사용해 초기화, 권한, 상태 검사, 업로드 바이트와 SHA-256, SQLite/세션 유지, 재생성 및 중지 후 백업/복원을 검증합니다. 테스트 리소스를 정리하고 로컬 이미지는 남깁니다.

```sh
docker compose config --quiet
node scripts/test-docker.mjs
```

테스트 대상을 지정하려면 `MONITOR_PLATFORM=linux/amd64` 또는 `linux/arm64`와 별도의 `MONITOR_IMAGE` 태그를 사용하세요. 비네이티브 실행에는 Docker 엔진의 에뮬레이터 지원이 필요하며 교차 빌드만으로 실행이 검증되지는 않습니다.

Node, Go 및 Alpine 단계를 분리하여 실행 이미지에는 빌드 도구를 포함하지 않습니다. npm/Go 잠금 파일과 버전을 지정한 기반 이미지를 사용합니다. 버전과 용도는 [제삼자 고지](../../THIRD_PARTY_NOTICES.md)를 참고하세요. 인증서와 시간대 데이터는 Alpine에서 제공하며 배포 시 기반 이미지와 OS 패키지를 재검토해야 합니다. `NODE_IMAGE`, `GO_IMAGE`, `RUNTIME_IMAGE`로 검토된 대체 이미지를 지정할 수 있습니다. BuildKit 교차 빌드를 지원하지만 실행 보장에는 대상별 검증이 필요합니다. 레지스트리 push, 릴리스 태그 생성 및 자동 게시는 수행하지 않습니다.

참고: [Docker 볼륨](https://docs.docker.com/engine/storage/volumes/) · [다중 플랫폼 빌드](https://docs.docker.com/build/building/multi-platform/).

## 신뢰할 프록시와 로그인 제한

Compose는 `MONITOR_TRUSTED_PROXY`를 `-trusted-proxy`로 전달합니다. 직접 접속할 때는 비워 두세요. 컨테이너에서 보이는 실제 프록시 CIDR만 지정하고 사설망 전체를 무조건 신뢰하지 마세요. 확인한 프록시 주소가 `172.20.0.2`라면 `MONITOR_TRUSTED_PROXY=172.20.0.2/32`입니다. 호스트에서 컨테이너로 전달하면서 주소가 달라질 수 있으므로 네트워크를 먼저 확인하세요.

`-trusted-proxy`는 쉼표로 구분한 프록시 CIDR 목록이며 기본값은 비어 있습니다(신뢰 없음). 잘못된 CIDR은 시작 오류를 발생시킵니다. 이 프로세스에서 보이는 실제 프록시만 지정하고 프록시가 `X-Forwarded-For`를 덮어쓰거나 안전하게 추가하도록 설정하세요. 서버는 오른쪽부터 확인하여 처음 만나는 신뢰 대상 밖의 주소를 사용합니다. 다른 연결의 헤더는 무시합니다. 로그인 실패만 집계하며 5분당 클라이언트 IP별 30회, 정규화된 사용자 이름별 10회입니다. 성공해도 횟수를 사용하거나 초기화하지 않습니다. 초기 설정은 모든 시도를 집계합니다(IP별 5분당 10회).
