import { defineConfig } from "vite";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  base: "/assets/dist/",
  plugins: [tailwindcss()],
  build: {
    emptyOutDir: true,
    outDir: "internal/server/ui/assets/dist",
    rollupOptions: {
      input: "frontend/src/app.js",
      output: {
        assetFileNames: (assetInfo) => {
          if (assetInfo.names?.some((name) => name.endsWith(".woff2"))) {
            return "fonts/[name][extname]";
          }
          if (assetInfo.names?.some((name) => name.endsWith(".css"))) {
            return "app[extname]";
          }
          return "assets/[name][extname]";
        },
        entryFileNames: "app.js",
      },
    },
  },
});
