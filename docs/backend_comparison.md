# 可插拔执行后端比较（D2，2026-10-26）

## 决策

当前需要执行不可信原生 Go 合约并设置 OS 资源配额时，推荐选择 D1 Docker 编译 + `runner/docker` 配置。WASI/wazero 保留为独立可选插件，适合探索纯 Go 嵌入、明确授权的 WASI 能力边界及跨平台产物；当前每帧重新 JIT 的原型不替换默认后端。ProcessRunner 继续用于可信代码的开发和测试。推荐仅针对当前实现与本机证据，不代表完成生产链的来源认证、持久化或共识验证。

编译器、Runner 和产物校验均由 PluginKit 配置选择，不由 Engine 自动挑选。已实现 `compiler/go`、`compiler/wasip1`，以及 `runner/process`、`runner/docker`、`runner/wazero`。TinyGo、Wasmtime 可作为未来的独立插件，不强制绑定任何一组厂商实现。

## 实测方法

主机为 Apple M4 / darwin arm64，VM 使用 Go 1.26.1、wazero v1.12.0。Docker Engine 28.5.2，通过本机 Linux VM 运行，daemon 可见 10 CPU / 12,590,047,232 字节内存，cgroup v2，内建 seccomp。Docker 与 WASI 都在同一受信任的本地 Go 1.26.9 / linux arm64 镜像中编译，镜像 ID：

`sha256:1c5382a472279e984800fd1adf87c3773bc2b0f9e79771b430b0546098e0f1eb`

Process 开发基线使用本机 Go 1.26.1 编译，不与 Go 1.26.9 的跨平台输出作纯编译器速度比较。Docker 执行限制 1 CPU / 256 MiB / 128 PID；wazero 配置 compiler 引擎 / 256 MiB 线性内存。wazero 不单独设置 OS CPU/PID 配额，使用宿主调度；这也不是等 OS 配额条件下的比较。两者的内存边界不同：Docker 的 cgroup 配额覆盖容器进程及 tmpfs，wazero 的限制仅覆盖 guest 线性内存，不覆盖 JIT 与 Go 宿主。

`assembly/backend_benchmark_test.go` 使用同一份源码和输入，执行完整 Engine 链路（加载、产物校验、事务、创建实例、调用、销毁/清理）。每个 workload 预热一次以验证可执行性，随后采集 3 轮，每轮 3 次调用；下表为三个 `ns/op` 结果的中位数，单位 ms。不是 p50 请求延迟、p95/p99、稳态吞吐或单个函数纯计算时间。样本较少，只用于原型比较；Docker VM/系统噪声可见，不能据此制定 SLO。

每次调用都创建并销毁 guest：Process 启动新进程，Docker create/start/rm，wazero 新建 runtime 并编译/实例化模块。跨合约用例含父/子两个实例。当前没有常驻容器、预热池或跨调用 wazero 编译缓存，不能把这些数据用于推断缓存版运行时的性能。

## 调用结果（ms，轮均值的中位数）

| 完整 Engine 调用 | Process（开发基线） | Docker | wazero |
| --- | ---: | ---: | ---: |
| Add | 27.07 | 300.92 | 644.11 |
| Loop(1000) | 25.53 | 280.18 | 629.66 |
| Object Write | 23.71 | 278.73 | 606.81 |
| 跨合约 Child→Write | 54.57 | 675.05 | 1229.80 |

对 Add，此版本 wazero 完整调用约为 Docker 的 2.14 倍；跨合约约为 1.82 倍。短函数场景主要体现实例生命周期与模块编译成本，不能据此断言 Wasm 指令执行本身慢同样倍数。

## 构建与分配

构建时间每个 backend 仅测一次，是编译缓存未命中的完整 Compile（含工具链探测、ABI/Gas 处理、发布清单）。宿主 Go 缓存为暖缓存，Docker 每次用临时 tmpfs 的冷 Go 缓存，因此这组数值表达当前配置的实际开销，不是相同缓存条件下的编译器性能。

| 项目 | Process | Docker | WASI/wazero |
| --- | ---: | ---: | ---: |
| 一次 Compile（ms） | 510.5 | 9017 | 6374 |
| 产物字节数 | 3,130,066 | 2,903,579 | 3,443,034 |
| Add 宿主 Go 分配字节/op，中位数 | 163,480 | 231,880 | 66,456,578 |
| 跨合约宿主 Go 分配字节/op，中位数 | 286,896 | 469,581 | 132,920,786 |

`B/op` 是测试进程的累计 Go 分配量，既不是峰值 RSS，也不是存活堆。Process/Docker 的数字不包含子进程/容器内分配，wazero 的模块验证/JIT/线性内存则在宿主进程内。因此此表不能用于推断总内存节省比例。当前原型未测真实峰值 RSS；严格部署预算应使用 OS 进程限制与负载测量。

## 安全与语义验证

- WASI 默认不提供宿主文件挂载、网络 socket、宿主环境、真实时钟或宿主随机源；只注入显式 VM 链上下文。未知 Host module import 拒绝。
- `WithMemoryLimitPages` 限制 guest 线性内存；模块输入另限制字节数。JIT/解析和 Go 宿主内存仍由部署方管理，不能把同进程 Wasm 运行时当作 OS 总内存隔离。
- `WithCloseOnContextDone` 可以终止无源码 Gas 插桩的 Wasm 死循环，但截止时间不是编译/验证的硬抢占，也不是指令 fuel。源码 Gas 与宿主 Gas 表继续适用，不信任任意来路的 Wasm 自报 Gas。
- 真实 Go WASI 测试覆盖整数宽度（64 位）、int64/uint64 边界、事件与原生结果/Gas 一致、并发隔离、Object、跨合约共享事务、业务失败/OOG/DryRun 回滚、文件/网络/环境负例、内存和输出超限。
- compiler/wasip1 默认只替换目标，编译仍发生于宿主；需要编译隔离时显式组合 `sandbox`。WASI 产物只被匹配的校验器和 Runner 接受，不提供静默降级。缓存指纹仍包含可信 VM 宿主的工具链/平台身份；WASI 可跨平台执行不代表在不同宿主上重编译会得到相同清单或部署地址。

因此，当前对不可信原生 Go 执行优先推荐 Docker 的 OS 边界；WASI 插件用于纯 Go 嵌入和后续受控优化验证。若未来加入有界编译缓存/常驻 runtime，应重新测量隔离、跨合约生命周期、取消与所有性能指标，再更新推荐，而不是删除其它后端。

## 重跑

原始结果见 [2026-10-26-backends.txt](benchmarks/2026-10-26-backends.txt)。固定镜像存在时：

```sh
VM_DOCKER_IMAGE=sha256:1c5382a472279e984800fd1adf87c3773bc2b0f9e79771b430b0546098e0f1eb \
VM_DOCKER_GO_VERSION=go1.26.9 \
go test ./assembly -run '^$' -bench '^BenchmarkBackend$' -benchtime=3x -count=3 -timeout 10m
```

不设置 Docker 环境变量时仅比较本机 Process 与本机工具链编译的 WASI，Docker 项明确 Skip；不是同一份三后端样本。增加样本数量可调整 `-benchtime` 与 `-count`，必须保留工具链、镜像、资源、是否缓存及并发信息。

API 与资源语义依据 [Go WASI](https://go.dev/blog/wasi) 和固定版本 [wazero v1.12.0 配置源码](https://github.com/tetratelabs/wazero/blob/v1.12.0/config.go)。配置入口见 [插件指南](plugins.md) 与 [`presets/wasi.yaml`](../presets/wasi.yaml)。
