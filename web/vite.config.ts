import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
// 默认代理到本机 5080；联调隔离实例时可用 MONITOR_GO_API_TARGET 指定其他地址。
const target = process.env.MONITOR_GO_API_TARGET || "http://127.0.0.1:5080";
export default defineConfig({
  plugins: [react()],
  base: "./",
  server: {
    proxy: { "/api": { target, changeOrigin: false } },
  },
});
