# RepoForge Codex 执行计划

> 目标：本文件用于让 Codex 按明确需求实现 RepoForge。  
> 重点不是做复杂仓库管理平台，而是做一个简单、可靠、可拷贝目录交付的 Linux 离线软件源构建与 HTTP 分发工具。

---

## 0. 已确认需求

RepoForge 的需求以以下内容为准。

### 0.1 交付形态

RepoForge 的默认交付形态是一个目录。

```text
/opt/repoforge
```

这个目录可以整体拷贝到离线机器。离线机器不需要重新下载依赖，不需要重新生成复杂交付包。

### 0.2 交互方式

RepoForge 是命令行工具。

不做 TUI。  
不做 Web 管理页面。  
不做图形界面。

### 0.3 软件包下载规则

配置文件里写了什么包，`repoforge make` 就下载什么包以及它们的依赖。

不需要 `--group default`。  
不需要 `groups`。  
不需要 `base/security/monitor/full/default` 分组逻辑。

示例命令：

```bash
repoforge make --profile kylin-v10-sp3-x86_64
```

含义：

```text
读取 packages.yaml 中 kylin-v10-sp3-x86_64 下的 packages；
把 packages 里的全部软件包和依赖下载完整；
生成对应的软件源索引。
```

### 0.4 不需要的能力

第一版明确不做：

```text
1. 不做 GPG 签名；
2. 不生成 manifest；
3. 不生成 checksums；
4. 不做 packages 分组；
5. 不做 --group 参数；
6. 不做 Web 管理；
7. 不做 TUI；
8. 不做上传接口；
9. 不做删除接口；
10. 不做完整公网软件源镜像；
11. 不做 make --from 已有包目录自动补齐依赖。
```

### 0.5 必须具备的能力

第一版必须具备：

```text
1. 单目录自包含；
2. 自动识别 RepoForge Home；
3. init 初始化目录；
4. check 检查环境和仓库状态；
5. make 制作离线软件源；
6. use 启用本机 file:// 软件源；
7. server start 启动 HTTP 只读分发服务；
8. server enable 安装 systemd 服务并开机自启；
9. server stop 停止 systemd 服务；
10. server disable 禁用并删除 systemd 服务；
11. server status 查看 HTTP 服务状态；
12. RPM 软件源制作；
13. RPM 本机源启用；
14. RPM HTTP 源分发；
15. DEB 软件源制作；
16. DEB 本机源启用；
17. DEB HTTP 源分发；
18. 中文错误提示；
19. 幂等执行。
```

---

## 1. 项目定位

RepoForge 是一个用于离线、内网、隔离网络、机房运维场景的 Linux 软件源构建和分发工具。

核心流程：

```text
在线机器：
  1. 用户编辑 packages.yaml；
  2. 执行 repoforge make --profile xxx；
  3. RepoForge 下载软件包及依赖；
  4. RepoForge 生成软件源索引；
  5. 得到 /opt/repoforge/repos/<profile>/。

离线机器：
  1. 拷贝整个 /opt/repoforge 目录；
  2. 执行 repoforge use --profile xxx；
  3. 本机启用 file:// 软件源；
  4. 可以用 yum/dnf/apt 安装配置里的软件。

局域网机器：
  1. 离线机器执行 repoforge server enable；
  2. 其他机器使用 http://离线机器IP:8080/<profile>/ 作为软件源。
```

RepoForge 不重新实现 RPM/DEB 依赖求解器。

依赖解析由系统原生命令完成：

```text
RPM 后端：dnf / yum / rpm / createrepo_c
DEB 后端：apt-get / apt-cache / dpkg-deb / dpkg-scanpackages
```

RepoForge 只负责编排：

```text
读取配置；
检测系统；
调用外部命令；
整理目录；
生成 repo/list 文件；
启动 HTTP 服务；
输出中文错误和修复建议。
```

---

## 2. 技术选型

### 2.1 语言

使用 Go。

### 2.2 CLI 框架

可以使用 Cobra。

如果为了减少依赖，也可以使用标准库 `flag`，但推荐 Cobra，因为子命令结构清晰。

### 2.3 YAML 解析

使用：

```text
gopkg.in/yaml.v3
```

### 2.4 HTTP 服务

使用 Go 标准库：

```go
net/http
```

不要引入复杂 Web 框架。

### 2.5 systemd

通过生成 systemd unit 文件实现。

不需要依赖 systemd Go SDK。

---

## 3. 项目目录结构

Codex 按下面结构创建项目：

