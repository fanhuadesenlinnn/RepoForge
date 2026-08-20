# Changelog

## Unreleased

## v1.1.3

- 修复：多架构仓库的 `basearch` 声明在 `sources.vars` 时（如麒麟源双源配置），各架构 variant 不再写入同一目录互相覆盖 repodata——按架构生成独立子目录（`<repo>/x86_64/`、`<repo>/aarch64/`），各自含完整 repodata。

## v1.1.2

- 修复：多架构仓库（`$basearch` 展开为 x86_64 + aarch64 等）依赖求解时架构过滤错误——aarch64 variant 会被按 x86_64 过滤导致选 0 包、报“无法满足依赖”。`archList` 现在从展开变体的 `$basearch` 推断架构。

## v1.1.1

- 修复：Windows 平台路径处理——`input.package_dirs` 不再把 `C:\...` 按 `:` 拆错盘符；`${home}` 前缀路径统一分隔符。
- 修复：CI 在 Windows 上因 CRLF 换行导致 gofmt 检查误报（新增 `.gitattributes` 强制 LF）。

## v1.1.0

- 新增：GitHub 流水线支持 Windows / macOS——CI 在三个平台跑测试与冒烟测试，打包 Linux/Darwin/Windows × amd64/arm64 共 6 种发布产物（Windows 为 zip，含 repoforge.exe）。
- 新增：变量语法放宽——`value: [x86_64, aarch64]` 列表写法与全局 `vars` 标量写法均可用，与 `values:` 等价；多架构配置不再需要 `values:` 字段。
- 文档：README 安装章节补充 macOS / Windows 安装示例。


首个稳定大版本：只保留单文件 `repo.yaml` + 纯 Go 引擎，不再依赖本机 dnf/apt/createrepo。

- 破坏性变更：移除 `config.yaml` / `packages.yaml` / profiles，以及本机 dnf·apt 后端。`list` / `check` / `make-upgrade` 改为只读 `repo.yaml`；`--profile` 已删除。
- 新增：`sync` 结束后生成完整 yum/apt 索引；`make`/`sync` 写出的 primary.xml 包含 Requires/Provides，离线 dnf/yum 能解依赖。
- 新增：sync/make 输出元数据与下载进度。
- 新增：`make-upgrade` 用本机 rpm/dpkg 对照上游后走新引擎下载并生成索引。
- 修复：官方麒麟镜像（`update.cs2c.com.cn` 等腾讯 EdgeOne CDN）导致卡住或极慢——补齐超时、自动降并发、关分段、流式解析元数据。

## v0.3.0

- 新增：`make-upgrade` 命令，支持为 RPM 系统下载当前机器升级所需的软件包并生成离线升级源。
- 新增：`list` 命令，可查看本地软件源目录中已有的软件包。
- 调整：README 快速开始和常用命令示例补充 `v0.3.0` 发布包、`make-upgrade` 与 `list` 的用法。

## v0.2.1

- 新增：`packages.yaml` 预置常用基础包（vim、curl、wget、telnet、nc），开箱即用。
- 调整：`packages.yaml` 注释更友好，每个包标注用途。

## v0.2.0

- 新增：下载软件包时实时显示进度（apt-get / dnf/yum 输出流式回显）。
- 新增：支持从 RPM 输入目录导入已有 rpm 并自动补齐缺失依赖。
- 新增：init 默认生成 11 个主流系统 profile（麒麟/CentOS/Rocky/openEuler/Debian/Ubuntu）。
- 调整：默认不禁用所有仓库，`disable_repos` 改为空列表。
- 调整：配置字段 `rpm_dirs` 重命名为 `package_dirs`，移除 `copy_input_rpms`（始终复制）。
- 调整：DEB 后端用 `dpkg-deb` 提取包名后按包名下载，兼容所有 apt 版本。
- 调整：profile 配置大幅简化（50行→7行），路径自动推导，兼容性检查放宽。
- 调整：`make`/`use`/`check` 无需 `--profile`，自动匹配当前系统。
- 调整：`packages.yaml` 模板精简，用户只添加需要的包名即可。

## v0.1.0

- 提供目录自包含的 RepoForge CLI 和幂等初始化。
- 支持 Linux 系统、架构、profile 兼容性和依赖检查。
- 支持 RPM 与 DEB 离线软件源制作及本机启用。
- 支持只读 HTTP 分发、systemd 管理和客户端配置生成。
- 提供 Linux amd64/arm64 自动构建与 GitHub Release 流水线。
