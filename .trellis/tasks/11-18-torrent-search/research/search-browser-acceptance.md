# 搜种真实应用页面验收

日期：2026-11-18。使用最终 Windows Go 应用提供 `web/dist` 生产页面，受控 Torznab／qB 提供协议响应，没有拦截真实应用 API。浏览器服务由以下命令在独立根启动：

```powershell
$env:PYTHONUTF8='1'
python scripts/verify_torrent_search.py --binary .runtime/verify/search-nas.exe --serve
```

本次根：`.runtime/verify/search-20261118-081611-704f22/`。随机凭据只保存在忽略文件中，未打印；联调完成创建 `stop` 文件，由 finally 回收所有服务。

## 已执行结果

1. 从真实登录表单进入下载中心，加载独立搜种状态，不自动查询索引器。
2. 全来源搜“示例关键词”，成功来源返回3条，另一来源失败单独提示。超过4GiB资源显示5.0GiB。HTML样本标题保持文本，没有生成图片或执行事件。
3. 体积从大到小排序后磁力样本置首；成功空结果显示“没有找到匹配资源”；全部来源503经Go转换为502，页面显示明确错误。再次搜索恢复结果。
4. 选择“本地示例合集 · 种子”，搜索弹窗卸载后才打开原添加表单，始终只有一个 `dialog[open]`。安全标题／来源／体积显示，不能改成任意链接。
5. 明确选“小说”，真实 POST 添加成功，原协调器进入 `awaiting_selection`；清单为2文件、总36B，尚未发布。
6. 只勾第一章26B并确认；最终 API：`contentType=novel,state=downloading,selectionLocked=true,ready=false`。第一章已选、第二章未选，两框均锁定禁用。该浏览器样本不模拟完成；实际选中原字节入库由43项协议集成覆盖。
7. 关闭详情，重新打开搜种后 Esc 关闭，`dialog[open]=0`，焦点回到“搜种”，body内联overflow恢复空值。

## 响应式与视觉

实际长标题／部分失败结果在375／768／1024／1440px、light／dark共8种组合，文档与弹窗横向溢出均false，弹窗边界在视窗内，只有1个打开模态框。手机主导航按钮数为5。

截图已查看：

- `.runtime/search-real-mobile-light.png`：375px浅色搜索结果。
- `.runtime/search-real-desktop-dark.png`：1440px深色搜索结果。
- `.runtime/search-real-selected-dark.png`：原详情内锁定的小说选集。

原生对话框内部滚动，完整页面截图包含后方不可交互页面。无页面脚本异常或浏览器警告；唯一console error是故意“allfailed”产生的预期HTTP502。

自动化曾使用不完整文件名和错误锁定提示文字等待，另有一次严格label选择器超时；读取实际UI后修正定位，确认生产行为正常，无对应代码变更。凭据加载尝试不支持的VM模块接口后，改为浏览器临时文件输入读取隔离凭据；输入节点立即移除，未回显密钥。

## 清理与边界

页面与共享浏览器已关闭，根目录stop触发服务退出，非真实站点搜索、不涉及用户账号。更广的迟到响应、取消、过期、离线、未配置、20条分页和请求重试由 `search-frontend-validation.md` 的可控前端专项覆盖。真实 NAS／站点／Docker 仍由用户手动验证。
