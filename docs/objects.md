# 多 Object 与所有权（E3）

E3 是可选能力，启用 `engine/objects`、`objects/transactional` 和 `objects/owner`，示例见 [objects.yaml](../presets/objects.yaml)。默认 Engine 仍只提供 DefaultObject；新能力的存储/授权由独立插件提供，不扩展 core.Engine、Runner 或状态后端接口。示例使用受审查原生进程，不构成 OS 沙箱；可替换 Runner/Compiler。

## 合约 API

```go
func Create(id string) error {
    o, err := contractapi.CreateObject(id)
    if err != nil { return err }
    return o.Set("balance", []byte("10"))
}
func Read(id string) (string, error) {
    v, _, err := contractapi.OpenObject(id).Get("balance")
    return string(v), err
}
func Transfer(id, to string) error {
    return contractapi.OpenObject(id).Transfer(to)
}
```

- `CreateObject(id)`：ID 在当前合约内唯一；owner 为宿主解析的 Sender。创建不接受自选 owner。
- `OpenObject(id)`：仅构造句柄，每次操作都检查存在和权限。空 ID 无效，不会退回 DefaultObject。
- `Get/Set/Delete`：读、写、删除字段；`Owner()` 查询 owner；`Transfer(to)` 转移；`Destroy()` 销毁整个对象。
- 同一合约范围可以读取对象及 owner；仅 owner 可以写字段、删字段、转移、销毁。缺失对象与授权拒绝是 `OBJECT_FAILURE`，失败整次调用、回滚调用树状态和事件，不能靠忽略宿主错误继续提交。
- 所属合约由 Engine 当前帧的实际地址绑定，不使用 guest 参数或可覆盖的 CallContext.ContractAddress。转移改变 owner，不改变归属。
- 子调用 Sender 是调用合约地址，不继承根账户身份；因此账户调用 A，再由 A 调用 B，不自动获得账户在 B 的所有权。B 可显式把对象转给 A，再允许 A 通过 B 的函数修改。

**根 Sender 必须由集成方认证。** 传入 CallContext/Host 的身份是可信声明，本库不验证签名、账户 nonce 或交易认证。合约决定哪些函数可读取数据；同合约可读不等于给外部调用者提供机密性。自定义授权、状态、Runner 插件及直接数据库访问均在信任边界内。

## 存储与事务

元数据和字段保存在同一事务键；键采用 `StateKey{Contract:实际合约地址, Key:"\xffvm/objects/v1/"+ID}`。前缀含无效 UTF-8 字节，DefaultObject 的 UTF-8 字段接口不能访问；原始 StateTransaction 仍是可信宿主能力。记录为版本1 JSON，带 owner、字段及墓碑。

销毁后保留墓碑，禁止重复创建同 ID，旧句柄不能访问新生对象。首版不枚举对象、不清理墓碑。字段与元数据同原子提交，冲突粒度是整个对象；不同对象可独立提交。同对象的 owner 变更会使旧快照写入产生 MVCC 冲突，重新执行重新鉴权。失败、OOG、取消和 DryRun 均不提交。

与 state/memory、state/mvcc、state/bbolt 组合；持久化无需增加数据库 schema。与 batch/ordered、batch/recoverable 组合时，对象业务失败成为失败项并继续；存储错误、取消和未知基础设施故障仍停止批次。状态、owner、墓碑和恢复回执复用同一落盘事务，不提供整批原子性。

## 资源与协议

| 项目 | 限制 |
|---|---|
| 对象 ID | 1–128 字节 UTF-8（包括合法 NUL） |
| owner | 1–256 字节 UTF-8，地址认证由宿主负责 |
| 字段名 / 值 | 1 KiB / 64 KiB，字段名允许空字符串 |
| 每对象字段数 | 128 |
| 单对象编码记录 | 1 MiB（JSON/base64 开销计入） |
| 每帧对象请求 / 累计回复 | 沿用 1024 次 / 1 MiB |

Gas：Get/Owner=5、Set=10、Delete字段=5、Create=20、Transfer=10、Destroy=10，另有入口和源码插桩费用。宿主验证最低费用，不能通过伪造协议低报。对象消息扩展可选 `id/owner` 字段；没有 ID 的默认对象请求保持原行为。内嵌 runtime 摘要随源码改变，升级后须重新编译部署产物；旧宿主不支持新对象消息。

启用/改变对象授权或实现时更新恢复配置的 `executionID`，不要用旧规则 ID 重放新语义。原始数据库不提供降级写入保护：旧宿主虽不能通过 DefaultObject 改对象记录，但可绕过 owner 规则或忽略对象扩展；不要让旧程序或绕过服务的插件写入含 E3 数据的库。

## 扩展接口

`ObjectService.Bind(StateTransaction,ObjectScope) ObjectSession` 只负责把现有事务绑定到帧；`ObjectSession.Apply/Err/Close` 处理请求、锁存错误并撤销帧能力，不获取或完成事务。`ObjectAuthorizer.Authorize(ctx,scope,owner,op)` 独立表达授权；内建 owner 策略拒绝写操作时返回 `ExecutionError{Code:CodeObjectFailure}`。自定义策略必须保留基础设施错误和取消分类，不能把存储故障当成业务拒绝。

engine/objects 的 deps 为 `components: engine/components` 与 `objects: ObjectService`。需要批量时，以该 Engine 作为 TransactionExecutor，再用现有 engine/batched 或 engine/recoverable 组合门面。服务构造不获取资源；会话共享本次调用的事务，禁止在内部独立 Begin/Commit。
