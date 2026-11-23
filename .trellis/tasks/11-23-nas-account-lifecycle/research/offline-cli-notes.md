# S02.3 离线重置实施准备

日期：2026-11-23。S02.2 正在独立复核，本文件仅补充下一切片准备，不宣称 CLI 已实施。冻结命令和边界以 `../info.md` 为准。

## 已核验的复用点

- `cmd/nas/main.go` 当前直接先 `LoadConfig`，维护命令必须在它之前分派，且分派前创建可响应 Ctrl+C/SIGTERM 的上下文。不能让缺媒体目录或非法继承配置阻止维护命令。
- `internal/app/instance.go` 的 `canonicalDataDirectory` / `acquireInstance` 是在线实例实际独占入口。其规范化支持符号链接/junction/长短路径；维护命令校验原目录已经存在后复用同一规范目录锁，不以 PID/端口推断或强制破锁。
- Windows `acquireInstanceFile` 通过无共享 CreateFile 句柄持有锁，拒绝锁文件重解析点。应持有到数据库关闭后，避免刚释放锁时数据库仍有写者。
- 不能调用 `openDatabase`，因为它包含正常启动迁移/恢复副作用。维护入口单独打开现有普通 `library.db`，显式只支持 schema5、现有单管理员和预期字段，不创建新库/新账号/自动升级或降级。
- `changeAccountPassword` 是在线会话身份核验语义，不能伪造 Cookie 复用它。离线路径应独立短事务 CAS 现有账号 ID/hash/revision、检查溢出、更新hash/revision并删除该账号会话；bcrypt、输入和确认都在事务外。

## 官方终端依赖核验

已用项目隔离环境执行只读模块查询：

```powershell
python -B scripts/project_env.py -- go list -m -json golang.org/x/term@v0.48.0
```

结果为官方模块 `golang.org/x/term v0.48.0`，GoVersion `1.26.0`，来源 `go.googlesource.com/term` 的 tag `refs/tags/v0.48.0`、commit `91b12352ff53e31b89c322da2261ab29e7af83f1`。下载缓存中的该版 `go.mod` 要求 `golang.org/x/sys v0.50.0`，与当前项目已固定版本一致。该查询未修改项目 go.mod/go.sum；实施者可固定此版本，避免无意升级其他依赖。官方 API 文档入口：`pkg.go.dev/golang.org/x/term`。

使用官方无回显读取，保留取消后的终端恢复。特别验证 signal.NotifyContext 下 Ctrl+C 不会让无回显状态遗留，也不会在等待输入时持有数据库写事务。不要只测 EOF 冒称真实终端 Ctrl+C；Windows终端实测、管道取消、Linux交叉编译和Linux实测必须分开记录。

## 输入与失败证据

- 参数只接受固定子命令、必填 `--data-dir` 与显式 `--password-stdin`，不接收 argv/环境中的密码，不在错误中原样回显未知参数值。
- 交互两次、无回显；stdin 模式有界读取一个密码，明确处理一行末尾 LF/CRLF，不 trim 密码本身，不静默截断或取多行的第一行冒称完整输入。取消、输入错误和确认不一致均零账号写入。
- 缺目录、缺库、符号链接/非普通数据库、旧schema/未来schema、无账号/多个账号、锁冲突/目录别名、只读权限、修订溢出和提交取消都要有失败保留证据。
- 对自有隔离实例先演示在线锁冲突且实例仍存活，再停机成功重置、重启证明旧Cookie/旧密码失效和新密码可用；媒体、个人状态、jobs/收集任务与文件不因维护入口被扫描、恢复或清理。
- 原始 stdout/stderr 内存捕获应扫描固定测试密码、bcrypt、Cookie及其摘要，再持久化安全结果。不得用已脱敏日志替代动态原始输出检查。
- 不虚报管理员权限环境下真实ACL拒绝、缺Linux环境下flock/终端行为，必要时记录有界注入测试与实机门禁分别待验。