```text
repoforge/
├── go.mod
├── main.go
├── cmd/
│   ├── root.go
│   ├── init.go
│   ├── check.go
│   ├── make.go
│   ├── use.go
│   └── server.go
├── internal/
│   ├── home/
│   │   └── detect.go
│   ├── config/
│   │   ├── load.go
│   │   ├── model.go
│   │   └── render.go
│   ├── detect/
│   │   ├── os.go
│   │   ├── arch.go
│   │   └── command.go
│   ├── executor/
│   │   └── executor.go
│   ├── backend/
│   │   ├── backend.go
│   │   ├── rpm/
│   │   │   ├── rpm.go
│   │   │   ├── make.go
│   │   │   ├── use.go
│   │   │   └── check.go
│   │   └── deb/
│   │       ├── deb.go
│   │       ├── make.go
│   │       ├── use.go
│   │       └── check.go
│   ├── repo/
│   │   └── verify.go
│   ├── server/
│   │   ├── http.go
│   │   └── systemd.go
│   ├── templates/
│   │   └── templates.go
│   ├── fileutil/
│   │   └── file.go
│   └── logx/
│       └── log.go
├── templates/
│   ├── config.yaml
│   ├── packages.yaml
│   ├── profiles/
│   │   ├── kylin-v10-sp3-x86_64.yaml
│   │   └── debian-12-amd64.yaml
│   ├── rpm-local.repo.tpl
│   ├── rpm-client.repo.tpl
│   ├── deb-local.list.tpl
│   ├── deb-client.list.tpl
│   └── repoforge-server.service.tpl
└── README.md
```

---

## 4. RepoForge Home 识别规则

RepoForge 默认安装目录：

```text
/opt/repoforge
```

但程序不能硬编码只能在 `/opt/repoforge` 运行。

### 4.1 自动识别规则

按以下顺序识别 RepoForge Home：

```text
1. 使用 os.Executable() 获取当前 repoforge 二进制路径；
2. 使用 filepath.EvalSymlinks() 解析软链接真实路径；
3. 如果二进制路径是 <home>/bin/repoforge，则 home = <home>；
4. 从二进制所在目录开始向上查找 .repoforge-home；
5. 从二进制所在目录开始向上查找 config/config.yaml；
6. 如果当前工作目录或其上级存在 .repoforge-home，也可作为候选；
7. 如果仍无法识别：
   - init 命令：以二进制所在目录的上级作为 home；
   - 非 init 命令：报错，不要静默写入 /opt/repoforge。
```

### 4.2 软链接示例

```text
/usr/local/bin/repoforge -> /opt/repoforge/bin/repoforge
```

程序必须解析真实路径，最终识别：

```text
/opt/repoforge
```

---

## 5. 运行目录结构

`repoforge init` 生成以下目录：

```text
/opt/repoforge/
├── .repoforge-home
├── bin/
│   └── repoforge
├── config/
│   ├── config.yaml
│   ├── packages.yaml
│   ├── profiles/
│   │   ├── kylin-v10-sp3-x86_64.yaml
│   │   └── debian-12-amd64.yaml
│   └── templates/
│       ├── rpm-local.repo.tpl
│       ├── rpm-client.repo.tpl
│       ├── deb-local.list.tpl
│       ├── deb-client.list.tpl
│       └── repoforge-server.service.tpl
├── repos/
│   ├── kylin-v10-sp3-x86_64/
│   └── debian-12-amd64/
├── cache/
│   ├── rpm/
│   ├── deb/
│   └── tmp/
├── client/
│   ├── repoforge-kylin.repo
│   └── repoforge-debian.list
└── logs/
    └── repoforge.log
```

### 5.1 不需要的目录

第一版不要默认创建：

```text
dist/
state/
state/manifests/
state/checksums/
```

原因：

```text
不默认导出 tar.gz；
不生成 manifest；
不生成 checksums。
```

---

## 6. 配置文件设计

## 6.1 config.yaml

路径：

```text
/opt/repoforge/config/config.yaml
```

默认内容：

```yaml
schema_version: 1

app:
  name: repoforge
  language: zh_CN
  log_level: info

paths:
  home_dir: auto
  config_dir: ${home}/config
  profile_dir: ${home}/config/profiles
  template_dir: ${home}/config/templates
  repo_dir: ${home}/repos
  cache_dir: ${home}/cache
  client_dir: ${home}/client
  log_dir: ${home}/logs

default:
  backend: auto
  profile: kylin-v10-sp3-x86_64

server:
  listen: 0.0.0.0:8080
  root: ${home}/repos
  public_url: auto
  readonly: true
  directory_listing: false
  systemd:
    enabled: true
    service_name: repoforge-server
    service_file: /etc/systemd/system/repoforge-server.service
    restart: always
  firewall:
    manage: false
    port: 8080
    protocol: tcp

verify:
  check_os: true
  check_arch: true
  check_index: true
  check_local_repo: true
  check_http: true
```

### 6.1.1 配置替换规则

所有配置中的 `${home}` 必须替换为自动识别到的 RepoForge Home。

例如：

```text
${home}/repos -> /opt/repoforge/repos
```

### 6.1.2 public_url 规则

如果：

```yaml
public_url: auto
```

则程序在生成客户端配置文件时自动判断局域网访问地址。

最低要求：

```text
1. 获取非 loopback IPv4 地址；
2. 如果只有一个候选 IP，使用 http://<ip>:<port>；
3. 如果多个 IP，输出提示并优先使用默认路由网卡 IP；
4. 如果无法判断，提示用户手动配置 server.public_url。
```

允许用户手动写死：

