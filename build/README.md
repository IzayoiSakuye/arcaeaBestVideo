# Build Directory

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
