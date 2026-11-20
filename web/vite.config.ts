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
      '/api': {
        // 业务接口仍转发至本机 Go 服务。
        target: 'http://127.0.0.1:8090',
        // 保留浏览器 Host，使后端继续按原始 Origin 校验同源请求。
        changeOrigin: false,
      },
      // 存储就绪检查的本机后端地址。
      '/healthz': {
        // 健康检查仍转发至本机 Go 服务。
        target: 'http://127.0.0.1:8090',
        // 保留浏览器 Host，使健康检查同样遵循原始 Origin 校验。
        changeOrigin: false,
      },
    },
  },
  // 构建目标兼容现代 NAS 客户端浏览器。
  build: {
    // 使用 ES2022 目标，减少多余兼容代码。
    target: 'es2022',
  },
})
