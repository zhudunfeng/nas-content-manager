# JMComic 高级能力浏览器验收

日期：2026-11-25，Windows、本机 Edge 无头浏览器。所有启动均通过项目 `project_env`，浏览器继承仓库内缓存与临时目录。仅连接新建的隔离回环应用，未连接用户实例。

## 最终结果

- 高级脚本 `scripts/verify_jmcomic_advanced_browser.py`：**50 项通过，退出 0**。
- 最终证据：`.runtime/verify/jmadvanced-final-browser-20261125-1/advanced-browser-153252/result.json`。
- 原浏览器脚本 `scripts/verify_jmcomic_browser.py`：**38 项通过，退出 0**，使用另一新根，避免已入库样例影响旧流程。
- 原流程证据：`.runtime/verify/jmadvanced-final-oldbrowser-20261125-1/browser-153252/result.json`。
- 两套均使用统一 `.runtime/bin/jmcomic-advanced-final/` 的主服务与 worker，及独立检查后重新构建的 `web/dist`。没有未处理页面异常。

## 实际覆盖

- 排序与时间参数实际发送；筛选变更清理旧结果；慢搜索的旧响应不覆盖新条件。
- 375、768、1024、1440 四种宽度及明暗主题下检查章节和批量模式，横向无溢出，Escape 后焦点归还入口，连续 Tab 焦点限制于弹窗。
- 每种尺寸保存全页截图及章节元素截图；额外用受控展示标题验证手机长章节换行。该标题拦截只改变展示文字，不冒充上游真实标题。
- 102 页全本预览超预算时保留目录并禁用提交；重新选择两页小章后，实际 worker 获取、发布、扫描，原漫画阅读器读取两张真实样例图片。
- 八页样例的实际 worker 获取期间观察非零且小于总数的原生进度条，而非人工填充界面进度。
- 超过十本、重复编号在发送前拒绝；逐本预览失败、全本超预算不阻断其余合法条目。
- 批量第一本实际受理；第二本以浏览器路由中断制造未知响应。仅调整浏览器时钟使预览过期后，重试仍保留第二本完整原请求及 UUID，已成功第一本不重发，来源／目标／模式锁定。
- 网络未知结果显示中文连接／重试提示，保留服务端受控错误。

## 视觉回读

主线实际查看最终证据中的以下图片：

- `advanced-375-dark-chapter-fields.png`：章节复选框、中文标题、编号与页数可读。
- `advanced-long-chapters.png`：长标题在窄屏换行，目录内部滚动；未横向覆盖控件。
- `advanced-batch-partial.png`：未知响应和超预算逐项显示，重试数量明确，禁用的目标字段可辨识。
- `advanced-page-progress.png`：实际获取中显示 `2 / 8 页`、已校验字节和复用页数，并说明后续校验发布流程。

首轮 49 项证据 `.runtime/verify/jmadvanced-browser-20261125-1/advanced-browser-152235/` 发生在最终长标题检查及中文错误补丁之前，仅保留历史，不与最终 50 项相加。

## 清理与边界

两个自有 serve 根均创建 `stop`，等待工具进程退出 0 后完成；未停止用户进程。最终进程和监听复核由 `final-validation.md` 所引主线审计记录。

这是实际应用、worker、浏览器和协议 fixture 的组合验证；部分错误与展示边界使用明确的浏览器响应控制。没有执行真实 Python 桥接或真实站点传输；真实固定依赖下载器的离线证据单独记在后端报告，不与浏览器断言混算。Docker、Linux 实机、物理 NAS、账号与公网可用性未验收。
