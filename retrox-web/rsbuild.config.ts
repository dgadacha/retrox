import { defineConfig } from "@rsbuild/core"
import { pluginReact } from "@rsbuild/plugin-react"
import path from "node:path"

// RETROX frontend build. Mirrors Notflix's rsbuild setup: assets land under
// `static/` so the Go server's `/static/*` route serves them, with index.html
// at the root for the embedded SPA. Dev server proxies /api to the Go backend.
export default defineConfig({
  plugins: [pluginReact()],
  source: {
    entry: { index: "./src/main.tsx" },
  },
  resolve: {
    alias: { "@": path.resolve(__dirname, "./src") },
  },
  server: {
    port: 50001,
    host: "0.0.0.0",
    proxy: {
      "/api": { target: "http://127.0.0.1:50000", changeOrigin: true },
    },
  },
  output: {
    cleanDistPath: true,
    distPath: { root: "out" },
    filename: {
      js: process.env.NODE_ENV === "production" ? "[name].[contenthash:8].js" : "[name].js",
      css: process.env.NODE_ENV === "production" ? "[name].[contenthash:8].css" : "[name].css",
    },
  },
  html: {
    template: "./index.html",
    title: "RETROX",
  },
})
