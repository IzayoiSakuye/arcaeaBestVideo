# Build Directory

## Windows 安装包

在仓库根目录执行。`data/` 是本机生成或从资源发行包安装的产物，不提交 Git；制作安装包前必须确认它已齐备。

```powershell
wails build
node scripts/build-ocr.ts
wails build -nsis
```

`makensis.exe` 需要位于 PATH。如果 NSIS 安装在 `E:\NSIS`，先在当前 PowerShell 会话执行 `$env:Path = 'E:\NSIS;' + $env:Path`。

安装器收录 `templates/`（含 OP1–7 WAV，不含 `.pkf`）、`data/arcaea-catalog.json`、`data/arcaea-backgrounds.json`、256px 曲绘、960px 背景和 `build/bin/ocr/`。FFmpeg、FFprobe 和 yt-dlp 不进安装包，用户下载后在设置中配置路径；YouTube 下载还依赖用户机器上的 Node。曲库更新及新版 OP 音频由应用下载到用户缓存目录。发布前在干净 Windows 环境安装并验证导入、资源更新、录像下载和导出。

## 成绩图文字 OCR（Windows）

构建机需要 Python 3.12 和 Node 26。执行 `wails build` 后运行
`node scripts/build-ocr.ts`：脚本安装隔离依赖，从
[PaddlePaddle 官方模型仓库](https://huggingface.co/PaddlePaddle/PP-OCRv6_small_rec_onnx)
下载 PP-OCRv6 small ONNX 模型，校验 SHA256，并生成 `build/bin/ocr/`。
分发时将 `build/bin/ocr/` 与应用 exe 放在同一目录；用户无需安装 Python 或下载模型。
模型仓库标注的许可证为 Apache-2.0。构建缓存位于 `.cache/`，不入库。

