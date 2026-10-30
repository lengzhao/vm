# 编译与执行沙箱（D1）

## 当前能力

默认仍为本机 Go 编译器和 ProcessRunner，适合可信/已审查代码。ProcessRunner 提供超时、Gas、输出和协议限制，不提供 OS 沙箱。D1 新增显式选择的 Docker 编译沙箱及 `runner/docker`；D2 已新增可选 WASI/wazero 插件及实测比较，见 [后端比较](backend_comparison.md)；当前仍按场景由配置选择后端。

Docker 路径复用相同的源码策略、ABI、Gas、双向 Host 协议、Object 与跨合约事务。失败、取消、输出超限或清理失败不提交事务及事件。不自动拉取镜像，不自动降级宿主。

## 启用

需要本机 Linux Docker daemon，macOS 可通过 Docker VM 运行。daemon 必须支持 cgroup 的 CPU/内存/PID 限制及默认 seccomp；daemon 和可信镜像属于管理员控制的信任边界。远程 daemon 的宿主挂载路径不由本项目同步，不支持把任意远程 Docker endpoint 当作可移植沙箱服务。

先自行准备受信任的 Go Linux 镜像，再查本地身份与版本。例如镜像已存在时：

```sh
docker image inspect golang:1.26 --format '{{.Id}}'
docker run --rm --network none --entrypoint /usr/local/go/bin/go golang:1.26 version
```

将 [`presets/docker.yaml`](../presets/docker.yaml) 中两处 `IMAGE_ID` 替换为完整 `sha256:...` 本地镜像 ID，将 `GO_VERSION` 替换为实际版本（如 `go1.26.9`），然后：

```sh
go run ./examples/plugins presets/docker.yaml
```

镜像中的 Go 路径须为 `/usr/local/go/bin/go`，并提供 `/bin/sh` 与 `sha256sum` 用于工具链探测。只支持 Linux / 当前宿主架构，禁止 CGO。所有模式继续校验清单内容、runtime/Gas 规则、部署身份。Docker 编译产物的目标是 Linux，在 macOS 上不能交给 ProcessRunner。默认 `policy/artifact` 也拒绝非本机目标，沙箱图需显式设置 `targetOS: linux`。

## 隔离与资源

两种容器均使用：非 root UID/GID 65532、只读根目录、禁网、移除全部 Linux capabilities、no-new-privileges、私有 PID/IPC/cgroup namespace、Docker 默认 seccomp、最多 256 个文件描述符、禁用健康检查与 Docker 日志落盘。镜像中已有文件仍可读取；不提供宿主目录或 Docker socket。出网和宿主文件访问不依赖 AST 审查开关。

| 资源 | 编译默认 | 执行默认 |
| --- | --- | --- |
| 内存（含 tmpfs） | 1024 MiB | 256 MiB |
| 内存 + swap 上限 | 同内存上限 | 同内存上限 |
| CPU 配额 | 1 核 | 1 核 |
| PID（含线程） | 256 | 128 |
| `/tmp` tmpfs | 512 MiB | 64 MiB |
| 操作超时 | 2 分钟 | 30 秒 |

`/tmp` 为 nosuid/nodev/noexec，容量受限。编译源目录仅单次构建可见且只读，只有单次产物目录可写；构建缓存位于临时 tmpfs，不挂载宿主 Go 缓存或凭据。Go 编译/汇编/链接子进程都在该容器内。Go 版本及实际 Go 二进制摘要通过同样的受限容器探测。编译镜像 ID、Linux 目标和实际工具链进入产物身份；缓存命中仍需校验工具链及产物。

执行先复制二进制到私有快照并重新校验摘要，再仅只读挂载这一个文件；不暴露部署目录。只转发显式 VM 链上下文和 Gas 环境，不转发宿主 PATH 或其它宿主环境。每个跨合约帧独立容器，整链继续共享宿主事务和 Gas；单容器资源限制之外，整链受深度、Host 请求次数和根截止时间限制。

容器资源错误属于节点执行失败；CPU 配额/超时不是共识 Gas 的替代。默认资源数值是开发基线，需要集成方按负载调整。

## 生命周期与故障

每次先创建随机名 `vm-d1-...` 的停止容器，再 attach/start。正常完成、运行失败、取消、输出超限都强制删除整个容器；在删除完成前不返回成功。清理使用独立 10 秒预算，清理失败向上传播，Engine 回滚整链状态及事件。

创建控制请求使用独立 15 秒预算，以免调用方取消直接截断 daemon 创建事务。调用方取消可能额外等待创建和清理，因此取消不是即时操作。创建 CLI 超时无法证明 daemon 未迟到创建：明确返回 `outcome uncertain` 并附随机容器名，绝不 start 或自动重试执行。daemon 断连/不可用时也不能保证已清理，管理员需恢复连接后按错误中的**具体容器名**核对并清理。迟到创建的停止容器不会自行执行合约。

进程异常退出、宿主崩溃或 daemon 故障不提供后台自动回收服务。应由集成方治理残留容器及整体请求并发。Docker daemon、镜像、宿主内核和可信 Go 前端（源码解析、审查、插桩、清单处理）仍在信任边界内；这里不承诺防御内核或 Docker 漏洞，也不构成最终生产后端认证。

## 验证

普通测试不依赖 Docker；真实隔离测试需显式启用，否则显示 Skip：

```sh
VM_DOCKER_IMAGE=sha256:<本地完整镜像ID> \
VM_DOCKER_GO_VERSION=go1.26.9 \
go test -p 1 -timeout 5m ./...
```

覆盖真实构建与缓存、编译/执行目标、文件访问拒绝、非 root、出网拒绝、CPU/内存/PID 参数、tmpfs 耗尽、OOM、实际 PID 耗尽、取消后的后代清理，以及 Object/跨合约事务成功与失败回滚。故障注入验证创建失败/超时不启动、清理失败传回 Runner 并阻止 Engine 发布状态和事件。

完整验证还包括同环境 `go test -race -p 1 -timeout 5m ./...`、`go vet ./...` 和 `git diff --check`。参数语义依据 [Docker CLI](https://docs.docker.com/reference/cli/docker/container/run/) 与 [默认 seccomp](https://docs.docker.com/engine/security/seccomp/)。

## 持续验证与完整组合

`.github/workflows/docker.yml` 在 Linux 上准备与 go.mod 相同版本的官方 Go 镜像，解析本地不可变 ID 和实际 Go 版本后设置两个 VM_DOCKER 环境变量，依次运行真实普通/race 隔离、事务和对象恢复集成测试，并检查测试容器残留。镜像标签只用于 CI 准备阶段；VM 实际配置始终使用完整 ID，生产应自行审核固定镜像来源。

严格策略+持久化+对象授权+可恢复批次+Docker 的组合模板为 `presets/objects-docker.yaml`；开发者指南说明替换占位符和运行方式。
