import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The backend to talk to during `vite dev`. In production the app is served by
// the Go node itself (same origin), so no proxy is involved.
const backend = process.env.VITE_BACKEND ?? "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  build: {
    // The Go server embeds this directory via //go:embed. Keep emptyOutDir
    // false so the committed placeholder.html survives a production build —
    // the built index.html takes precedence at serve time.
    outDir: "../internal/webui/dist",
    emptyOutDir: false,
  },
  server: {
    proxy: {
      "/v1": { target: backend, changeOrigin: true },
      "/raft": { target: backend, changeOrigin: true },
      "/cluster": { target: backend, changeOrigin: true },
      "/node": { target: backend, changeOrigin: true },
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/setupTests.ts"],
    css: false,
  },
});
