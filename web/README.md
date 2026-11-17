# 栖库前端

Vue 3 + TypeScript + Vite 单页应用。开发使用 `npm ci`、`npm run dev`；检查使用 `npm run typecheck`；生产使用 `npm run build`。生产产物在 `dist/`，由 Go 同源提供，不需要单独部署 Node 服务。

## 代码结构

- `src/App.vue`：会话、导航、索引分页、搜索、收藏、标签和管理界面。
- `src/components/LibraryDialog.vue`：受控挂载目录选择与建库。
- `src/components/ReaderDialog.vue`：视频、图片、漫画、TXT、EPUB 消费流程。
- `src/components/DialogShell.vue`：原生 dialog、焦点约束、退出与恢复。
- `src/epub-safety.ts`：有界下载、压缩目录预检、章节清理和 CSP。
- `src/api.ts` / `src/types.ts`：同源接口、错误与共享模型。
- `src/style.css`：语义主题、媒体书架与响应式布局。

API 契约见 `../docs/api.md`，部署说明见仓库根 README。

## 构建配置说明

`vite.config.ts` 和 `tsconfig.json` 使用逐参数中文注释。`package.json` 为标准 JSON，参数含义如下：

| 参数 | 中文解释 |
| --- | --- |
| `name` | 仅用于本地依赖管理的前端包名 |
| `private` | 禁止误发布到 npm |
| `version` | 前端源码版本，服务版本以 `/api/status` 为准 |
| `type` | 使用标准 ECMAScript 模块 |
| `scripts.dev` | 在本机启动 Vite，代理后端端口 8090 |
| `scripts.typecheck` | 严格检查 TypeScript 和 Vue 模板，不输出文件 |
| `scripts.build` | 先通过类型检查，再生成静态发布资源 |
| `scripts.preview` | 本机预览静态构建；实际集成测试应使用 Go 服务 |
| `dependencies.vue` | 界面与响应式状态框架 |
| `dependencies.epubjs` | 按需加载的 EPUB 布局与定位组件 |
| `dependencies.dompurify` | 清理电子书章节中的活动内容 |
| `overrides.@xmldom/xmldom` | 固定 EPUB 传递依赖的安全补丁版本，更新需重新审计与验证 |
| `devDependencies.@types/node` | Vite 配置的 Node 类型声明 |
| `devDependencies.@vitejs/plugin-vue` | Vue 单文件组件构建插件 |
| `devDependencies.typescript` | 严格类型检查编译器 |
| `devDependencies.vite` | 本机开发与生产打包工具 |
| `devDependencies.vue-tsc` | Vue 模板类型检查工具 |

依赖锁文件 `package-lock.json` 由 npm 管理，不手工改动哈希或传递依赖树。

## 阅读与资源边界

- 视频 URL 直接交给原生 `<video>`，`preload="metadata"`；不通过 JS 下载或复制整个视频。浏览器使用鉴权 Range，关闭时暂停并清空源地址。
- 首页每页 48 条。漫画连续滚动每组最多挂载 12 页，图片懒加载；TXT 每组最多挂载 80 段。
- EPUB 下载上限 64 MiB；压缩目录最多 5000 项，单项解压标称大小不超过 32 MiB、总和不超过 256 MiB；不支持加密、分卷和 ZIP64。无 DRM 不代表所有排版都兼容。
- EPUB 使用禁用脚本的 iframe 沙箱，章节在渲染前经 DOMPurify 清理，并添加禁止脚本、外网和同源敏感资源访问的 CSP。书籍图片与字体通过本地 blob/data 资源展示。
- EPUB 位置保存为 CFI；视频为秒；漫画为零起始页码；TXT 为零起始段落。进度约每 8 秒合并提交，翻页、暂停与关闭时额外保存；页面退出使用 keepalive 尽力同步，断网时最多恢复到上次成功同步的位置。
- 阅读字号、行距、纸色及应用主题保存在当前浏览器本地。原始媒体、收藏和阅读进度由 NAS 保存。
