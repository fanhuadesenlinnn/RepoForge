# Changelog

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
