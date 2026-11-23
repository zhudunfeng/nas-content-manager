# S02 实施前源码复核

日期：2026-11-23。此文件是 S01 实施期间的只读源码复核，尚未实现 S02，也不代表任何账号行为已通过验收。

## 已核对的复用入口

- CodeGraph `query createSession`、`decodeJSON`、`routes`、`acquireInstanceFile` 定位后读取当前文件。
- `internal/app/auth.go`：`validNewPassword` 已有精确的 10–72 ASCII 四类规则；`login` 仅校验旧 bcrypt、不 trim 密码。`createSession` 当前只按 userID 插入，会话没有公开 ID、修订或创建时间，因此需要在 bcrypt 比较后的插入点增加账号快照条件。
- `internal/app/server.go`：`auth` 当前把安全 User 放进 context；账号敏感写仍须在提交事务内重查有效会话。`security` 已提供同源写检查与 64 KiB JSON 上限，`decodeJSON` 拒绝未知字段和尾随对象，但标准 decoder 不拒绝重复字段；S02 需要局部补齐，避免顺带改变全部旧 API。
- `internal/app/store.go`：当前最大 schema 为 4；增量迁移顺序在一个连接中执行。`openDatabase` 同时清缓存、恢复 jobs/外部动作，离线账号命令不能复用它。新增版本时需搜索旧 verifier 的版本断言并准确升级，不放松其保留断言。
- `internal/app/instance.go`：`canonicalDataDirectory` 和 `acquireInstance` 统一目录身份。Windows `acquireInstanceFile` 零共享打开非重解析点锁；Linux 使用 `O_NOFOLLOW` + 非阻塞 flock。复用锁，不删除锁文件、不靠 PID 或端口判断占用。
- `cmd/nas/main.go`：当前没有参数分派；维护子命令必须在 `LoadConfig` 和服务初始化前截获，避免要求媒体目录或启动服务。
- `web/src/App.vue`：设置页已有 settings-panel；`session-expired` 既有处理负责返回登录。账号安全可做局部组件，不复制页面导航/播放器。`PasswordInput.vue`、`api.ts`、`password.ts` 提供输入、401 和同策略校验。

## 需在实现时冻结的决策

1. 当前 schema 基于 4，S02 如无其他迁移插入使用下一版本；不要直接改 schemaV1 或清理旧会话来规避迁移。旧会话创建时间保留 null，公开随机 ID 与 bearer token 完全独立。
2. 登录使用 `INSERT ... SELECT ... WHERE password_hash=? AND auth_revision=?` 或等价事务 CAS；只在影响一行后设置 Cookie。初始化管理员也传递所创建的实际快照，避免和离线/在线改密出现不一致的调用路径。
3. 改密/revoke-others 的 bcrypt 比较在事务外，事务内再次校验用户 hash/修订、当前会话存在且未过期。双改密只有一次提交；撤销后新请求失败，不强杀已准入媒体流。
4. 会话客户端描述优先由 UA 映射为有限浏览器/系统类别，避免保存原始 UA/IP。分页使用独立公开 ID 的稳定顺序与有界 cursor；列表不返回摘要。
5. 离线写仅打开已有普通 SQLite 文件，显式读写模式且不创建空库，确认实际 schema/管理员后操作。交互需要无回显；显式 stdin 模式有界读取，不从 argv/环境取密码。当前依赖有 x/sys，尚无 x/term；选实现前核对依赖，不复制完整终端库。
6. 离线命令要持有同一 canonical 数据锁直到 DB 关闭；不调用完整服务/任务恢复，不接触 cache 或媒体。实际 Linux 内核测试若不可用，交叉构建与 Windows 锁实测分开记录。

## 需要保留的验收

- 复用原密码矩阵，增加改密、旧弱/中文 currentPassword、前后端同值、失效 Cookie、会话分页/撤销、重启，以及媒体 Range 不回退。
- 用临时 Go 包内用例精确制造“bcrypt 已通过但 CAS 尚未执行”的登录竞态和敏感提交竞态；执行后清除本次生成的测试源码。长期 HTTP/CLI 行为断言放可复用集成脚本。
- 错误体、日志、DTO 和公开证据不得出现密码、token/hash、原 UA、私密绝对路径。跨站、重复/未知字段、超大正文、猜测限流和中断保留状态都要有断言。
