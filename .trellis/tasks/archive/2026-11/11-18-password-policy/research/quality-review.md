# 密码规则独立质量复核

复核日期：2026-11-18。复核范围为密码规则实现及验证脚本凭据调用；文档、规范、页面和完整媒体集成检查由主代理负责。

> 首轮历史复核：此报告早于用户追加“不可以包含中文”，当时只要求四类 ASCII 字符，允许附加中文或空格。当前规则禁止任何 U+0021–U+007E 之外的字符；本轮复核及验证以 `ascii-quality-review.md` 与 `ascii-implementation-result.md` 为准。

## Findings (fixed)

未发现需要修复的实现问题。本次复核未修改生产代码或覆盖 `web/dist`。

## Findings (not fixed)

无。

## 契约与实际代码路径

- `internal/app/auth.go` 的 `validNewPassword(password string) bool` 使用 Go 字符串字节长度、有效 UTF-8 和四类 ASCII 字符校验。四段可打印 ASCII 标点范围覆盖全部 32 个标点；中文、全角符号、空格和表情不能替代必需类别。
- `/api/setup` 在生成 bcrypt 哈希和写用户表前拒绝无效密码，返回中文 400。用户名与密码分开校验；没有 trim 或截断密码，日志只记录校验失败事实。
- `/api/login` 沿用原始 bcrypt 匹配和 72 字节上界，没有套用新复杂度；存量密码哈希与数据库结构均不修改。
- `web/src/password.ts` 的 TextEncoder 字节长度和 ASCII 类别与后端对应；`App.vue` 仅在 `setupRequired` 时调用新规则，保留确认密码和旧账户登录。
- 五个验证脚本所有应用 `/api/setup` 自动生成凭据均复用 `generate_password() -> str`。固定四类前缀加安全随机后缀不会随机缺少类别；外部/保存的旧应用密码继续原样使用。qB 引擎密码、SID 与搜索来源密钥没有误套应用规则。
- 新命名函数均有中文方法注释和显式参数/返回类型；新增凭据字段说明、关键业务行注释和受控中文日志符合项目要求。

## Verification

- `gofmt -l internal/app/auth.go`：无输出，通过。
- `.tools/go/bin/go.exe vet ./...`：通过。
- `npm --prefix web run typecheck`：严格 vue-tsc 通过。
- 设置 `NAS_PASSWORD_CASES_FILE` 为隔离矩阵后执行 `.tools/go/bin/go.exe test ./...`：通过。实际运行 `internal/app` 临时测试（约 11.4 秒），包含 53 组规则/真实本机 HTTP setup 与 login 测试以及存量弱密码登录、错误密码 401、二次设置 409、哈希不变和日志保密断言；其他包无测试源，仅提供编译证据。
- 五个修改 Python 验证脚本直接 `compile(...)`：通过，没有生成保留测试源。
- 独立生成 1000 个验证密码并检查 10–72 UTF-8 字节和四类 ASCII：全部通过，不输出密码值。
- `git diff --check`：通过，仅 Git 的 LF/CRLF 转换提示。

主代理持有并在最终验证后删除 `internal/app/password_policy_generated_test.go`；本复核没有新增单元测试源。未运行 Docker、创建提交或归档任务。
