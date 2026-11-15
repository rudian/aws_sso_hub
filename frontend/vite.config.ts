/// <reference types="vitest/config" />
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import wails from "@wailsio/runtime/plugins/vite";
import { cspPlugin, keepGitkeepPlugin, mockStubPlugin } from "./scripts/vite-plugins.ts";

const mockStub = fileURLToPath(new URL("./src/api/mockStub.ts", import.meta.url));

// https://vitejs.dev/config/
export default defineConfig(({ mode }) => ({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [
    react(),
    wails("./bindings"),
    cspPlugin(),
    keepGitkeepPlugin(),
    // Only a production build drops the mock layer; `build:dev` (mode development) keeps it for local inspection.
    ...(mode === "production" ? [mockStubPlugin(mockStub)] : []),
  ],
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.ts", "src/**/*.test.tsx", "scripts/**/*.test.ts"],
  },
}));
