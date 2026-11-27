# 管理员密码规则实现与验证

日期：2026-11-18。

> 首轮历史记录：下述 53 例及含中文密码创建成功是用户追加“不可以包含中文”之前的实际执行结果，已被新建密码 ASCII 白名单取代，不能作为当前密码规则。追加实现和最终验收见 `ascii-implementation-result.md`，独立复核见 `ascii-quality-review.md`。

## 实现

- 后端 `validNewPassword(password string) bool` 在生成哈希与落库之前检查新建管理员密码；UTF-8 字节范围 10–72，必须含四类 ASCII 字符。用户名与密码错误分别反馈，日志只记录规则校验结果。
- 前端 `src/password.ts` 提供同名规则函数与中文提示，`App.vue` 仅在 setup 时使用，并保留确认密码。提示通过 `aria-describedby` 与输入关联。
- 密码原样传递，不 trim/截断；原登录匹配与密码哈希不改，不新增改密/重置接口，不迁移数据库。
- 五个验证脚本的应用密码复用 `generate_password() -> str`；引擎与来源凭据保持原行为。
- README、API 与两层 runtime/app 规范记录规则、错误、兼容性和回归要求。

## 已执行证据

1. `.runtime/password-policy/cases.json` 作为前后端同一输入矩阵，53 例全部通过。Go 使用 `NAS_PASSWORD_CASES_FILE` 加载矩阵，TypeScript 通过本机 TypeScript 编译器转译后在 Node 执行。覆盖 9/10/72/73 字节与中文多字节边界、缺失任一类别、全角和其他 Unicode 字符不替代 ASCII 类别、空格保留及 32 个标点。
2. 临时 `TestGeneratedNewPasswordPolicy` / `TestGeneratedLegacyPasswordLogin` 通过，运行真实本机 HTTP 与隔离 SQLite。断言 400 不创建用户、201 会话 HttpOnly、原始输入 bcrypt 匹配、旧弱密码 200、错误密码 401、重复 setup 409、原哈希不变和日志不含密码。独立复核再次通过，临时单元测试源最终删除。
3. 独立复核执行 Go vet、gofmt、全包测试、vue-tsc 与五个 Python 脚本语法检查；另生成 1000 个验证密码全部通过。参见 `quality-review.md`。
4. 实现阶段完成实际 CLI 构建和 Vue 生产构建。浏览器实际连接该 CLI 与 `web/dist`，不是认证 API mock：弱密码 setup POST 数为 0；合法含中文密码创建 201；logout 后原密码 login 200；较弱候选仍可提交到 login 并得到错误密码 401；登录页无 setup 提示。
5. 浏览器在 375/768/1024/1440 × light/dark 共 8 组中检查提示可访问关联和横向边界，均通过；已人工查看 375 深色与 1440 浅色截图。浏览器已关闭。观察到错误密码 401 和退出时在途 `/api/jobs` 请求 401/中文告警，未发现新的密码校验错误。
6. `python -X utf8 scripts/verify.py --root <本任务独立目录>/verify/media --base-url <本机独立端口> --username browser-verify --large-file --ffmpeg <本机已有工具>`：52 项全部通过，覆盖五类媒体、音乐、鉴权、持久个人状态、备份完整性和 5 GiB 稀疏文件大偏移。首次未加 `-X utf8` 时因本机 GBK 无法打印勾号而停止；明确 UTF-8 后重跑成功。没有为此修改无关产品代码。

## 清理与边界

- 测试只使用本机回环和新建目录，启动前清除继承的 `NAS_*` 环境参数；未连接真实媒体、远程服务器或 Docker。
- 仅停止本任务记录且可执行文件路径匹配的隔离 PID，删除临时样本、数据库、备份与凭据，保留忽略目录内脱敏日志和截图用于复查。
- 用户要求的临时单元测试源已删除。最终 `go test ./...` 无测试文件，仅作为编译检查，不代替前述行为证据。
- Docker 部署继续由用户验证；没有提交或归档，任务交付 review。
