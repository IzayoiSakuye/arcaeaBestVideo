# Arcaea B50 视频生成器

**摘星，双星甚至三星**！？想要发布视频分享自己的成绩、和大佬交流推分心得？

定数计算更新后b30→b50，做视频更加麻烦？

在这里，只需一份Arceaeb50分表，便可自动搜索视频并构建属于你的Arcaea best50视频

**（目前仅支持Windows10及以上的操作系统）**

![b50gen](https://img.wjwj.top/2026/11/14/121573b0c097870ebf409f3690ff6080.png)

## 快速开始 

- **获取安装器：**可以从右侧[Releases](https://github.com/IzayoiSakuye/arcaeaBestVideo/releases)页面下载最新installer，根据安装器提示完成安装（基础资源预计占用约 195MB，还需要保留下载谱面预览的空间）
- **准备运行环境：**
  - 下载[FFmpeg和FFprobe](https://ffmpeg.org/download.html)，用于视频渲染与校验
  - 下载[yt-dlp](https://github.com/yt-dlp/yt-dlp)，用于从流媒体下载视频
  - Edge浏览器，用于构建画面

- **设置运行环境：**启动生成器，首次进入会按照指引进入设置页面，在此处填写上方下载工具的可执行文件路径，参考路径：
  - FFmpeg：`ffmpeg-<version>-full_build\bin\ffmpeg.exe`
  - FFprobe：`ffmpeg-<version>-full_build\bin\ffprobe.exe`
  - yt-dlp：`yt-dlp.exe`
  - Edge：`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`

- **获取更新资源：**通过安装器下载的生成器会自带一份对应版本发布时的最新资源，若有资源更新，可以通过设置页面手动更新，或自动检测更新。
- **生成第一份视频：**从最左侧开始，根据引导导入图片，填写信息，等待生成即可~

## 生成器使用方法

1. 按照上方快速开始做好前置准备
2. 在「导入图片 / JSON」中选择你的b50成绩图或成绩JSON信息，点击「导入」。也可以选择手动新增歌曲。（目前只支持Yurisaki版式的b50成绩图）
3. 在「填写感想」中添加若干段单独的文字/图片片段，选择配乐。
4. 在「识别信息」中核对曲名、难度、分数、定数、单曲 PTT 和评级。应用会尝试自动获取录像；对于识别/获取失败或信息有误的曲目，会自动在顶部显示，在详情页可以看到具体的问题；你也可以自己填写要导入的视频链接或选取本地MP4文件；可以预览并调整每首歌的截取区间；默认取前 10 秒。
5. 在「编排顺序」中编排每首曲目以及感想片段的顺序，默认按照分表倒序
6. 在「生成视频」选择输出位置并开始生成。完成后可在「完成与导出」查看结果。

感想和单曲片段设置属于当前作品；**重新导入或退出应用后不会保留**，请在完成编辑后再导出。下载好的录像会留在缓存中，后续可复用。

## 效果预览

- 单曲视频生成效果：

  ![song eff](https://img.wjwj.top/2026/11/13/3cea11d2a01c69235c279b54da9b810b.png)

- 感想生成效果：

![thought eff](https://img.wjwj.top/2026/11/13/afab329572d1f5e63f9bb5533792cb86.png)

- 导入界面：

![import](https://img.wjwj.top/2026/11/13/79aba1786800c875c7a4cba198426046.png)

- 识别乐曲信息界面：

![info](https://img.wjwj.top/2026/11/13/e2d4101d52c0f3da5239719787502fbd.png)

## 贡献与本地构建

此项目是本人首个开源小玩具，做的比较粗糙请谅解

欢迎通过 [Issue](https://github.com/IzayoiSakuye/arcaeaBestVideo/issues) 反馈问题或提出建议~

报告问题时，请说明操作步骤、错误信息、应用版本，以及使用的录入方式；必要时可附截图。

也欢迎提交 Pull Request，贡献自己的想法喵



## 从源码构建

技术栈：

- Golang 1.26.4
- Wails v2.16.0
- Node v26.9.0
- ffmpeg 8.0.1
- PP-OCRv6 small ONNX

开发环境需要 Windows、Go、Node.js、Wails v2；构建图片识别组件还需要 Python 3.12。

构建命令在仓库根目录运行：

```powershell
go build ./...
go vet ./...
go test ./... -count=1
wails build
node scripts/build-ocr.ts
```

`wails build` 生成 `build/bin/arcaea-best-video.exe`；图片导入还需要同级的 `build/bin/ocr/`。制作 Windows 安装包前准备好本地 `data/`，安装 NSIS 并运行 `wails build -nsis`。详细打包说明见 [build/README.md](build/README.md)。
