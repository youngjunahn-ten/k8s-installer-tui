# k8s-installer-tui

`inventory.yml`과 `group_vars/all/vars.yml`을 터미널 UI(TUI)로 작성하는 도구입니다.  
[k8s-installer](https://github.com/ten1010-io/k8s-installer)의 설치 전 설정 단계에서 사용합니다.

## 사용 흐름

```
1. 이 레포에서 Go 소스 코드 유지 및 배포
        ↓
2. 서버(또는 CI)에서 빌드 → 바이너리 패키지로 저장
        ↓
3. k8s-installer의 download-bin.sh로 바이너리 다운로드
        ↓
4. 설치 담당자가 TUI 실행 → inventory.yml / vars.yml 생성
        ↓
5. k8s-installer 플레이북 실행
```

## 빌드

### 요구 사항

- Go 1.21 이상
- `make`
- 인터넷 연결 (첫 빌드 시 의존성 다운로드)

### 빠른 시작 — `set-env.sh` (Ubuntu/Debian 서버)

Go와 `make`가 설치되어 있지 않은 서버에서 한 번에 환경 세팅 + 빌드를 수행합니다.

```bash
bash set-env.sh
```

스크립트 동작 순서:

1. `apt install -y make`
2. Go 1.24.3 (linux-amd64) 다운로드 → `/usr/local/go`에 설치
3. `PATH` 영구 등록 (`/etc/profile`)
4. `make build` 실행

> **참고:** `set-env.sh`는 Ubuntu/Debian 계열에서만 동작합니다. 다른 OS에서는 [Go 공식 사이트](https://go.dev/dl/)에서 직접 설치 후 아래 명령어를 사용하세요.

### 명령어

```bash
# 의존성 설치 + 현재 OS용 바이너리 빌드
make build

# Linux amd64 바이너리 빌드 (배포용)
make build-linux

# Linux arm64 바이너리 빌드
make build-linux-arm64

# 빌드 결과물 삭제
make clean
```

빌드된 바이너리는 `dist/` 디렉터리에 생성됩니다.

| 명령어 | 출력 파일 |
|--------|-----------|
| `make build` | `dist/k8s-installer-tui` |
| `make build-linux` | `dist/k8s-installer-tui-linux-amd64` |
| `make build-linux-arm64` | `dist/k8s-installer-tui-linux-arm64` |

## 실행

```bash
# 기본 실행 (현재 디렉터리의 inventory.yml, group_vars/all/vars.yml 사용)
./k8s-installer-tui

# 경로 직접 지정
./k8s-installer-tui \
  --inventory /path/to/inventory.yml \
  --vars /path/to/group_vars/all/vars.yml

# 버전 확인
./k8s-installer-tui --version
```

### 기존 파일이 있는 경우

기존 `inventory.yml`과 `vars.yml`이 있으면 자동으로 읽어 편집 모드로 시작합니다.  
저장 시 기존 파일은 저장 시각을 붙인 이름으로 복사해 두므로, 여러 번 저장해도 이전 백업이 덮이지 않습니다.

```
inventory.yml      →  inventory.yml.bak.20261110-153000  (백업, 복사)
inventory.yml      ←  새 내용으로 저장 (임시 파일에 쓴 뒤 바꿔치기)
```

저장은 TUI가 다루는 값만 바꿉니다. TUI 화면에 없는 키(`kubelet_*`, `sysctl_extra` 등),
호스트별 다른 변수, 다른 그룹(`broken_node` 등)과 주석은 그대로 남습니다.

- `vars.yml`: TUI가 다루는 최상위 키의 줄만 바꾸고 나머지 줄은 한 글자도 바꾸지 않습니다. 파일에 없던 키는 끝에 붙입니다.
- `inventory.yml`: 내용과 주석은 남지만, YAML 라이브러리 특성상 빈 줄이 빠지고 일부 주석의 들여쓰기가 바뀔 수 있습니다.

## 키 조작

| 키 | 동작 |
|----|------|
| `Ctrl+N` | 다음 단계 (현재 단계 검증 후 이동) |
| `Ctrl+P` | 이전 단계 |
| `Ctrl+C` | 종료 |
| `Tab` / `Shift+Tab` | 필드 또는 섹션 이동 |
| `↑` / `↓` | 목록 이동 |
| `Space` | 체크박스/라디오 토글 |
| `a` | 항목 추가 |
| `e` / `Enter` | 항목 편집 |
| `d` | 항목 삭제 |
| `s` | 저장 (7단계 미리보기 화면에서) |

## 7단계 Wizard

```
[1.노드] → [2.그룹] → [3.네트워크] → [4.K8s] → [5.AIPub] → [6.인증서] → [7.저장]
```

### 1단계 — 노드 정의 (`inventory.yml: all.hosts`)

클러스터를 구성하는 노드를 추가합니다.

| 필드 | 설명 | 기본값 |
|------|------|--------|
| 이름 | 노드 식별자 (예: `node1`) | — |
| ansible_host | 접속할 IP 또는 호스트명 | — |
| ansible_port | SSH 포트 | `22` |
| ansible_ssh_user | SSH 사용자 | `root` |

### 2단계 — 그룹 할당 (`inventory.yml: ki_cp_node, k8s_node`)

노드별로 역할을 체크박스로 지정합니다.

| 열 | 설명 |
|----|------|
| `ki_cp_node` | KI Control Plane 노드 (DNS, NTP, 레지스트리 등) |
| `Control` | 설치를 실행하는 노드 → `control_node.hosts.localhost.control_node_ih`. 하나만 고르며 `ki_cp_node`여야 합니다(고르면 `ki_cp_node`도 함께 체크). 비어 있으면 첫 `ki_cp_node`로 정해집니다. k8s-installer는 이 값이 없으면 시작하지 않습니다(`check-control-node.yml`) |
| `k8s_node` | Kubernetes 클러스터 노드 |
| `k8s_cp` | Kubernetes Control Plane 노드 (`k8s_node` 필수) |
| `nvidia_gpu` | GPU 노드 (`k8s_node` 필수) |

### 3단계 — 네트워크 (`vars.yml`)

| 항목 | 설명 |
|------|------|
| 내부 서브넷 | `internal_network_subnets` (CIDR 목록) |
| CP HA 모드 | `ki_cp_ha_mode` + `ki_cp_ha_mode_vip` |
| DNS upstream | `ki_cp_dns_server_upstream_servers` |
| NTP upstream | `ki_cp_ntp_server_upstream_servers` |
| DNSSEC 검증 | `ki_cp_dns_dnssec_validation` |
| 내부 DNS 서버 활성화 여부 | 활성화 시 `internal_network_extra_zone`/`internal_network_extra_zone_a_records` 주석 해제, 도메인은 5단계 인그레스 도메인, IP는 4단계 인그레스 VIP로 자동 설정 |

### 4단계 — Kubernetes (`vars.yml`)

| 항목 | 설명 |
|------|------|
| 인증서 유효기간 | `k8s_certificate_validity_period` (예: `26280h`) |
| 로드밸런서 | `k8s_load_balancers` (이름, VIP, 노드 목록) |
| 기본 인그레스 | `k8s_default_ingress_class` (LB 선택, 호스트 포트) |

### 5단계 — AIPub (`vars.yml`)

| 항목 | 설명 |
|------|------|
| 인그레스 도메인 | `aipub_ingress_zone` |
| AIPub HA 모드 | `aipub_ha_mode` + `aipub_ha_mode_storage_class` |
| AIPub CP 노드 | `aipub_cp_nodes` |
| Harbor 설정 | 서브도메인, 레플리카 수, 스토리지 크기 4종 |

### 6단계 — 인증서 모드 (`vars.yml: ki_cert_mode`)

| 값 | 설명 |
|----|------|
| `self_signed` | CA 자동 생성 + TLS 인증서 발급 (기본값) |
| `ca_provided` | 고객이 `ca.crt` + `ca.key` 제공, TLS만 발급 |
| `tls_provided` | 고객이 `tls.crt` + `tls.key` 모두 제공 |

### 7단계 — 미리보기 & 저장

- `Tab`으로 `inventory.yml` / `vars.yml` 탭 전환
- 전체 검증 결과 표시 (오류가 있으면 저장 불가)
- `s`로 저장

## 검증 규칙

저장 전 다음 규칙을 모두 통과해야 합니다.

| # | 규칙 |
|---|------|
| R1 | `ki_cp_ha_mode=true`이면 `ki_cp_ha_mode_vip` 필수이며 VIP는 CP 노드 서브넷 내에 있어야 함 |
| R2 | 모든 `ki_cp_node`는 `internal_network_subnets` 에 속해야 함 |
| R3 | `k8s_load_balancers[].nodes`는 `k8s_node` 그룹의 부분집합이어야 함 |
| R4 | `k8s_default_ingress_class.load_balancer`는 정의된 LB 이름 중 하나여야 함 |
| R5 | `aipub_ha_mode=true`이면 `aipub_ha_mode_storage_class` 필수 |
| R6 | `aipub_cp_nodes`는 `k8s_node` 그룹의 부분집합이어야 함 |

## 프로젝트 구조

```
k8s-installer-tui/
├── main.go                          # 진입점 (플래그 파싱, 파일 로드, TUI 실행)
├── go.mod
├── Makefile
└── internal/
    ├── state/
    │   ├── types.go                 # NodeConfig
    │   └── state.go                 # AppState, DefaultState()
    ├── fileio/
    │   ├── loader.go                # YAML 파일 → AppState
    │   ├── writer.go                # AppState → YAML, 시각 붙인 백업, 안전한 쓰기
    │   ├── merge.go                 # 기존 파일에 TUI 값만 반영 (나머지 키·주석 보존)
    │   └── merge_test.go            # testdata/: k8s-installer main의 inventory·vars 예시
    ├── validator/
    │   └── rules.go                 # R1~R6 검증
    └── ui/
        ├── app.go                   # Bubbletea App, 화면 라우팅
        ├── styles/
        │   ├── styles.go            # Lipgloss 스타일
        │   └── keys.go              # 키 바인딩
        └── screens/
            ├── common.go            # Screen 인터페이스
            ├── s1_nodes.go
            ├── s2_groups.go
            ├── s3_network.go
            ├── s4_kubernetes.go
            ├── s5_aipub.go
            ├── s6_cert_mode.go
            └── s7_preview.go
```

## 의존성

| 라이브러리 | 용도 |
|-----------|------|
| [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) | TUI 프레임워크 (Elm Architecture) |
| [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles) | 텍스트 입력, 뷰포트 등 UI 컴포넌트 |
| [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) | 터미널 스타일링 |
| [gopkg.in/yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3) | YAML 파싱 및 직렬화 |

## 라이선스

Apache License 2.0