```yaml
public_url: http://192.168.10.20:8080
```

---

## 6.2 packages.yaml

路径：

```text
/opt/repoforge/config/packages.yaml
```

第一版不要设计分组。

默认内容：

```yaml
schema_version: 1

profiles:
  kylin-v10-sp3-x86_64:
    packages:
      - vim
      - wget
      - curl
      - tar
      - unzip
      - net-tools
      - bind-utils
      - lsof
      - rsync
      - firewalld
      - audit
      - aide
      - sysstat
      - htop
      - iotop

  debian-12-amd64:
    packages:
      - vim
      - wget
      - curl
      - tar
      - unzip
      - net-tools
      - dnsutils
      - lsof
      - rsync
      - ufw
      - auditd
      - sysstat
      - htop
      - iotop
```

### 6.2.1 行为要求

执行：

```bash
repoforge make --profile kylin-v10-sp3-x86_64
```

必须读取：

```yaml
profiles.kylin-v10-sp3-x86_64.packages
```

里面有什么软件包，就全部下载。

不得要求用户传 `--group`。

---

## 6.3 RPM profile

路径：

```text
/opt/repoforge/config/profiles/kylin-v10-sp3-x86_64.yaml
```

默认内容：

```yaml
schema_version: 1

profile: kylin-v10-sp3-x86_64
backend: rpm

target:
  os: kylin
  version: V10-SP3
  arch: x86_64

compatibility:
  require_same_os: true
  require_same_version: true
  require_same_arch: true
  allow_cross_build: false

online:
  package_manager: auto
  resolver: installroot
  releasever: ""
  enable_repos:
    - base
    - updates
    - extras
  disable_repos:
    - "*"
  include_weak_deps: false
  use_installroot: true
  installroot: ${home}/cache/rpm/installroot/kylin-v10-sp3-x86_64
  clean_installroot_before_make: true

repository:
  profile_dir: ${home}/repos/kylin-v10-sp3-x86_64
  package_dir: ${home}/repos/kylin-v10-sp3-x86_64
  metadata_tool: createrepo_c
  createrepo_update: true
  gpgcheck: false

local_repo:
  repo_id: repoforge-local
  repo_name: RepoForge Local RPM Repo
  repo_file: /etc/yum.repos.d/repoforge-local.repo
  baseurl: file://${home}/repos/kylin-v10-sp3-x86_64
  makecache_after_enable: true

client_repo:
  repo_id: repoforge-lan
  repo_name: RepoForge LAN RPM Repo
  output: ${home}/client/repoforge-kylin.repo
  baseurl: ${server.public_url}/kylin-v10-sp3-x86_64
  gpgcheck: false
```

---

## 6.4 DEB profile

路径：

```text
/opt/repoforge/config/profiles/debian-12-amd64.yaml
```

默认内容：

```yaml
schema_version: 1

profile: debian-12-amd64
backend: deb

target:
  os: debian
  version: "12"
  codename: bookworm
  arch: amd64

compatibility:
  require_same_os: true
  require_same_version: true
  require_same_arch: true
  allow_cross_build: false

online:
  package_manager: apt
  include_recommends: false
  include_suggests: false
  use_apt_root: true
  apt_root: ${home}/cache/deb/apt-root/debian-12-amd64
  apt_cache: ${home}/cache/deb/apt-cache/debian-12-amd64
  apt_state: ${home}/cache/deb/apt-state/debian-12-amd64
  apt_sources_mode: copy_from_host
  run_apt_update_before_make: true

repository:
  profile_dir: ${home}/repos/debian-12-amd64
  package_dir: ${home}/repos/debian-12-amd64
  metadata_tool: dpkg-scanpackages
  trusted: true

local_repo:
  repo_file: /etc/apt/sources.list.d/repoforge-local.list
  baseurl: file:${home}/repos/debian-12-amd64
  suite: ./
  trusted: true
  update_after_enable: true

client_repo:
  output: ${home}/client/repoforge-debian.list
  baseurl: ${server.public_url}/debian-12-amd64
  suite: ./
  trusted: true
```

---

## 7. CLI 命令行为

## 7.1 root 命令

```bash
repoforge
```

输出中文帮助。

必须包含：

```text
RepoForge 是 Linux 离线软件源构建与分发工具。

常用命令：
  repoforge init
  repoforge check --profile xxx
  repoforge make --profile xxx
  repoforge use --profile xxx
  repoforge server start
  repoforge server enable
```

---

## 7.2 init 命令

```bash
repoforge init
repoforge init --force
```

### 7.2.1 行为

```text
1. 识别 RepoForge Home；
2. 创建 .repoforge-home；
3. 创建 config/；
4. 创建 config/packages.yaml；
5. 创建 config/config.yaml；
6. 创建 config/profiles/；
7. 创建 config/templates/；
8. 创建 repos/；
9. 创建 cache/；
10. 创建 client/；
11. 创建 logs/。
```

### 7.2.2 幂等要求

```text
1. 目录已存在不报错；
2. 配置文件已存在不覆盖；
3. --force 时允许覆盖默认模板文件；
4. --force 不应删除用户已有 repos/ 下的软件包。
```

