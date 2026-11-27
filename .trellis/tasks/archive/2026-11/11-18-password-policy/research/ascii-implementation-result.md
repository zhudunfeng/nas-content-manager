# 禁止中文密码追加实现与验收

日期：2026-11-18。承接用户追加的“不可以包含中文”，本记录取代首轮允许附加中文或空格的新建密码规则；首轮执行事实保留在 `implementation-result.md`。

## 实现与契约

- 新管理员密码仅允许 U+0021–U+007E，即 ASCII 英文字母、数字和英文标点；长度 10–72 位，必须同时包含大写字母、小写字母、数字和英文特殊字符。该范围内字符数等于 UTF-8 字节数。
- `internal/app/auth.go` 在白名单分类的默认分支直接拒绝其他字符；`web/src/password.ts` 用非法字符检索 `/[^\x21-\x7e]/` 拒绝任何额外字符，避免 JavaScript `$` 对末尾换行的宽松行为。
- 中文含扩展汉字、全角字符、重音字符、空白、控制字符和表情，即使附加在已有四类的候选密码上也必须拒绝；不 trim、不替换、不截断。
- 前后端中文提示一致；`App.vue` 沿用仅首次创建账户时校验的调用，登录不套用新建规则，既有弱密码和中文密码继续按原哈希匹配。数据库、哈希算法与改密流程均未更改。
- README、API 与前后端契约同步；首轮五个验证脚本凭据生成方法已天然满足此次 ASCII 白名单，无需再次改写引擎凭据。

## 本轮实际执行

1. 前后端共用 `.runtime/password-policy/ascii-cases.json`，完整 71 例均通过。TypeScript 使用实际源文件经本机编译器转译后执行，Go 使用 `NAS_PASSWORD_CASES_FILE` 加载同一矩阵。覆盖 9/10/72/73 边界、四类缺失、全部 32 个英文标点、中文与扩展/兼容汉字、全角与中文标点、重音、空白/控制字符/表情，包含尾部 LF 和 CRLF。前后端提示逐字相同。
2. 临时 `TestGeneratedASCIIPolicy` / `TestGeneratedASCIILegacy` 完整执行通过。每个矩阵用例使用独立 SQLite 与真实回环 HTTP：非法 setup 为 400、提示一致且 users/sessions 均无新增；合法 setup 为 201、有 HttpOnly Cookie，bcrypt 匹配原始输入且原密码登录为 200。另验证非法 UTF-8 拒绝、旧弱密码及旧中文密码登录为 200、错误密码为 401、重复合法 setup 为 409、原哈希不变、日志不含密码。
3. 独立复核未发现实现缺陷；实际前端校验另通过 65,553 项断言，包括全部 BMP 码元及补充 Unicode。Go vet、gofmt、严格 vue-tsc、diff 空白检查均通过，见 `ascii-quality-review.md`。
4. 本轮实际 CLI 构建与 `npm --prefix web run build` 生产构建通过；浏览器连接该 CLI 与新 `web/dist`，未模拟认证 API。创建页的中文、全角、表情候选均在 POST 前被阻止；直接绕过前端提交含中文密码返回 400，账户仍未创建；合法 ASCII 候选创建 201、退出后原密码登录 200、最后退出 200。登录页无新建密码提示。
5. 创建页在 375/768/1024/1440 × light/dark 共 8 组尺寸主题中无密码提示横向溢出，`aria-describedby` 正确关联。人工查看了 `.runtime/password-policy/ascii-setup-375-dark.png`，提示完整换行、未遮挡表单。主动绕过前端的负例产生预期 400 控制台记录。退出后的首次自动化等待误用了按钮名称而超时，读取实际快照并使用页面的“进入内容空间”后，真实登录与退出均成功；不是产品登录故障。

## 清理与验证边界

- 测试使用新的本机回环端口及隔离数据根，启动前清除继承 `NAS_*`；没有接入真实媒体、远程服务器或 Docker。
- 已关闭验证浏览器；核对可执行文件后停止本任务隔离 PID，删除其数据库、WAL/SHM、锁文件及空目录。核验进程数 0、11774 监听数 0、实例目录不存在；脱敏日志保留于忽略目录 `.runtime/password-policy/ascii-server.log`。
- 已删除本轮临时 Go 单元测试源 `internal/app/password_ascii_generated_test.go`。删除后的 `go test ./...` 只作为编译检查，不代替上述 71 例及实际 HTTP 行为证据。
- 最终 `go test ./...` 所有包编译检查通过；`gofmt -l internal/app/auth.go` 无输出，`git diff --check` 与 Trellis `task.py validate` 均通过。任务转入 `review`，验收条目已勾选。
- 此轮仅修订密码字符规则；五媒体/5 GiB 稀疏 Range 的 52 项回归是首轮结果，未在此轮重新执行。Docker 部署由用户手动验证，没有运行 Docker、提交或归档。
