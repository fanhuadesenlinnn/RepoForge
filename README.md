# RepoForge

RepoForge 是一个用于离线、内网和隔离网络环境的 Linux 软件源构建与只读分发工具。

它把配置、软件包、索引、客户端配置和可执行文件放在同一个目录中。在线机器制作完成后，将整个目录复制到离线机器即可使用。

## 支持范围

- RPM 系统：使用 `dnf` 或 `yum` 下载依赖，使用 `createrepo_c` 生成索引。
- DEB 系统：使用 `apt-get` 下载依赖，使用 `dpkg-scanpackages` 生成索引。
- HTTP 分发：使用 Go 标准库提供仅允许 GET/HEAD 的文件服务。
- 服务管理：使用 systemd 安装、启停和查看 HTTP 服务。

制作离线源时，应使用与目标机器相同的发行版、版本和架构。

## 快速开始

从 [Releases](https://github.com/fanhuadesenlinnn/RepoForge/releases) 下载对应架构的压缩包，并解压到 `/opt`：

```bash
sudo tar -C /opt -xzf repoforge_v0.1.0_linux_amd64.tar.gz
sudo /opt/repoforge/bin/repoforge init
```

编辑软件包列表：

```bash
sudo vi /opt/repoforge/config/packages.yaml
```

在在线机器制作软件源：

```bash
/opt/repoforge/bin/repoforge check --profile kylin-v10-sp3-x86_64
/opt/repoforge/bin/repoforge make --profile kylin-v10-sp3-x86_64
```

将整个目录复制到离线机器：

```bash
scp -r /opt/repoforge root@offline-host:/opt/repoforge
```

在离线机器启用本机软件源：

```bash
sudo /opt/repoforge/bin/repoforge use --profile kylin-v10-sp3-x86_64
```

将离线机器作为局域网软件源：

```bash
sudo /opt/repoforge/bin/repoforge server enable
/opt/repoforge/bin/repoforge server status
```

## 常用命令

```text
repoforge init [--force]                 初始化目录和默认配置
repoforge check [--profile NAME]         只读检查环境、依赖和仓库状态
repoforge make --profile NAME            下载包及依赖并生成软件源索引
repoforge use --profile NAME             启用本机 file:// 软件源
repoforge use --disable --profile NAME   禁用本机软件源
repoforge use --disable --remove --profile NAME
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

项目的详细实现计划见 [repoforge_codex_execution_plan.md](repoforge_codex_execution_plan.md)。