### 7.2.3 输出示例

```text
RepoForge 初始化完成。

Home：/opt/repoforge
配置目录：/opt/repoforge/config
软件源目录：/opt/repoforge/repos

下一步：
1. 编辑 /opt/repoforge/config/packages.yaml
2. 执行 repoforge make --profile kylin-v10-sp3-x86_64
```

---

## 7.3 check 命令

```bash
repoforge check
repoforge check --profile kylin-v10-sp3-x86_64
```

### 7.3.1 行为

`check` 只检查，不修改系统。

检查内容：

```text
基础检查：
  Home 是否识别成功；
  config.yaml 是否存在；
  packages.yaml 是否存在；
  profile 文件是否存在；
  repos 目录是否存在；
  cache 目录是否存在。

系统检查：
  /etc/os-release 是否可读；
  当前 OS；
  当前版本；
  当前架构；
  当前 backend；
  是否与 profile target 匹配。

RPM 检查：
  dnf 或 yum 是否存在；
  rpm 是否存在；
  createrepo_c 是否存在；
  repos/<profile>/repodata/repomd.xml 是否存在。

DEB 检查：
  apt-get 是否存在；
  apt-cache 是否存在；
  dpkg-scanpackages 是否存在；
  repos/<profile>/Packages.gz 是否存在。

本机源检查：
  RPM: /etc/yum.repos.d/repoforge-local.repo 是否存在；
  DEB: /etc/apt/sources.list.d/repoforge-local.list 是否存在。

HTTP 检查：
  端口是否监听；
  RPM: repomd.xml 是否可访问；
  DEB: Packages.gz 是否可访问。
```

### 7.3.2 输出风格

使用清晰中文：

```text
[OK] RepoForge Home: /opt/repoforge
[OK] 配置文件存在: /opt/repoforge/config/config.yaml
[OK] 当前系统: kylin V10-SP3 x86_64
[OK] profile 匹配: kylin-v10-sp3-x86_64
[WARN] HTTP 服务未运行
[ERROR] 未找到 createrepo_c 命令

解决建议：
  dnf install createrepo_c
```

---

## 7.4 make 命令

```bash
repoforge make --profile kylin-v10-sp3-x86_64
repoforge make --profile debian-12-amd64
```

### 7.4.1 禁止事项

不得实现或要求：

```text
--group
--set
--package-group
groups.default
groups.full
```

### 7.4.2 通用流程

```text
1. 识别 Home；
2. 加载 config.yaml；
3. 加载 profile；
4. 加载 packages.yaml；
5. 根据 profile 名称读取 packages；
6. 检查 packages 不能为空；
7. 检查当前系统与 profile target 是否匹配；
8. 根据 backend 选择 rpm/deb 后端；
9. 调用后端下载软件包及依赖；
10. 生成软件源索引；
11. 输出软件源路径。
```

### 7.4.3 输出示例

```text
开始制作离线软件源。

profile: kylin-v10-sp3-x86_64
backend: rpm
软件包数量: 15
软件源目录: /opt/repoforge/repos/kylin-v10-sp3-x86_64

正在下载软件包及依赖...
正在生成 repodata...

完成。
软件源目录：/opt/repoforge/repos/kylin-v10-sp3-x86_64
```

---

## 7.5 use 命令

```bash
repoforge use --profile kylin-v10-sp3-x86_64
repoforge use --profile debian-12-amd64
repoforge use --disable --profile kylin-v10-sp3-x86_64
repoforge use --disable --profile debian-12-amd64
repoforge use --disable --remove --profile kylin-v10-sp3-x86_64
```

### 7.5.1 启用流程

```text
1. 识别 Home；
2. 加载 profile；
3. 检查 repos/<profile>/ 是否存在；
4. 检查索引文件是否存在；
5. 检查是否 root；
6. 生成本机源配置文件；
7. 写入系统源目录；
8. RPM 执行 dnf/yum makecache；
9. DEB 执行 apt-get update；
10. 输出完成提示。
```

### 7.5.2 RPM 本机源文件

路径：

```text
/etc/yum.repos.d/repoforge-local.repo
```

内容：

```ini
[repoforge-local]
name=RepoForge Local RPM Repo
baseurl=file:///opt/repoforge/repos/kylin-v10-sp3-x86_64
enabled=1
gpgcheck=0
metadata_expire=-1
```

### 7.5.3 DEB 本机源文件

路径：

```text
/etc/apt/sources.list.d/repoforge-local.list
```

内容：

```text
deb [trusted=yes] file:/opt/repoforge/repos/debian-12-amd64 ./
```

### 7.5.4 禁用逻辑

RPM：

```text
默认：将 enabled=1 改为 enabled=0；
带 --remove：删除 /etc/yum.repos.d/repoforge-local.repo。
```

DEB：

```text
默认：把 repoforge-local.list 重命名为 repoforge-local.list.disabled；
带 --remove：删除 /etc/apt/sources.list.d/repoforge-local.list。
```

---

## 7.6 server 命令

