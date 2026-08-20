# RepoForge

RepoForge 是一个**跨平台（Windows / Linux / macOS）**的离线 Linux 软件源构建与分发工具。

它用**单文件配置 `repo.yaml`** 描述一切：多个仓库、上游 URL（支持变量占位符和多值架构）、制作方式
（全量镜像 `sync` / 按需制作 `make`）。引擎**纯 Go 实现，不依赖本机 dnf/apt/createrepo_c**，
因此可以在任意机器上制作面向任意发行版的离线 yum/apt 源。

## 核心能力

- **`sync`**：全量镜像上游仓库（整仓拉取、增量、校验），并生成带 Requires/Provides 的 yum/apt 索引。
- **`make`**：按需制作离线源——只下载指定软件及其依赖（RPM 与 DEB **自动依赖求解**）。
- **`make-upgrade`**：对照本机已装包，从上游拉更新及依赖（需在目标 Linux 上运行）。
- **`list` / `check`**：查看本地离线源、检查 repo.yaml 与上游可达性。
- **`client` / `use` / `server`**：生成客户端配置、启用本机 file:// 源、局域网 HTTP 分发。
- **`gpg`**：OpenPGP 密钥管理——`gpg init` 生成签名密钥对，`gpg export` 导出公钥；`signing.enabled` 后自动签名 repodata / Release。
- 单文件配置、多仓库、多架构自动展开、变量占位符、下载进度输出。

### 元数据签名

- `repoforge gpg init` 生成 Ed25519 密钥对（纯 Go，无需本机 gpg）到 `home/config/signing/`。
- repo.yaml 设置 `signing.enabled: true` 后，`sync` / `make` 自动签名生成的索引：RPM 写 `repodata/repomd.xml.asc`（客户端 `repo_gpgcheck=1` 校验），DEB 写 `Release` / `InRelease` / `Release.gpg`（apt `Signed-By` 校验）。
- `repoforge gpg export` 导出公钥，分发给客户端配置 `gpgkey` / `Signed-By`。
- 未配置密钥时跳过签名并提示，不影响 `trusted=yes` / `gpgcheck=0` 的客户端。

### 元数据兼容

- **zstd 压缩 repodata**：Fedora 39+ / openSUSE / 新版 EPEL 的 `primary.xml.zst`，以及 DEB 的 `Packages.zst` 均可解析。
- **校验算法**：`upstream.verify` 支持 `auto`（按上游元数据声明的 sha256/sha1/md5 校验，缺省）与强制 `sha256|sha1|md5`；生成的 primary.xml / Packages 按实际算法输出。

### 下载能力

- **多文件并行**：多个包同时下载（`sync.concurrency`，默认 8）。
- **单大文件分段**：`sync.segment` 是开关+数量一体——`false`=不分片（只用多文件并行）；正整数=单文件分段上限；缺省=智能(默认 8)，
  段数按文件大小自动 `ceil(size/segment_threshold)`。例如 20 MiB 每段：40MiB→2 段、100MiB→5 段、200MiB→8 段(封顶)；≤20MiB 不分段。
- **断点续传**：中断后从断点继续，不重下（`sync.resume: true`）。中途被 CDN 掐断时也会带着已下字节重试 Range。
- **增量**：已存在且校验匹配的包跳过；孤儿 `.part` 自动清理。
- **脆弱镜像自动降压**：官方麒麟源（`*.cs2c.com.cn` / `*.kylinos.cn`，腾讯 EdgeOne）会重置或限速 Go HTTP 客户端。引擎自动把并发降到 2、关闭分段，并给连接加上空闲超时，避免卡死。

## 安装

