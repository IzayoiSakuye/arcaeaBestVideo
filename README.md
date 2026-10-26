# Arcaea Best50 视频生成器

Windows 桌面应用。导入 Arcaea Best50 成绩，校对结果和录像素材后生成视频。

## 当前功能

- 导入一张 Yurisaki 版式的 Best50 图片（1920 × 1750），或导入成绩 JSON。
- 识别分数、定数、单曲 PTT、通关类型和判定数；通过曲绘匹配曲目，识别不确定时提示校对。
- 导入后逐首获取谱面录像，复用完整录像缓存；缺少来源时可粘贴 B站／YouTube 视频链接或选择本地 MP4。
- 在「识别信息」页预览录像，用同一条双端点滑条或分:秒输入设置单曲开始、结束时间，点击「应用片段」后生成视频使用该区间。未设置时取前 10 秒；修改区间只重渲该曲片段，不重新下载录像。
- 在桌面界面中修改成绩、检查工具状态并生成视频；素材未就绪时会提示对应曲目。
- 提供命令行渲染入口，用于处理已有的项目数据。

片段时间支持 `1:23`、`1:23.5` 等格式。开始晚于结束、时长为 0、超出录像时长时会分别提示。设置保存在当前项目中，重新导入成绩后需重新选择。填写感想与编排顺序仍待实现；自制样式放在后续版本。

## 从源码构建

需要 Windows、Go、Node.js、Wails v2、Python 3.12，以及 ffmpeg 和 ffprobe；自动获取录像还需 yt-dlp。Python 只用于构建 OCR 组件，运行应用时不需要单独安装。

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