```bash
repoforge server start
repoforge server enable
repoforge server stop
repoforge server disable
repoforge server status
```

### 7.6.1 server start

```bash
repoforge server start
```

行为：

```text
1. 前台启动 HTTP 服务；
2. 默认监听 0.0.0.0:8080；
3. 默认根目录为 ${home}/repos；
4. 只允许 GET 和 HEAD；
5. 禁止 POST/PUT/DELETE/PATCH；
6. 默认关闭目录浏览；
7. Ctrl+C 停止。
```

注意：

```text
server start 不做后台 daemon；
后台和开机自启由 server enable + systemd 负责。
```

### 7.6.2 server enable

```bash
repoforge server enable
```

行为：

```text
1. 检查 root 权限；
2. 生成 systemd service；
3. 写入 /etc/systemd/system/repoforge-server.service；
4. 执行 systemctl daemon-reload；
5. 执行 systemctl enable repoforge-server；
6. 执行 systemctl restart repoforge-server；
7. 生成 client/ 下的客户端源配置文件；
8. 输出客户端使用方法。
```

### 7.6.3 server stop

```bash
repoforge server stop
```

行为：

```text
停止 systemd 管理的 repoforge-server 服务。
```

如果服务不存在，不要报致命错误，输出提示。

### 7.6.4 server disable

```bash
repoforge server disable
```

行为：

```text
1. 检查 root 权限；
2. systemctl stop repoforge-server；
3. systemctl disable repoforge-server；
4. 删除 /etc/systemd/system/repoforge-server.service；
5. systemctl daemon-reload。
```

服务不存在时不报致命错误。

### 7.6.5 server status

```bash
repoforge server status
```

输出：

```text
服务状态；
监听地址；
软件源根目录；
当前可用 profile；
客户端 repo/list 文件路径；
局域网访问 URL。
```

---

## 8. 后端接口设计

定义统一 Backend 接口。

```go
type Backend interface {
    Name() string
    Check(ctx context.Context, env *Environment, profile *ProfileConfig) error
    Make(ctx context.Context, cfg *Config, profile *ProfileConfig, packages []string) error
    EnableLocalRepo(ctx context.Context, cfg *Config, profile *ProfileConfig) error
    DisableLocalRepo(ctx context.Context, cfg *Config, profile *ProfileConfig, remove bool) error
    GenerateClientRepo(ctx context.Context, cfg *Config, profile *ProfileConfig, publicURL string) error
    VerifyRepo(ctx context.Context, cfg *Config, profile *ProfileConfig) error
}
```

业务层只依赖 Backend 接口，不直接写 RPM/DEB 细节。

---

## 9. 外部命令执行要求

所有外部命令必须通过：

```text
internal/executor
```

不得在各模块散落 `os/exec.Command`。

executor 必须支持：

```text
1. 命令名称；
2. 参数列表；
3. 工作目录；
4. 环境变量；
5. 超时；
6. stdout 捕获；
7. stderr 捕获；
8. exit code；
9. dry-run；
10. 中文错误包装。
```

命令执行错误示例：

```text
错误：执行命令失败。

命令：createrepo_c --update /opt/repoforge/repos/kylin-v10-sp3-x86_64
退出码：127
错误输出：command not found

解决建议：
请先安装 createrepo_c。
```

---

## 10. 系统检测要求

## 10.1 OS 检测

读取：

```text
/etc/os-release
```

字段：

```text
ID
ID_LIKE
VERSION_ID
VERSION_CODENAME
PRETTY_NAME
```

麒麟系统额外尝试读取：

```text
/etc/kylin-release
```

### 10.2 RPM 系识别

如果 `ID` 或 `ID_LIKE` 包含以下内容，认为是 RPM 系：

```text
rhel
fedora
centos
rocky
almalinux
kylin
openeuler
```

### 10.3 DEB 系识别

如果存在：

```text
/etc/debian_version
```

或者 `ID` / `ID_LIKE` 包含：

```text
debian
ubuntu
```

认为是 DEB 系。

### 10.4 架构识别

使用：

```bash
uname -m
```

内部需要归一化：

```text
x86_64 -> x86_64 / amd64
amd64  -> x86_64 / amd64
aarch64 -> aarch64 / arm64
arm64 -> aarch64 / arm64
```

RPM profile 使用：

```text
x86_64
aarch64
```

DEB profile 使用：

```text
amd64
arm64
```

---

## 11. RPM 后端实现要求

## 11.1 make 行为

执行：

```bash
repoforge make --profile kylin-v10-sp3-x86_64
```

流程：

```text
1. 读取 packages；
2. 检查 dnf 或 yum；
3. 检查 rpm；
4. 检查 createrepo_c；
5. 创建 repos/<profile>/；
6. 创建 installroot；
7. 调用 dnf/yum 下载软件包和依赖；
8. rpm 文件保存到 repos/<profile>/；
9. 调用 createrepo_c 生成 repodata；
10. 检查 repodata/repomd.xml 是否存在。
```

### 11.2 推荐命令模型

优先使用 dnf：