从 [Releases](https://github.com/fanhuadesenlinnn/RepoForge/releases) 下载对应系统和架构的压缩包：

| 系统 | 架构 | 格式 | 示例文件 |
| --- | --- | --- | --- |
| Linux | amd64 / arm64 | tar.gz | `repoforge_v1.0.0_linux_amd64.tar.gz` |
| macOS | amd64 / arm64 (Apple Silicon) | tar.gz | `repoforge_v1.0.0_darwin_arm64.tar.gz` |
| Windows | amd64 / arm64 | zip | `repoforge_v1.0.0_windows_amd64.zip` |

**Linux / macOS**：

```bash
sudo tar -C /opt -xzf repoforge_v1.0.0_linux_amd64.tar.gz
export PATH="/opt/repoforge/bin:$PATH"
repoforge init
```

**Windows**（解压 zip 后，把 `bin` 目录加入 PATH）：

```powershell
Expand-Archive repoforge_v1.0.0_windows_amd64.zip -DestinationPath C:\repoforge
$env:Path += ";C:\repoforge\repoforge\bin"
repoforge init
```

## 快速开始

1. 放置 `repo.yaml`（参考 `repo.example.yaml`），填写你想同步/制作的仓库。
2. 运行：

```bash
# 全量镜像配置里启用了 sync 的所有仓库
repoforge sync

# 只镜像某个仓库
repoforge sync --repo rocky-9

# 按需制作离线源（自动求解依赖）
repoforge make --repo rocky-9-install

# 命令行临时追加要装的软件
repoforge make vim curl --repo rocky-9-install

# 生成客户端源配置
repoforge client

# 对照本机已装包制作升级源（需在目标 Linux 上）
repoforge make-upgrade --repo rocky-9

# 列出本地离线源里的包
repoforge list

# 检查环境和仓库状态
repoforge check

# 本机启用 file:// 软件源（Linux，需 root）
sudo repoforge use
```

## 单文件配置示例（`repo.yaml`）

```yaml
schema_version: 2

vars:                        # 全局共享变量（仓库内可覆盖）
  basearch: [x86_64]

paths:
  repo_dir: ${home}/repos    # 全局仓库根（唯一）

repositories:
  # RPM 全量镜像：多架构自动展开成独立子目录
  - name: rocky-9
    backend: rpm
    upstream:
      url: http://mirrors.rockylinux.org/pub/rocky/$releasever/BaseOS/$basearch/os/
      vars:
        - name: releasever
          value: "9"
        - name: basearch
          values: [x86_64, aarch64]   # 多值 → 每个架构一个子目录
    sync:
      enabled: true
      delete_policy: keep   # keep=只增不删；prune=删上游已下线包；prune-expired=只删超过 expire_days 天的下线包

  # RPM 按需制作：只下载指定软件 + 自动依赖求解
  - name: rocky-9-install
    backend: rpm
    upstream:
      url: http://mirrors.rockylinux.org/pub/rocky/$releasever/BaseOS/$basearch/os/
      vars:
        - name: basearch
          value: x86_64
    input:
      packages: [vim, curl, nginx]

  # DEB 多套件/组件/架构
  - name: debian-12
    backend: deb
    upstream:
      url: https://deb.debian.org/debian
      suites:
        - suite: bookworm
          components: [main, contrib]
      arch: [amd64, all]
    input:
      packages: [vim, htop]
```

### 关键配置点

- **变量**：`value` 单值或列表（`value: x86_64` / `value: [x86_64, aarch64]`，直接替换 URL 占位符 `$名字`）；`values` 多值（笛卡尔展开成多组）。
  顶层 `vars` 共享（每项同样支持标量或列表），`upstream.vars` 局部可覆盖。
- **目录**：全局 `paths.repo_dir` 定根；仓库级 `repo_dir` 可选覆盖（为该仓库内容根）；
  多架构/套件自动展开子目录，无需手写。
- **制作方式**：配 `sync` → `repoforge sync` 整仓镜像；配 `input.packages` → `repoforge make` 按需。
- **`input.package_dirs`**：本地已有包目录（可多个），**完整发布进离线源**——解析本地 rpm/deb 自身元数据（含依赖），本地包写入 repodata（客户端可直接安装），其缺失依赖自动从上游下载。
  - 相对路径会先按当前目录、再按 RepoForge home 解析（如 `package_dirs: [tem-rpm-x86]`）。
  - **按架构过滤**：每个多架构 variant 只发布匹配架构的包（`noarch` 总是匹配），其他架构的包会提示跳过。
  - **本地版本优先**：本地包与上游版本冲突时以本地为准（想要上游新版请用 `upgrade_packages`）。
  - 多个目录可分别放不同架构的包；同一目录内也可以混放多架构（如 x86_64 + noarch）。
- **下载性能**：`sync.concurrency` 多文件并行数（默认 8）；`sync.segment_threshold` 每段大小 MiB
  （默认 20）；`sync.segment`（`false`=不分片 / 正整数=单文件分段上限 / 缺省=智能默认8）；
  `sync.resume: true` 断点续传。官方麒麟源会自动降并发、关分段。
- **多仓库聚合**：真实发行版常由多个子仓组成（如 CentOS 的 BaseOS + AppStream，vim 在 AppStream，
  而 glibc 等基础库在 BaseOS）。用 `upstream.sources` 聚合多个源做统一依赖求解并输出到同一目录：

```yaml
  - name: centos8
    backend: rpm
    upstream:
      sources:
        - url: http://mirrors.aliyun.com/centos-vault/8.5.2111/BaseOS/$basearch/os/
          vars: [{ name: basearch, value: x86_64 }]
        - url: http://mirrors.aliyun.com/centos-vault/8.5.2111/AppStream/$basearch/os/
          vars: [{ name: basearch, value: x86_64 }]
      verify: auto   # auto=按上游声明（sha256/sha1/md5）；也可强制 sha256|sha1|md5
    input:
      packages: [vim-enhanced]
    dependency:
      weak_deps: false
      conflicts: report   # report=版本冲突报错；resolve=尝试找满足全部约束的版本
```

## 依赖求解

`make` / `sync` 会从上游仓库元数据中解析依赖关系（RPM 的 Requires/Provides、DEB 的 Depends/Provides），
传递求解并选择合适版本，生成可离线使用的 repodata（RPM 生成 `repomd.xml` + 带 Requires/Provides 的 `primary.xml.gz`，
DEB 生成 `Packages`）。全程不调用本机包管理器。

内部已处理：
- RPM 的库能力匹配（`libc.so.6(GLIBC_2.28)(64bit)` 这类 soname/符号能力，按基础名 + 版本近似匹配）。
- 文件路径依赖（`/usr/bin/bash`、`/sbin/ldconfig` 等，映射到提供它们的包）。
- 多架构过滤（默认 x86_64 + noarch，避免误拉 multilib i686）。
- 多仓库聚合（`upstream.sources`）。

## 架构

```
repo.yaml
 ├─ internal/repo     单文件配置模型 / loader / 变量展开 / 目录推导
 ├─ internal/upstream 上游元数据解析（RPM repomd→primary；DEB suites→Packages）
 ├─ internal/engine   制作引擎（下载/校验/增量/repodata/依赖求解），sync + make
 ├─ cmd               命令层（sync / make / list / check / client / use / server ...）
 └─ 复用基础设施      home / fileutil / render / privilege
```

## 开发

需要 Go 1.23+（本仓库附 `scripts/gdev.sh` 辅助脚本，使用可用的工具链与可写缓存）：

```bash
./scripts/gdev.sh test ./...
./scripts/gdev.sh vet ./...
./scripts/gdev.sh build -o bin/repoforge .
```
