import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";

// base "./" is required so the built assets resolve from the Wails asset
// server regardless of the served path.
export default defineConfig({
  plugins: [react()],
  base: "./",
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    chunkSizeWarningLimit: 600,
    rollupOptions: {
      output: {
        manualChunks: {
          xterm: ["xterm", "xterm-addon-fit", "xterm-addon-web-links"],
          d3: ["d3"],
          radix: ["@radix-ui/react-dialog", "@radix-ui/react-dropdown-menu", "@radix-ui/react-tabs"],
        },
      },
    },
  },
  server: {
    port: 9245,
    strictPort: true,
  },
});