```bash
dnf --installroot=<installroot> \
  --releasever=<releasever> \
  install -y \
  --downloadonly \
  --downloaddir <package_dir> \
  --setopt=install_weak_deps=False \
  <packages...>
```

如果没有 dnf，但存在 yum，可以使用 yum 兼容命令。

第一版不要实现复杂多策略下载。

### 11.3 生成索引

```bash
createrepo_c --update <package_dir>
```

验收文件：

```text
<package_dir>/repodata/repomd.xml
```

---

## 12. DEB 后端实现要求

## 12.1 make 行为

执行：

```bash
repoforge make --profile debian-12-amd64
```

流程：

```text
1. 读取 packages；
2. 检查 apt-get；
3. 检查 apt-cache；
4. 检查 dpkg-scanpackages；
5. 创建 repos/<profile>/；
6. 创建 apt_root、apt_cache、apt_state；
7. 调用 apt-get 下载软件包和依赖；
8. deb 文件保存到 repos/<profile>/；
9. 生成 Packages；
10. 生成 Packages.gz；
11. 检查 Packages.gz 是否存在。
```

### 12.2 apt root 初始化

需要创建：

```text
<apt_root>/etc/apt/
<apt_root>/etc/apt/sources.list
<apt_root>/etc/apt/sources.list.d/
<apt_state>/lists/
<apt_state>/lists/partial/
<apt_cache>/archives/
<apt_cache>/archives/partial/
<apt_root>/var/lib/dpkg/status
```

`status` 文件可以为空，但必须存在。

### 12.3 推荐命令模型

```bash
apt-get install \
  --download-only \
  --reinstall \
  -o Dir=<apt_root> \
  -o Dir::Cache=<apt_cache> \
  -o Dir::State=<apt_state> \
  -o Dir::Cache::archives=<package_dir> \
  -o APT::Install-Recommends=false \
  -o APT::Install-Suggests=false \
  <packages...>
```

### 12.4 生成索引

```bash
cd <package_dir>
dpkg-scanpackages . /dev/null > Packages
gzip -9c Packages > Packages.gz
```

验收文件：

```text
<package_dir>/Packages.gz
```

---

## 13. HTTP 服务实现要求

## 13.1 基本要求

```text
1. 使用 net/http；
2. 根目录为 ${home}/repos；
3. 只读分发；
4. 只允许 GET 和 HEAD；
5. 禁止 POST、PUT、DELETE、PATCH；
6. 默认关闭目录浏览；
7. 防止路径穿越；
8. 不提供上传接口；
9. 不提供删除接口；
10. 不提供管理 API。
```

## 13.2 访问路径

RPM：

```text
http://<ip>:8080/kylin-v10-sp3-x86_64/repodata/repomd.xml
```

DEB：

```text
http://<ip>:8080/debian-12-amd64/Packages.gz
```

## 13.3 目录浏览

默认不展示目录列表。

如果请求目录：

```text
http://<ip>:8080/kylin-v10-sp3-x86_64/
```

可以返回 403，或者只允许访问明确文件。

但注意：yum/dnf/apt 需要访问明确索引文件和包文件，不能影响正常下载。

## 13.4 客户端配置文件生成

`server enable` 后生成：

```text
/opt/repoforge/client/repoforge-kylin.repo
/opt/repoforge/client/repoforge-debian.list
```

RPM 客户端文件内容：

```ini
[repoforge-lan]
name=RepoForge LAN RPM Repo
baseurl=http://192.168.10.20:8080/kylin-v10-sp3-x86_64
enabled=1
gpgcheck=0
metadata_expire=-1
```

DEB 客户端文件内容：

```text
deb [trusted=yes] http://192.168.10.20:8080/debian-12-amd64 ./
```

---

## 14. systemd 服务模板

路径：

```text
/etc/systemd/system/repoforge-server.service
```

模板：

```ini
[Unit]
Description=RepoForge Offline Repository HTTP Server
After=network.target

[Service]
Type=simple
Environment=REPOFORGE_HOME=/opt/repoforge
ExecStart=/opt/repoforge/bin/repoforge server start
Restart=always
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true
ReadOnlyPaths=/opt/repoforge/repos

[Install]
WantedBy=multi-user.target
```

如果日志需要写入 `/opt/repoforge/logs`，则增加：

```ini
ReadWritePaths=/opt/repoforge/logs
```

第一版也可以只输出到 journald，不写独立日志文件。

---

## 15. 权限要求

## 15.1 不需要 root 的命令

```text
repoforge check
repoforge make
repoforge server start，监听 8080 时
```

注意：某些系统上 `dnf --installroot` 或 apt 下载可能仍需要 root。遇到权限错误时输出中文提示。

## 15.2 需要 root 的命令

```text
repoforge use
repoforge use --disable
repoforge server enable
repoforge server stop
repoforge server disable
```

原因：

```text
use 需要写 /etc/yum.repos.d 或 /etc/apt/sources.list.d；
server enable/disable 需要写 /etc/systemd/system 并调用 systemctl。
```

## 15.3 权限错误示例

