# Arcaea Best50 视频生成器

Windows 桌面应用。导入 Arcaea Best50 成绩，校对识别结果后，使用本地素材生成视频。

## 当前功能

- 导入一张 Yurisaki 版式的 Best50 图片（1920 × 1750），或导入成绩 JSON。
- 识别分数、定数、单曲 PTT、通关类型和判定数；通过曲绘匹配曲目，识别不确定时提示校对。
- 在桌面界面中修改成绩、检查工具状态并生成视频。生成时使用本地已有的谱面录像；缺少素材会报错。
- 提供命令行渲染入口，用于处理已有的项目数据。

## 从源码构建

需要 Windows、Go、Node.js、Wails v2、Python 3.12，以及 ffmpeg 和 ffprobe。Python 只用于构建 OCR 组件，运行应用时不需要单独安装。

本仓库不包含可直接运行的 `data/` 数据包。先按 [数据脚本说明](scripts/README.md)生成本地数据，再构建应用：

```powershell
wails build
node scripts/build-ocr.ts
```

OCR 构建步骤会生成 `build/bin/ocr/`；使用图片导入功能时，该目录须与 `build/bin/rhythm-game-bvideo.exe` 放在一起。更多构建说明见 [build/README.md](build/README.md)。

已有本地录像和项目数据时，也可运行命令行入口：

```powershell
go run ./cmd/pipeline
```

## 检查

```powershell
go build ./...
go vet ./...
go test ./... -count=1
```

游戏图片、音频和谱面录像的版权属于各自权利人；曲库生成产物和录像缓存不入库。
