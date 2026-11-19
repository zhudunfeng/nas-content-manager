# 栖库 Windows 图标

`qiku.ico` 沿用 `web/public/favicon.svg` 的深色圆角底和薄荷色双书架，含 16、20、24、32、40、48、64、96、128、256 像素的透明 32 位图像。更新品牌时请同步导出这些尺寸，保留透明边角，并在小尺寸下检查辨识度。

桌面入口预置 `cmd/nas-desktop/resource_windows_amd64.syso`。普通 `go build ./cmd/nas-desktop` 和现有 Windows 发布流程自动链接，不需要额外资源编译器、外置图标文件或桌面框架。

更新 ICO 后，从仓库根执行：

```powershell
# 只在维护图标时再生；首次需要联网下载固定版本 rsrc。
python -B scripts/project_env.py -- python -B scripts/generate_windows_icon.py
# 再次生成并逐字节核对，保证 ICO 与提交的资源对象一致。
python -B scripts/project_env.py -- python -B scripts/generate_windows_icon.py --check
```

也可使用 `python -B scripts/project_env.py -- go generate ./cmd/nas-desktop`。维护工具固定为 `github.com/akavel/rsrc@v0.10.2`，其下载、构建缓存与临时输出遵循项目 `.runtime/` 环境；不加入应用 `go.mod`，不要求终端用户安装。

当前资源对象只包含图标，图标组 ID 固定为 1，供 `LoadImageW` 使用。不要直接向生成命令增加 manifest：rsrc 会因此改变自动分配的图标组 ID。以后扩展资源类型或 Windows 架构时，必须同时核对生成器、运行时加载 ID、对象目标架构和集成验证。