```text
错误：当前命令需要 root 权限。

原因：repoforge use 需要写入系统软件源目录。

请使用：
  sudo repoforge use --profile kylin-v10-sp3-x86_64
```

---

## 16. 幂等性要求

所有命令必须可重复执行。

```text
init：
  目录存在不报错；
  配置存在不覆盖；
  --force 才覆盖默认配置模板。

check：
  只检查，不修改系统。

make：
  已有 rpm/deb 不主动删除；
  可以重复下载缺失包；
  每次 make 后重新生成仓库索引。

use：
  本机源文件存在时更新内容；
  可重复执行 makecache 或 apt update；
  不删除其他源配置。

use --disable：
  源文件不存在时不报致命错误。

server start：
  端口占用时明确报错。

server enable：
  service 存在时覆盖更新；
  服务已运行时 restart。

server disable：
  服务不存在时不报致命错误。
```

---

## 17. 错误提示规范

所有错误必须中文输出，并给解决建议。

### 17.1 缺少 createrepo_c

```text
错误：未找到 createrepo_c 命令。

解决建议：
1. 请先在在线机器上安装 createrepo_c；
2. 麒麟/RHEL 系系统可尝试：
   dnf install createrepo_c
   或：
   yum install createrepo_c
3. 安装完成后重新执行：
   repoforge make --profile kylin-v10-sp3-x86_64
```

### 17.2 缺少 dpkg-scanpackages

```text
错误：未找到 dpkg-scanpackages 命令。

解决建议：
1. 请先安装 dpkg-dev；
2. Debian/Ubuntu 可执行：
   apt-get install dpkg-dev
3. 安装完成后重新执行：
   repoforge make --profile debian-12-amd64
```

### 17.3 profile 不匹配

```text
错误：当前系统与 profile 目标系统不一致。

当前系统：Debian 12 amd64
profile 目标：Kylin V10-SP3 x86_64

解决建议：
1. 请在与目标系统一致的在线机器上制作离线源；
2. 不建议跨发行版制作离线源；
3. 如确认风险，可在 profile 中设置 allow_cross_build: true。
```

### 17.4 HTTP 端口占用

```text
错误：端口 8080 已被占用。

解决建议：
1. 查看占用进程：
   ss -lntp | grep 8080
2. 或修改配置：
   /opt/repoforge/config/config.yaml 中的 server.listen
3. 修改后重新执行：
   repoforge server start
```

---

## 18. 验收标准

## 18.1 init 验收

执行：

```bash
repoforge init
```

预期：

```text
1. 生成 .repoforge-home；
2. 生成 config/config.yaml；
3. 生成 config/packages.yaml；
4. 生成 config/profiles/；
5. 生成 repos/；
6. 生成 cache/；
7. 生成 client/；
8. 重复执行不会破坏已有配置。
```

## 18.2 RPM make 验收

执行：

```bash
repoforge make --profile kylin-v10-sp3-x86_64
```

预期：

```text
1. repos/kylin-v10-sp3-x86_64/ 下出现 rpm 包；
2. repos/kylin-v10-sp3-x86_64/repodata/repomd.xml 存在；
3. 不需要 manifest；
4. 不需要 checksums；
5. 不需要 GPG 签名。
```

## 18.3 RPM use 验收

执行：

```bash
sudo repoforge use --profile kylin-v10-sp3-x86_64
```

预期：

```text
1. /etc/yum.repos.d/repoforge-local.repo 存在；
2. repo 文件中 gpgcheck=0；
3. dnf/yum makecache 成功；
4. 可以执行：
   dnf --disablerepo="*" --enablerepo="repoforge-local" install vim
```

## 18.4 DEB make 验收

执行：

```bash
repoforge make --profile debian-12-amd64
```

预期：

```text
1. repos/debian-12-amd64/ 下出现 deb 包；
2. Packages 存在；
3. Packages.gz 存在；
4. 不需要 Release 签名；
5. 不需要 manifest；
6. 不需要 checksums。
```

## 18.5 DEB use 验收

执行：

```bash
sudo repoforge use --profile debian-12-amd64
```

预期：

```text
1. /etc/apt/sources.list.d/repoforge-local.list 存在；
2. list 文件中包含 trusted=yes；
3. apt-get update 成功；
4. 可以执行：
   apt-get install vim
```

## 18.6 HTTP 服务验收

执行：

```bash
repoforge server start
```

RPM 验证：

```bash
curl http://本机IP:8080/kylin-v10-sp3-x86_64/repodata/repomd.xml
```

DEB 验证：

```bash
curl http://本机IP:8080/debian-12-amd64/Packages.gz
```

预期：

```text
1. 返回 HTTP 200；
2. 局域网其他机器可访问；
3. 不允许 POST/PUT/DELETE/PATCH；
4. 不提供上传和删除能力。
```

## 18.7 server enable 验收

执行：

```bash
sudo repoforge server enable
```

预期：

```text
1. /etc/systemd/system/repoforge-server.service 存在；
2. systemctl status repoforge-server 显示运行中；
3. /opt/repoforge/client/ 下生成客户端源配置文件；
4. 重复执行不会失败。
```

