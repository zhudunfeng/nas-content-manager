# JMComic 自动初始化集成验收脚本

日期：2026-11-27（Asia/Shanghai）。本记录仅描述已完成的脚本实现与静态校验；本子任务未执行网络下载或声明真实运行时验收通过。

## 实际交付

- `scripts/verify_jmcomic_bootstrap.py`：真实 Windows 隔离验收入口，复用既有桌面 IPC、登录、只读 ACL 与独占映像访问辅助函数。
- 仅复制本轮传入的桌面 EXE 和 worker 到随机隔离程序目录，对程序目录实际施加写入拒绝 ACL；profile、媒体与 CWD 独立，包含中文/空格。
- 子进程 PATH 仅含 System32，污染 PYTHONHOME/PYTHONPATH，剔除继承的业务和站点配置；不请求漫画或视频站点。
- 状态判定兼容原 `collectionInitialization` 单对象与新增 `collectionInitializations` 数组；最终以 worker 固定版本能力和登录后的认证 API 为准。

## 脚本检查矩阵

1. 显式 false、disabled、external、无效显式 JMComic Python 均零安装。
2. 错误代理真实下载失败，基础媒体保持 ready、中文提示可重试、无 current/stage/gen；失败后不无限重试，主动重检触发新一轮核验。
3. 观察到真实下载字节后停用和退出，等待清理且无迟到激活/半成品；退出后立即独占访问全部私有 EXE/DLL/PYD。
4. 同一失败 profile 联网准备后，真实 JMComic 2.7.12 worker 能力、认证 API 与最终目录 `jmcomic_runtime.verify(..., probe=True)` 一致。
5. 以错误代理离线重启，已发布全部文件 SHA256/mtime/current 原字节不变；漫画目录不包含视频工具。
6. 可选 `--with-uav`：空 profile 同时显式启用 JMComic 与 UAV，验证独立完整运行时、两个真实能力和最终状态；日志证据要求初次媒体 ready 加一次自动激活 ready，离线重启仅增加一次 ready。

## 已执行验证

全部命令通过项目环境入口运行，返回码均为 0：

```text
python -B scripts/project_env.py -- python -B -c "import ast; ... ast.parse(Path('scripts/verify_jmcomic_bootstrap.py').read_text(encoding='utf-8'))"
python -B scripts/project_env.py -- python -B scripts/verify_jmcomic_bootstrap.py --help
python -B scripts/project_env.py -- python -B -c "... 初始化单对象/数组兼容、真实能力空值与固定版本判定断言 ..."
```

未生成临时单元测试类或测试源码；保留的是持久化集成脚本。

## 主线运行方式

使用主代理生成资源并构建的最终包 EXE/worker，推荐直接验收最终包，避免以较早开发构建替代最终交付：

```text
python -B scripts/project_env.py -- python -B scripts/verify_jmcomic_bootstrap.py --binary <最终包/栖库.exe> --worker <最终包/nas-collection-worker.exe> --report .runtime/verify/jmcomic-enable-runtime/bootstrap-report.json --with-uav
```

若是没有内嵌网页的开发构建，额外传入 `--web-dir web/dist`。`--timeout` 默认每轮 1800 秒。脚本只在随机本轮目录运行，成功报告不包含账号凭据、原始站点信息或私有路径。

## 边界与后续

- 真实联网/离线/取消/双插件运行结果待主线执行，本记录不是该结果。
- 本机管理员隔离验证不能代替干净 Windows 普通用户或真实站点验收。
- 许可/对应源码人工分发审查仍是独立工作。
- 来源：`scripts/verify_uav_bootstrap.py`、`scripts/verify_desktop.py`、`scripts/verify_desktop_collection.py`、任务 `prd.md`。
