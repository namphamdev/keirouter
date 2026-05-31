import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// During development the dashboard talks to the Go backend on :20180.
// In production the built assets are served by the backend itself.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5180,
    proxy: {
      "/api": "http://127.0.0.1:20180",
      "/v1": "http://127.0.0.1:20180",
      "/healthz": "http://127.0.0.1:20180",
    },
  },
  build: {
    outDir: "dist",
  },
});