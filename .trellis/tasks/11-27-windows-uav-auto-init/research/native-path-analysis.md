# Windows 原生依赖最终路径加载失败分析

## 1. 根因分类

- **D：测试覆盖缺口；E：隐含假设。** 自动还原后的严格库存、架构和来源校验通过，暂存路径真实 worker 探针也通过，但最初的 `generation-<32hex>` 比 `stage-<32hex>` 长5个字符。最终 managed worker 导入 `curl_cffi._wrapper` 时，Windows 原生加载器报“文件名或扩展名太长”。
- 静态 `<260` UTF-16文件长度预算不能证明第三方加载器可用。独立边界实验中，最长规范DLL路径248/249/250单元能导入，251–260单元不能导入；这只是本机固定物料的观察值，不能替代最终位置的真实检查。
- 同解释器、环境和cwd，仅改名即复现；因此本次复现排除了桌面路径丢失、Python环境污染和managed数据目录差异。

## 2. 先前检查为何没有覆盖

1. 初始真实探针在暂存目录执行，未覆盖改名后的部署位置。
2. 名义MAX_PATH检查只核对规范文件名，未为原生DLL加载器内部路径操作留出余量。
3. 自动激活失败的通用状态掩盖了实际worker的安全失败原因，延长了定位。

## 3. 已实施的预防措施

| 优先级 | 措施 | 来源 |
| --- | --- | --- |
| P0 | 不可变目录缩短为`gen-<32hex>`，保留完整随机身份；最终位置反而比stage短2个字符 | `internal/uavbootstrap/bootstrap.go` |
| P0 | 下载前及Resolve检查最终EXE/DLL/PYD路径，最多248个UTF-16单元；过长中文提示使用更短profile | 同上 |
| P0 | 顺序固定为完整还原→stage严格校验→改名→final严格校验→final真实worker探针→原子选择 | 同上；`internal/desktop/uav_initialization.go` |
| P0 | final探针失败或取消只清理本次未选代次，保留原current和既有可用代次 | 同上；独立故障注入 |
| P1 | 激活失败保留worker已脱敏的真实原因，中文状态和日志一致，禁止无界重试 | `internal/desktop/uav_initialization.go` |

## 4. 系统性扩展和验证证据

- 同类需要部署后导入的原生组件，不能仅用暂存目录执行成功作为最终可用证据。静态完整性与真实加载各自有独立作用。
- 构建输入和profile位置不相同；测试必须保留中文、空格、异cwd、只读程序目录及最终目录深度，不通过缩短测试路径规避产品缺陷。
- 桌面独立复现：`.trellis/tasks/11-27-windows-uav-auto-init/research/desktop-implementation.md`，stage DLL249成功、长5字符后254失败。
- 复核边界数据：`.runtime/verify/uav-review/native-loader-paths.json`。
- runtime修复回归：`.runtime/uav-bootstrap-final-relocation-tests.log`；原harness深度最终DLL248，20份真实缓存物料还原，最终位置真实`CheckUAV`、发布、Resolve和零新增下载复用通过。
- 复核故障注入：真实缓存还原旧版本后，仅在临时测试进程中把相同库存改走payload通道，执行完整Initialize；覆盖首次final探针失败、旧选择存在时失败，以及final探针返回成功但context取消。候选清理和旧选择保持通过。此检查是故障注入，不冒充真实联网验收。
- 配方库存为791项，另有精确清单和回执2份元数据，共793个实际文件；计数应区分。
- 原始两轮真实联网均在最终激活失败，失败日志保留于`.runtime/verify/windows-uav-auto-init/bootstrap-report.failure.log`，不能把这些历史运行记为成功。

## 5. 知识固化

- 已更新`.trellis/spec/backend/uav-windows-contract.md`的路径预算、目录命名、最终探针和原子发布契约。
- 已同步任务`info.md`、`docs/uav-windows.md`和长期集成脚本`scripts/verify_uav_bootstrap.py`。
- 仓库不存在`src/templates/markdown/spec/`镜像目录，无需创建新的模板副本。
- 不自动提交；任务按项目finish-work约定保留待审查工作树。

最终产品集成与发行边界见本任务`research/final-validation.md`及`research/quality-review.md`；本分析不代替这些验收记录。
