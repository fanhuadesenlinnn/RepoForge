# RepoForge

RepoForge 是一个用于离线、内网和隔离网络环境的 Linux 软件源构建与只读分发工具。

它把配置、软件包、索引、客户端配置和可执行文件放在同一个目录中。在线机器制作完成后，将整个目录复制到离线机器即可使用。

## 支持范围

- RPM 系统：使用 `dnf` 或 `yum` 下载依赖，使用 `createrepo_c` 生成索引。
- RPM 系统升级源：使用 `dnf upgrade --downloadonly` 或 `yum update --downloadonly` 下载当前系统升级所需 RPM 包，并使用 `createrepo_c` 生成索引。
- DEB 系统：使用 `apt-get` 下载依赖，使用 `dpkg-scanpackages` 生成索引。
- HTTP 分发：使用 Go 标准库提供仅允许 GET/HEAD 的文件服务。
- 服务管理：使用 systemd 安装、启停和查看 HTTP 服务。

制作离线源时，应使用与目标机器相同的发行版、版本和架构。
制作系统升级离线源时，应使用与目标机器相同发行版、版本、架构、已安装包状态尽量一致的在线机器。

## 快速开始

从 [Releases](https://github.com/fanhuadesenlinnn/RepoForge/releases) 下载对应架构的压缩包，并解压到 `/opt`：

```bash
sudo tar -C /opt -xzf repoforge_v0.3.0_linux_amd64.tar.gz
sudo /opt/repoforge/bin/repoforge init
```

编辑软件包列表（添加你需要的包）：

```bash
sudo vi /opt/repoforge/config/packages.yaml
```

在在线机器制作软件源（自动匹配当前系统 profile）：

```bash
/opt/repoforge/bin/repoforge make
```

在在线机器制作当前系统升级离线源：

```bash
/opt/repoforge/bin/repoforge make-upgrade
```

查看本地软件源中已有的软件包：

```bash
/opt/repoforge/bin/repoforge list
```

将整个目录复制到离线机器：

```bash
scp -r /opt/repoforge root@offline-host:/opt/repoforge
```

在离线机器启用本机软件源：

```bash
sudo /opt/repoforge/bin/repoforge use
```

将离线机器作为局域网软件源：

```bash
sudo /opt/repoforge/bin/repoforge server enable
/opt/repoforge/bin/repoforge server status
```

## 常用命令

```text
repoforge init [--force]                 初始化目录和默认配置
repoforge check                          检查环境、匹配 profile、仓库状态
repoforge make                           下载包及依赖并生成软件源索引
repoforge make --profile NAME            指定 profile 制作
repoforge make-upgrade                   下载当前系统升级所需 RPM 包并生成软件源索引
repoforge make-upgrade --profile NAME    指定 profile 制作系统升级离线源
repoforge list                           列出本地软件源中的软件包
repoforge list --profile NAME            指定 profile 列出软件包
repoforge use                            启用本机 file:// 软件源
repoforge use --disable                  禁用本机软件源
repoforge use --disable --remove         禁用并删除配置文件
repoforge server start                   前台启动只读 HTTP 服务
repoforge server enable                  安装并启动 systemd 服务
repoforge server stop                    停止 systemd 服务
repoforge server disable                 禁用并删除 systemd 服务
repoforge server status                  查看服务和可用 profile
```

## 运行目录

```text
/opt/repoforge/
├── bin/repoforge
├── config/
│   ├── config.yaml
│   ├── packages.yaml
│   ├── profiles/
│   └── templates/
├── repos/
├── cache/
├── client/
└── logs/
```

`repoforge init` 可重复执行。默认不会覆盖已有配置；只有 `repoforge init --force` 会更新受管的默认配置和模板。已有 `repos/` 软件包不会被删除。

## 系统修改

以下命令需要 root 权限：

- `repoforge use`：写入 profile 中配置的 yum/dnf 或 apt 源文件。
- `repoforge server enable`：写入 systemd unit 并启用服务。
- `repoforge server stop`、`repoforge server disable`：管理 systemd 服务。

RepoForge 不修改其他软件源配置。禁用或删除时只处理自己管理的源文件和 systemd unit。

## 从已有 RPM 目录补齐依赖

如果已有一批 rpm 包，可以在 profile 中配置：

```yaml
input:
  package_dirs:
    - /root/input-rpms
  recursive: false
```

然后执行：

```bash
repoforge make --profile kylin-v10-sp3-x86_64
```

RepoForge 会：

1. 扫描输入目录中的 rpm；
2. 复制到当前 profile 的软件源目录；
3. 调用 dnf/yum 下载这些 rpm 缺失的依赖；
4. 生成 createrepo_c 索引。

## 制作系统升级离线源

RPM 系统可以使用 `make-upgrade` 下载当前系统升级所需 RPM 包：

```bash
repoforge make-upgrade --profile kylin-v10-sp3-x86_64
```

RepoForge 会：

1. 使用当前系统已安装包状态计算可升级包；
2. 调用 `dnf upgrade --downloadonly` 或 `yum update --downloadonly` 下载 RPM 包；
3. 下载目录使用当前 profile 的 `repository.package_dir`；
4. 调用 `createrepo_c --update` 生成 yum/dnf 软件源索引；
5. 校验 `repodata/repomd.xml`。

注意：`make-upgrade` 不使用 `--installroot`。如果在线制作机与离线目标机已安装包状态差异较大，生成的升级源可能无法完整覆盖目标机升级需求。

## 查看本地软件源软件包

可以使用 `list` 查看当前 profile 软件源目录中的软件包：

```bash
repoforge list --profile kylin-v10-sp3-x86_64
```

RPM backend 会优先使用 `rpm -qp` 读取 RPM 包头，并输出包名、版本、发布号、架构、大小和文件名。DEB backend 当前按 deb 文件名和大小输出。

## 客户端使用

启用 HTTP 服务后，客户端配置会生成到 `/opt/repoforge/client/`。

RPM 客户端：

```bash
sudo cp /opt/repoforge/client/repoforge-kylin.repo /etc/yum.repos.d/
sudo dnf makecache --disablerepo="*" --enablerepo="repoforge-lan"
sudo dnf install --disablerepo="*" --enablerepo="repoforge-lan" vim curl
```

DEB 客户端：

```bash
sudo cp /opt/repoforge/client/repoforge-debian.list /etc/apt/sources.list.d/
sudo apt-get update
sudo apt-get install vim curl
```

## 故障排查

先执行只读检查：

```bash
/opt/repoforge/bin/repoforge check --profile debian-12-amd64
```

查看服务状态和局域网地址：

```bash
/opt/repoforge/bin/repoforge server status
sudo systemctl status repoforge-server
```

端口占用时：

```bash
ss -lntp | grep 8080
```

也可以修改 `/opt/repoforge/config/config.yaml` 中的 `server.listen` 和 `server.public_url`。

## 开发

需要 Go 1.23 或更高版本：

```bash
go test ./...
go vet ./...
go build -o bin/repoforge .
```

项目的详细实现计划仅保留在源码仓库中，不随发布包分发。
