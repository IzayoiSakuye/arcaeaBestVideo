# Build Directory

## Windows 安装包

在仓库根目录执行。`data/` 是本机生成或从资源发行包安装的产物，不提交 Git；制作安装包前必须确认它已齐备。

```powershell
wails build
node scripts/build-ocr.ts
New-Item -ItemType Directory -Force build/bin/tools | Out-Null
Copy-Item (Get-Command ffmpeg.exe).Source build/bin/tools/ffmpeg.exe
Copy-Item (Get-Command ffprobe.exe).Source build/bin/tools/ffprobe.exe
wails build -nsis
```

`makensis.exe` 需要位于 PATH。如果 NSIS 安装在 `E:\NSIS`，先在当前 PowerShell 会话执行 `$env:Path = 'E:\NSIS;' + $env:Path`。

安装器收录 `templates/`（不含 OP WAV）、`data/arcaea-catalog.json`、`data/arcaea-backgrounds.json`、256px 曲绘、960px 背景、`build/bin/ocr/` 和 `build/bin/tools/` 中的 FFmpeg/FFprobe。NSIS 缺文件时会直接构建失败。`yt-dlp` 按项目约定不进安装包，用户需要在设置中配置；YouTube 下载还依赖用户机器上的 Node。资源更新和 OP 音频由应用下载到用户缓存目录。发布前在干净 Windows 环境安装并验证导入、资源更新、录像下载和导出。

## 成绩图文字 OCR（Windows）

构建机需要 Python 3.12 和 Node 26。执行 `wails build` 后运行
`node scripts/build-ocr.ts`：脚本安装隔离依赖，从
[PaddlePaddle 官方模型仓库](https://huggingface.co/PaddlePaddle/PP-OCRv6_small_rec_onnx)
下载 PP-OCRv6 small ONNX 模型，校验 SHA256，并生成 `build/bin/ocr/`。
分发时将 `build/bin/ocr/` 与应用 exe 放在同一目录；用户无需安装 Python 或下载模型。
模型仓库标注的许可证为 Apache-2.0。构建缓存位于 `.cache/`，不入库。

The build directory is used to house all the build files and assets for your application. 

The structure is:

* bin - Output directory
* darwin - macOS specific files
* windows - Windows specific files

## Mac

The `darwin` directory holds files specific to Mac builds.
These may be customised and used as part of the build. To return these files to the default state, simply delete them
and
build with `wails build`.

The directory contains the following files:

- `Info.plist` - the main plist file used for Mac builds. It is used when building using `wails build`.
- `Info.dev.plist` - same as the main plist file but used when building using `wails dev`.

## Windows

The `windows` directory contains the manifest and rc files used when building with `wails build`.
These may be customised for your application. To return these files to the default state, simply delete them and
build with `wails build`.

- `icon.ico` - The icon used for the application. This is used when building using `wails build`. If you wish to
  use a different icon, simply replace this file with your own. If it is missing, a new `icon.ico` file
  will be created using the `appicon.png` file in the build directory.
- `installer/*` - The files used to create the Windows installer. These are used when building using `wails build`.
- `info.json` - Application details used for Windows builds. The data here will be used by the Windows installer,
  as well as the application itself (right click the exe -> properties -> details)
- `wails.exe.manifest` - The main application manifest file.
