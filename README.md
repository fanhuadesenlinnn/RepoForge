# RepoForge

RepoForge 是一个**跨平台（Windows / Linux / macOS）**的离线 Linux 软件源构建与分发工具。

它用**单文件配置 `repo.yaml`** 描述一切：多个仓库、上游 URL（支持变量占位符和多值架构）、制作方式
（全量镜像 `sync` / 按需制作 `install`）。引擎**纯 Go 实现，不依赖本机 dnf/apt/createrepo_c**，
因此可以在任意机器上制作面向任意发行版的离线 yum/apt 源。

## 核心能力

- **`sync`**：全量镜像上游仓库（整仓拉取、增量、校验）。
- **`install`**：按需制作离线源——只下载指定软件及其依赖（RPM 与 DEB **自动依赖求解**）。
- **`client`**：按 `repo.yaml` 生成客户端 yum/apt 源配置。
- **`use`**：启用/禁用本机 file:// 软件源。
- 单文件配置、多仓库、多架构自动展开、变量占位符。

### 下载能力

- **多文件并行**：多个包同时下载（`sync.concurrency`，默认 8）。
- **单大文件智能分段**：段数按文件大小自动计算——`ceil(size/segment_threshold)` 且上限 `max_segments`。
  例如 20 MiB 每段：40MiB→2 段、100MiB→5 段、200MiB→8 段(封顶)；≤20MiB 不分段走单连接。
- **断点续传**：中断后从断点继续，不重下（`sync.resume: true`）。
- **增量**：已存在且校验匹配的包跳过；孤儿 `.part` 自动清理。

## 快速开始

1. 放置 `repo.yaml`（参考 `repo.example.yaml`），填写你想同步/制作的仓库。
2. 运行：

```bash
# 全量镜像配置里启用了 sync 的所有仓库
repoforge sync

# 只镜像某个仓库
repoforge sync --repo rocky-9

# 按需制作离线源（自动求解依赖）
repoforge install --repo rocky-9-install

# 命令行临时追加要装的软件
repoforge install vim curl --repo rocky-9-install

# 生成客户端源配置
repoforge client

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
      delete_policy: keep

  # RPM 按需制作：只下载指定软件 + 自动依赖求解
  - name: rocky-9-install
    backend: rpm
    upstream:
      url: http://mirrors.rockylinux.org/pub/rocky/$releasever/BaseOS/$basearch/os/
      vars:
        - name: basearch
          value: x86_64
    install:
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
    install:
      packages: [vim, htop]
```

### 关键配置点

- **变量**：`value` 单值（直接替换 URL 占位符 `$名字`）；`values` 多值（笛卡尔展开成多组）。
  顶层 `vars` 共享，`upstream.vars` 局部可覆盖。
- **目录**：全局 `paths.repo_dir` 定根；仓库级 `repo_dir` 可选覆盖（为该仓库内容根）；
  多架构/套件自动展开子目录，无需手写。
- **制作方式**：配 `sync` → `repoforge sync` 整仓镜像；配 `install.packages` → `repoforge install` 按需。
- **下载性能**：`sync.concurrency` 多文件并行数（默认 8）；`sync.segment_threshold` 每段大小 MiB
  （默认 20，段数自动算）；`sync.max_segments` 单文件分段上限（默认 8）；`sync.resume: true` 断点续传。
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
      verify: sha256
    install:
      packages: [vim-enhanced]
    dependency:
      weak_deps: false
```

## 依赖求解

`install` 会从上游仓库元数据中解析依赖关系（RPM 的 Requires/Provides、DEB 的 Depends/Provides），
传递求解并选择合适版本，生成可离线使用的 repodata（RPM 生成 `repomd.xml` + `primary.xml.gz`，
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
 ├─ internal/engine   制作引擎（下载/校验/增量/repodata/依赖求解），sync + install
 ├─ cmd               命令层（sync / install / client / use / server ...）
 └─ 复用基础设施      home / fileutil / render / privilege
```

## 开发

需要 Go 1.23+（本仓库附 `scripts/gdev.sh` 辅助脚本，使用可用的工具链与可写缓存）：

```bash
./scripts/gdev.sh test ./...
./scripts/gdev.sh vet ./...
./scripts/gdev.sh build -o bin/repoforge .
```
