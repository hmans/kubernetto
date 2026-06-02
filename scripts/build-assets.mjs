import { mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const rootDir = path.dirname(fileURLToPath(import.meta.url));
const fontPackage = "@fontsource-variable/inter";
const fontSlug = "inter";
const fontLabel = "Inter Variable";
const packageDir = path.dirname(require.resolve(`${fontPackage}/package.json`));
const fontsourceDir = path.join(rootDir, "..", "internal", "server", "ui", "assets", "vendor", "fontsource");
const targetDir = path.join(fontsourceDir, fontSlug);
const sourceCssPath = path.join(packageDir, "index.css");
const targetFilesDir = path.join(targetDir, "files");

const css = await readFile(sourceCssPath, "utf8");
const fontPaths = [...new Set([...css.matchAll(/url\((?:'|")?\.\/files\/([^)'"]+)(?:'|")?\)/g)].map((match) => match[1]))];

await mkdir(targetFilesDir, { recursive: true });

let changed = 0;

if (await writeIfChanged(path.join(targetDir, "index.css"), Buffer.from(css))) {
  changed += 1;
}

for (const fontPath of fontPaths) {
  if (await copyIfChanged(path.join(packageDir, "files", fontPath), path.join(targetFilesDir, fontPath))) {
    changed += 1;
  }
}

for (const fileName of await readdir(targetFilesDir)) {
  if (!fontPaths.includes(fileName)) {
    await rm(path.join(targetFilesDir, fileName));
    changed += 1;
  }
}

for (const entry of await readdir(fontsourceDir, { withFileTypes: true })) {
  if (entry.isDirectory() && entry.name !== fontSlug) {
    await rm(path.join(fontsourceDir, entry.name), { recursive: true, force: true });
    changed += 1;
  }
}

console.log(`${fontLabel} assets are current (${fontPaths.length} font files, ${changed} changed).`);

async function copyIfChanged(sourcePath, targetPath) {
  const source = await readFile(sourcePath);
  return writeIfChanged(targetPath, source);
}

async function writeIfChanged(targetPath, body) {
  try {
    const current = await readFile(targetPath);
    if (Buffer.compare(current, body) === 0) {
      return false;
    }
  } catch (error) {
    if (error.code !== "ENOENT") {
      throw error;
    }
  }

  await writeFile(targetPath, body);
  return true;
}
