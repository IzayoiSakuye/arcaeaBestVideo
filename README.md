# Arcaea Best50 视频生成器

## 1. 项目介绍

这是一个 Windows 桌面应用，用于把 **Arcaea Best50 成绩**制作成视频。导入成绩后，可以校对每首歌的信息、补充或更换谱面录像、填写感想、调整出场顺序，最后导出 MP4。当前只支持 Arcaea Best50。

支持导入 Yurisaki 版式的 Best50 成绩图或成绩 JSON；也可以手动新增歌曲。图片识别结果可能有误，请在生成前逐首核对。

## 2. 运行环境与磁盘空间

- **系统**：Windows；应用使用系统 WebView2。
- **视频处理**：自行下载包含 `ffmpeg.exe` 和 `ffprobe.exe` 的 [FFmpeg Windows 构建包](https://ffmpeg.org/download.html)，解压后保留两个程序。
- **录像下载**：自行下载 [yt-dlp.exe](https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe)。只使用本地 MP4 时可以不下载 yt-dlp。
- **文字画面**：安装 [Microsoft Edge](https://www.microsoft.com/edge)，应用需要 `msedge.exe` 渲染视频中的文字。
- **YouTube 录像**：若使用 YouTube 链接，还需在电脑上安装 Node.js；B站链接和本地 MP4 不需要。

FFmpeg、FFprobe 和 yt-dlp **不包含在安装包中**。首次启动后，到「设置」页检查工具；自动检测不到时，点击「⋯」分别选择对应的 `.exe`，再点「验证可用」和「保存配置」。

请给应用所在磁盘和用户缓存所在磁盘预留空间。应用会保存曲库与曲绘、下载的**完整谱面录像**、渲染中间文件和最终 MP4；其中录像与中间文件占用最多，50 首歌可能需要数 GB，具体取决于来源视频大小和所选片段时长。建议先预留 **10 GB 以上**，空间不足时可在「设置」清理渲染缓存；「清理全部缓存」还会删除已下载录像，下次使用需要重新下载。

## 3. 使用方法

1. 从本项目的 [Releases](https://github.com/IzayoiSakuye/arcaeaBestVideo/releases) 下载 Windows 安装包并安装，准备好上面的工具。
2. 打开应用，在「设置」中检测或选择工具路径，点击「验证可用」和「保存配置」。如果提示资源缺失或有更新，点击「更新资源」获取曲库、曲绘和 OP 音频；曲库更新后按提示重启应用。
3. 在「导入图片 / JSON」中选择 Best50 成绩图或 JSON，点击「导入」。没有成绩图也可以手动新增歌曲。图片导入目前只支持 Yurisaki 版式。
4. 在「识别信息」中核对曲名、难度、分数、定数、单曲 PTT 和评级。应用会尝试获取录像；缺少或不合适时，可以填写 B站／YouTube 视频链接，或选择本地 MP4。需要时预览并调整每首歌的截取区间；未设置时默认取前 10 秒。
5. 在「填写感想」中添加可选的文字、图片和配乐；在「编排顺序」中调整歌曲与感想的顺序。
6. 到「生成视频」选择输出位置并开始生成。完成后可在「完成与导出」打开 MP4。

感想和单曲片段设置属于当前作品；**重新导入或退出应用后不会保留**，请在完成编辑后再导出。下载好的录像会留在缓存中，后续可复用。

## 4. 性能与常见影响因素

生成时会调用 FFmpeg 对每首歌分别编码，CPU 占用较高，笔记本可能发热或降频；建议使用电源并确保磁盘空间充足。首次生成还会建立文字渲染缓存。录像下载耗时取决于网络和视频来源，**不计入渲染时间**。调整片段时长、增加感想或使用较长的录像片段，也会增加生成时间与缓存占用。

开发时对默认 10 秒片段做过少量样本测试，50 首的渲染耗时外推约 **6–7 分钟**；这不是所有电脑和作品的保证值。生成失败时，先检查「设置」中的工具是否可用、录像是否就绪，以及磁盘剩余空间。

## 5. 视频来源与感谢

感谢提供 Arcaea 谱面录像的创作者，尤其是为曲目索引提供大量来源的「谱面保管室」，以及维护曲库和曲绘资料的社区贡献者。视频、曲绘、音乐等内容的权利归各自权利人所有。

应用的「识别信息」页会显示已知的录像发布者和来源。使用他人录像制作并发布视频时，请核对实际使用的来源，并在投稿简介中注明发布者与原视频链接；手动更换来源后也请相应更新署名。

## 6. 贡献与本地构建

欢迎通过 [Issue](https://github.com/IzayoiSakuye/arcaeaBestVideo/issues) 反馈问题或提出建议。报告问题时，请说明操作步骤、错误文字、应用版本，以及使用的是图片、JSON 还是手动录入；必要时可附脱敏截图。提交 Pull Request 前，建议先描述要解决的问题，并运行相关检查。

项目结构：`frontend/` 是 React 界面，`internal/app/` 是 Wails 接口，`internal/score/` 与 `internal/ocr/` 负责成绩校验和图片识别，`internal/resource/` 与 `internal/catalog/` 负责素材和曲库，`internal/render/` 与 `internal/job/` 负责渲染和任务编排，`templates/` 是视频画面模板，`sidecar/ocr/` 是本地 OCR 程序。曲库与曲绘的构建脚本位于独立的 [资源仓库](https://github.com/IzayoiSakuye/arcaea-b50-resources)。

开发环境需要 Windows、Go、Node.js、Wails v2；构建图片识别组件还需要 Python 3.12。仓库不提交 `data/` 资源包，可在应用内「更新资源」获取。构建命令在仓库根目录运行：

```powershell
go build ./...
go vet ./...
go test ./... -count=1
wails build
node scripts/build-ocr.ts
```

`wails build` 生成 `build/bin/rhythm-game-bvideo.exe`；图片导入还需要同级的 `build/bin/ocr/`。制作 Windows 安装包前准备好本地 `data/`，安装 NSIS 并运行 `wails build -nsis`。详细打包说明见 [build/README.md](build/README.md)。
