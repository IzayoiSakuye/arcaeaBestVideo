// 将官方 PP-OCRv6 small ONNX 模型与本地识别程序放到 Wails 产物旁边。
import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { copyFileSync, cpSync, existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(fileURLToPath(new URL("..", import.meta.url)));
const cache = path.join(root, ".cache");
const venv = path.join(cache, "ocr-venv");
const python = path.join(venv, "Scripts", "python.exe");
const modelDir = path.join(cache, "ocr-onnx-model");
const buildDir = path.join(cache, "ocr-onnx-build");
const output = path.join(root, "build", "bin", "ocr");
const modelFiles = {
  "inference.onnx": "5435fd747c9e0efe15a96d0b378d5bd157e9492ed8fd80edf08f30d02fa24634",
  "inference.yml": "ab078671bb49f06228eadccd34f1bb501e157f7a047095ffb943ba81512c77d1",
};

function run(program: string, args: string[]) {
  const result = spawnSync(program, args, { cwd: root, stdio: "inherit" });
  if (result.status !== 0) throw new Error(`${program} 执行失败：${result.status ?? result.error}`);
}

function matches(file: string, expected: string) {
  return existsSync(file) && createHash("sha256").update(readFileSync(file)).digest("hex") === expected;
}

if (!existsSync(python)) run("python", ["-m", "venv", venv]);
run(python, ["-m", "pip", "install", "-r", path.join(root, "sidecar", "ocr", "requirements.txt")]);
mkdirSync(modelDir, { recursive: true });
for (const [name, digest] of Object.entries(modelFiles)) {
  const file = path.join(modelDir, name);
  if (!matches(file, digest)) {
    const url = `https://huggingface.co/PaddlePaddle/PP-OCRv6_small_rec_onnx/resolve/main/${name}`;
    const response = await fetch(url);
    if (!response.ok) throw new Error(`下载 ${name} 失败：HTTP ${response.status}`);
    writeFileSync(file, Buffer.from(await response.arrayBuffer()));
  }
  if (!matches(file, digest)) throw new Error(`${name} 的 SHA256 校验失败`);
}

run(python, [
  "-m", "PyInstaller", "--noconfirm", "--clean", "--onedir", "--name", "recognize",
  "--distpath", path.join(buildDir, "dist"),
  "--workpath", path.join(buildDir, "work"),
  "--specpath", buildDir,
  path.join(root, "sidecar", "ocr", "recognize.py"),
]);
cpSync(path.join(buildDir, "dist", "recognize"), output, { recursive: true, force: true });
mkdirSync(path.join(output, "model"), { recursive: true });
for (const [name, digest] of Object.entries(modelFiles)) {
  const file = path.join(output, "model", name);
  copyFileSync(path.join(modelDir, name), file);
  if (!matches(file, digest)) throw new Error(`发布目录中的 ${name} 校验失败`);
}
console.log(`OCR 组件已生成：${output}`);
