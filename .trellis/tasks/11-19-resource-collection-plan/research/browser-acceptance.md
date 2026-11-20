# 资源收集浏览器验收

日期：2026-11-20，UTC+08:00（Asia/Shanghai）。使用本机Edge、Python Playwright和真实Go服务；浏览器由项目环境包装器启动。仅使用新建隔离账号、样本媒体、受控RSS与qB。

## 操作与结果

`scripts/verify_collection_browser.py` 实际36项通过：

- 登录、执行端能力、成功空订阅、新订阅默认暂停、自动模式显式授权、保存及试运行不产生任务。
- 来源新增/修订、原生模态焦点、Escape关闭与焦点恢复；本地预览→提交→真实扫描→媒体详情及音乐播放。
- 已受理POST的响应被主动中断后，界面重试保留原UUID及完整计划；切换创建库时隐藏旧libraryId清零。
- RSS候选跳转原BT选文件详情；任务筛选空结果与来源列表503独立错误。
- 两项网页批次的受控响应：首项422保留失败、第二项成功；这只验证界面部分失败处理，不计为实际网页获取。
- 375、768、1024、1440px，两主题，概览及来源弹窗无横向溢出；手机底栏保持五项。

证据：`.runtime/verify/collection-browser-acceptance/browser-result.json`。`pageErrors=[]`；3条console错误分别对应主动中断、受控422、受控503，均为预期注入。

截图检查发现收集弹窗内容缺少与现有媒体库表单一致的内边距；已补桌面26px、手机18px。随后使用最后BT修订后的主程序和重新构建的前端，在 `.runtime/verify/collection-browser-reviewed/` 再次通过全部36项；`pageErrors=[]`，仍只有三项预期注入错误。已实际查看 `collection-1440-light.png` 与 `collection-375-dark.png`，字段及底部按钮留白、换行、边界正常。停止文件已触发隔离服务退出。

## 复验命令

```powershell
python -B scripts/project_env.py -- python -B scripts/verify_collection.py --binary .runtime/verify/nas.exe --worker-binary .runtime/verify/nas-collection-worker.exe --serve --root .runtime/verify/collection-browser-new --ytdlp D:/adunsoftware/python/python3.14.4/Scripts/yt-dlp.exe --ffmpeg D:/adunsoftware/ffmpeg/ffmpeg-master-latest-win64-gpl-shared/bin/ffmpeg.exe
python -B scripts/project_env.py -- python -B scripts/verify_collection_browser.py --root .runtime/verify/collection-browser-new --browser 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'
```

第一条保留隔离服务，另开受控命令执行第二条；在该隔离root创建`stop`文件后服务退出。不要复用已有root或真实媒体目录。浏览器结果不替代公网网页、Docker容器或NAS实机验收；真实网页工具链路见 `worker-tool-versions.md`。
