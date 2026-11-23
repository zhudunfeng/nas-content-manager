# S01 实施契约

状态：2026-11-23 实施前冻结；实现后在 implementation.md 记录实际执行。只新增回归编排及档案，不修改业务 API、数据库或现有验证脚本。

## 命令与输入

```text
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite local --artifacts .runtime/bin --cases core --ffmpeg <绝对路径> --ffprobe <绝对路径> [--root .runtime/verify/<唯一编号>] [--port 0] [--timeout 300] [--startup-timeout 30] [--large-file]
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite target --target-file docs/nas-acceptance.template.json [--root .runtime/verify/<唯一编号>]
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --self-test
```

- local 可选择 core、downloads、search、engines、external、components、collection、release。默认仅 core；不带真实账号、Docker、真实网页/rclone选项。release 复用现有 --self-test，不冒充发布包验收。
- artifacts 是已构建目录；需要时分别查找 nas[.exe]、nas-collection-worker[.exe]。core 要求显式提供 FFmpeg/ffprobe 和已构建 web/dist/index.html，避免缺失视频/元数据却报告五媒体通过。
- timeout 为每个验证脚本的墙钟秒数（1–3600）；startup-timeout 为 core 就绪等待秒数（1–120）。端口为 0（内核选空闲回环端口）或 1024–65535；被占用即 blocked，不杀原监听器。
- root 必须是仓库 .runtime/verify 的新建后代，拒绝已有路径、..、符号链接、junction/reparse祖先；缺省编号包含 UTC 微秒与随机后缀。无效根在写入前拒绝，exit 2，不能为了记录失败向不安全位置写文件。
- target 版本1只校验模板并输出独立结果，不登录设备、不执行状态改变、不导入通过证据；M1/M2/M3只能为 blocked/not_run。真实执行与证据导入属于后续显式扩展，校验模板通过不等于目标验收通过。

## 运行与清理

每次新根分别保存 core/data、core/media、core/web、各领域夹具目录、logs、evidence、run-manifest.json。受控构建缓存仍复用项目 .runtime 下配置目录，原脚本自动清理的临时文件也遵从 project_env。

core 清除继承的 NAS_* 参数，仅设置本次数据/媒体/网页/回环端口和显式媒体工具；禁用下载、搜索、worker/helper。复制构建网页到本次web根并创建随机探针文件。启动后必须同时确认：自有进程仍存活、SQLite健康成功、本次随机静态探针精确匹配；随后才能调用 verify.py --base-url/--root。端口探测后关闭再启动存在极小竞争窗口，随机探针避免把第三方HTTP200当作本实例；竞争失败不接触第三方。

所有子命令参数数组执行，不用 shell。Windows 先 CREATE_SUSPENDED，归入 kill-on-close Job 后再恢复，归组失败不降级；清理封闭新进程准入、保存等待句柄、终止并等待自有整树。POSIX 使用新 session/process group，超时终止自有组。正常脚本退出仍回收其异常后代。不按名字或全局端口杀进程。清理失败必须使对应项 failed。

子进程输出持续排空、日志限长，持久内容脱敏：凭据字段、Authorization/Cookie、URL、已知秘密、仓库与外部绝对路径。原脚本私密fixture（例如credentials.json）仅作为本地受限材料，不列为可公开证据；生成日志在结束后再次按已知夹具秘密脱敏。

## 清单 v1 与四态

run-manifest 字段：formatVersion、runId、suite、startedAt、finishedAt、status、source（commit/dirty/修改计数）、platform（系统/版本/架构/Python/文件系统未知说明）、tools（版本/摘要/安全显示路径）、artifacts（SHA256/字节数/安全显示路径）、configuration（有界参数）、cases、limitations。

case 字段：id、selected、status、startedAt、finishedAt、reason、assertions、evidenceKinds、commands、evidence；命令保存 argv（脱敏数组）、exitCode（未启动为null）、timedOut、log、durationSeconds。证据一律为运行根相对路径。每个验证必须 exit0 且有实际断言计数/成功结果；缺少结果文件或成功标记不能只凭exit0通过。

| 条件 | 状态/退出码 |
| --- | --- |
| 已执行且断言与结果完整 | passed；整体0 |
| 断言失败、非零退出、超时、缺少预期结果、清理错误 | failed；整体1 |
| 缺产物/工具/网页，端口占用，安全进程所有权不可建立，目标设备未验 | blocked；整体2（failed优先1） |
| 未选择的组合，目标里程碑无实测 | not_run；不计成功 |
| 安全根非法或重复 | 写入前拒绝，exit2 |

单项失败后继续其他独立项；运行中持续原子保存清单，意外中止不能遗留passed。开始和结束时间使用UTC ISO8601。

## 检查断言

- Good/base：相同构建两次core运行，目录与端口不同，五媒体/鉴权/用户状态/SQLite快照有原脚本实际断言；5GiB稀疏样本仅为HTTP偏移。
- Bad：缺产物为blocked；已存在根、越界根和链接根拒绝；占端口的独立监听器存活；失败及超时孩子均failed；清理自有孙进程但独立旁观进程继续运行；秘密/私密URL/绝对路径被遮蔽。
- 持久 --self-test 使用真实短命子进程验证以上边界，不生成持久单元测试类；输出独立自检清单，不将自检解释成产品功能验收。
- target 模板保留真实OS/架构、CPU/内存/盘/文件系统、UID/GID/ACL、挂载、Docker/Compose/摘要、客户端/样本、起播/拖动/吞吐/并发/CPU/RSS/大库查询/重启恢复和M1–M3责任输入。未知值为null，禁止用fixture成绩填写真实设备指标。
