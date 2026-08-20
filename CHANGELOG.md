# Changelog

## v1.4.1

- 修复：RPM 源未声明架构时静默按 x86_64 过滤，导致 aarch64/arm64 源（如麒麟 `.../base/aarch64/`）解析出空仓库。现在按优先级：`upstream.arch` → `target.arch` → `$basearch` 变量 → **从 URL 推断架构**（`aarch64`/`arm64`/`x86_64`/`amd64`）；URL 也无法推断时不按架构过滤，绝不静默产出空仓。

## v1.4.0

- 新增：OpenPGP 签名（纯 Go，无 gpg 依赖）——`repoforge gpg init` 生成 Ed25519 密钥对，`gpg export` 导出公钥；`signing.enabled: true` 后 sync/make 自动签名：RPM 生成 `repomd.xml.asc`（yum repo_gpgcheck），DEB 生成 `Release` / `InRelease` / `Release.gpg`（apt Signed-By）。
- 新增：zstd 压缩元数据支持——Fedora 39+ / openSUSE / 新版 EPEL 的 primary.xml.zst 与 DEB Packages.zst 均可解析（此前直接报错）。
- 新增：`sync.delete_policy: prune-expired` 真正实现——配合 `sync.expire_days`（缺省 30 天），只删除上游已下线且本地副本超过宽限期的包，给客户端缓存留出缓冲期（此前与 prune 行为相同）。
- 新增：`dependency.conflicts: resolve`——两个包把同一依赖钉在不同版本时，尝试寻找同时满足全部约束的单一版本；无法满足才报错（report 维持原行为）。
- 新增：`upstream.verify` 落实——`auto`（缺省）按上游元数据声明的算法校验（sha256/sha1/md5），也可强制 `sha256|sha1|md5`；生成的 primary.xml / Packages 按实际算法输出。
- 修复：`upstream.verify` 生成的 primary.xml checksum type 此前硬编码 sha256，sha1/md5 上游会导致客户端校验失败——现已按实际算法输出。
- 变更：`init` 不再生成无用的 client/local 源模板（rpm-local/rpm-client/deb-local/deb-client）——`use`/`client` 命令本就硬编码生成配置，这些模板从未被读取；systemd 服务单元改为直接从二进制内置读取，`server enable` 不再依赖 `config/templates` 目录（未 init 也能用）。`init` 现在只生成一个配置文件 `config/repo.yaml`。
- 移除：`paths.template_dir` 配置项与 `config/templates` 目录结构。
- 跨平台：CI 测试矩阵扩为 Linux/macOS/Windows × amd64/arm64 全 6 组合真实运行时测试；交叉编译与打包（tar.gz/zip）已实测验证。
- 清理：删除 staticcheck/deadcode 发现的 11 处死代码与 2 个死配置字段（`server.readonly`、`local.enabled_external`）。

## v1.3.0

- 新增：跨架构补全——`input.package_dirs` 只含一种架构时，其他架构自动从上游拉取同名包 + 依赖（`input.packages`/`upgrade_packages` 同样支持）；上游没有对应架构版本的第三方包降级为提示，不再报错。
- 性能：依赖求解索引化（provides 完整字符串哈希，yum 式）——大源（2 万+ 包）依赖树求解从卡死/数分钟降至秒级。
- 新增：元数据缓存（yum/apt 式）——primary/filelists 按 repomd sha256 缓存原始文件 + 解析结果（gob），二次运行跳过下载与 XML 解析，元数据阶段从 1-2 分钟降至几秒。

## v1.2.0

- 新增：`input.package_dirs` 本地包完整发布——解析本地 rpm/deb 文件自身元数据（名称/版本/架构/依赖），本地包直接进依赖求解并写入 repodata，其缺失依赖自动从上游下载。第三方/内网包不再只是“复制文件”，客户端 yum/apt 可直接安装。
- 变更：本地包与上游版本冲突时**以本地版本为准**（不再改用上游版本），上游版本有差异时不再提示切换。
- 新增：本地 deb 解析（ar 归档 + control，支持 gz/xz/zst 压缩）；rpm 解析使用纯 Go 库（无需本机 rpm/createrepo）。
- 新增依赖：`cavaliergopher/rpm`、`ulikunitz/xz`、`klauspost/compress`。

## v1.1.6

- 修复：`input.package_dirs` 的本地包与上游重复下载——本地副本复制在输出根目录（flat），上游下载在 `Packages/` 下，导致同包出现两份且 flat 副本不在 repodata。现在本地包与上游同名（同 NEVRA）时移到上游位置进 repodata；版本不同时删除本地副本、采用上游版本并提示。

## v1.1.5

- 新增：`input.package_dirs` 提示与架构感知——扫描目录时提示找到/复制/跳过数量；多架构 variant 只复制架构匹配的本地包（`noarch` 总是匹配），不匹配的提示跳过（如 `跳过 4 个架构不匹配的包（x86_64: 4）`），避免 x86_64 包混入 aarch64 目录。多个目录可分别放不同架构的包，同一目录也可混放。
- 新增：`package_dirs` 相对路径支持——先按当前目录解析，找不到再按 RepoForge home 解析。

## v1.1.4

- 新增：下载进度条展示——终端（TTY）下每个包完成时用 `\r` 刷新显示 `[下载] ████░░ 42% 42/92  完成: xxx.rpm`；重定向/日志时保持逐行输出。

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