---

## 19. README 主流程

README 只展示普通用户需要的流程，不展示内部实现细节。

### 19.1 在线机器

```bash
repoforge init
vi /opt/repoforge/config/packages.yaml
repoforge check --profile kylin-v10-sp3-x86_64
repoforge make --profile kylin-v10-sp3-x86_64
```

### 19.2 拷贝目录

```bash
scp -r /opt/repoforge root@离线机器:/opt/repoforge
```

### 19.3 离线机器启用本机源

```bash
sudo /opt/repoforge/bin/repoforge use --profile kylin-v10-sp3-x86_64
```

### 19.4 离线机器作为局域网源

```bash
sudo /opt/repoforge/bin/repoforge server enable
```

### 19.5 局域网客户端使用

RPM：

```bash
cp /opt/repoforge/client/repoforge-kylin.repo /etc/yum.repos.d/
dnf makecache --disablerepo="*" --enablerepo="repoforge-lan"
dnf install --disablerepo="*" --enablerepo="repoforge-lan" vim curl
```

DEB：

```bash
cp /opt/repoforge/client/repoforge-debian.list /etc/apt/sources.list.d/
apt-get update
apt-get install vim curl
```

---

## 20. Codex 执行顺序

Codex 按以下顺序实现，不要一次性把所有复杂能力混在一起。

### 阶段 1：项目骨架和配置

```text
1. 创建 Go 项目；
2. 创建 CLI 子命令；
3. 实现 Home 检测；
4. 实现 config.yaml / packages.yaml / profile 加载；
5. 实现 ${home} 变量替换；
6. 实现 init 命令。
```

验收：

```bash
repoforge init
repoforge check
```

### 阶段 2：系统检测和 check

```text
1. 实现 OS 检测；
2. 实现架构检测；
3. 实现 backend 自动识别；
4. 实现命令存在性检测；
5. 实现 profile 匹配检查；
6. 实现 check 输出。
```

验收：

```bash
repoforge check --profile kylin-v10-sp3-x86_64
```

### 阶段 3：RPM 后端

```text
1. 实现 RPM Check；
2. 实现 RPM Make；
3. 实现 createrepo_c 索引生成；
4. 实现 RPM use；
5. 实现 RPM client repo 文件生成。
```

验收：

```bash
repoforge make --profile kylin-v10-sp3-x86_64
sudo repoforge use --profile kylin-v10-sp3-x86_64
```

### 阶段 4：DEB 后端

```text
1. 实现 DEB Check；
2. 实现 apt root/cache/state 初始化；
3. 实现 DEB Make；
4. 实现 Packages / Packages.gz 生成；
5. 实现 DEB use；
6. 实现 DEB client list 文件生成。
```

验收：

```bash
repoforge make --profile debian-12-amd64
sudo repoforge use --profile debian-12-amd64
```

### 阶段 5：HTTP 服务和 systemd

```text
1. 实现 server start；
2. 实现只允许 GET/HEAD；
3. 实现路径安全检查；
4. 实现 server status；
5. 实现 systemd service 生成；
6. 实现 server enable；
7. 实现 server stop；
8. 实现 server disable；
9. 实现客户端配置文件生成。
```

验收：

```bash
repoforge server start
sudo repoforge server enable
repoforge server status
sudo repoforge server disable
```

### 阶段 6：中文错误和 README

```text
1. 统一错误输出；
2. 给关键错误加解决建议；
3. 更新 README；
4. 删除 README 中任何 --group、manifest、checksums、GPG 签名相关描述。
```

---

## 21. 明确禁止 Codex 引入的内容

实现过程中不要主动添加以下能力：

```text
1. 不要添加 --group 参数；
2. 不要添加 packages 分组；
3. 不要生成 manifest；
4. 不要生成 checksums；
5. 不要做 GPG 签名；
6. 不要做 Web 管理；
7. 不要做 TUI；
8. 不要做上传接口；
9. 不要做删除接口；
10. 不要做 make --from；
11. 不要做完整公网 repo 镜像同步；
12. 不要在各模块直接 os/exec；
13. 不要把配置、缓存、日志分散到 /etc、/var/lib、/var/cache；
14. 不要让普通用户必须传 --home。
```

---

## 22. 最终效果

最终用户看到的体验应该是：

```bash
# 在线机器
repoforge init
vi /opt/repoforge/config/packages.yaml
repoforge make --profile kylin-v10-sp3-x86_64

# 拷贝 /opt/repoforge 到离线机器

# 离线机器启用本机源
sudo /opt/repoforge/bin/repoforge use --profile kylin-v10-sp3-x86_64

# 离线机器作为局域网源
sudo /opt/repoforge/bin/repoforge server enable
```

用户不需要理解：

```text
--group
manifest
checksums
GPG
复杂仓库管理
依赖求解器
```

核心原则：

```text
配置里有什么包，就下载什么包；
目录拷过去就能用；
命令少；
提示清楚；
HTTP 只读分发；
不做额外复杂功能。
```
