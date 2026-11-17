import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 开发代理只在本机生效，生产环境由 Go 服务提供同源资源。
export default defineConfig({
  // Vue 单文件组件编译插件。
  plugins: [vue()],
  // 开发服务配置，仅供本机前后端联调。
  server: {
    // 前端本机开发端口。
    port: 5173,
    // 保持会话同源，将 API 和健康检查转发到 Go 服务。
    proxy: {
      // 业务接口的本机后端地址。
      '/api': 'http://127.0.0.1:8090',
      // 存储就绪检查的本机后端地址。
      '/healthz': 'http://127.0.0.1:8090',
    },
  },
  // 构建目标兼容现代 NAS 客户端浏览器。
  build: {
    // 使用 ES2022 目标，减少多余兼容代码。
    target: 'es2022',
  },
})
