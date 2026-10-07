# rhythm-game-bvideo

为音游玩家自动生成成绩视频的桌面应用。用户提供 best50/40，工具自动配齐曲绘 / 游玩背景 / 谱面录像，
按模板合成一段可直接投稿到 B站的视频。

首款支持 **Arcaea**，后续扩 osu! → Phigros → maimai / CHUNITHM。
Windows 优先，**离线优先** —— 除了谱面录像，运行时不访问任何外部站点。

> 目标不是"能用"，而是**比现有工具快一个数量级**：50 段渲染 ≥60 分钟 → **≤10 分钟**。

---

## 当前进度

| 阶段 | 状态 |
|---|---|
| 方案设计 | ✅ |
| 数据层（曲库 / 索引 / 曲绘 / 背景 / 合并 / 打包） | ✅ 564 曲 · 1849 谱面 |
| `catalog` / `tools` / `render` / `resource` + `cmd/pipeline` | ✅ 端到端跑通 |
| 性能实测 | ✅ **50 段外推 ≈ 6.3 分钟** |
| 收尾 + 门禁评审 | ✅ |
| **门禁 G1**（5–10 个玩家） | ✅ **已过** · 反馈：多数说想要，头号短板是「成绩录入麻烦」 |
| P2 成绩层 | 🔄 进行中：骨架优先（导入 → 校对 → 出片），见 `docs/04-P2实现规格.md` |

## 目录

```
cmd/pipeline/     ★ 端到端 CLI（调试主力，不参与 wails build）
internal/         catalog 元数据查询 · resource 素材装配 · tools 外部程序定位 · render 渲染
scripts/          构建期数据管线（TypeScript，Node 直接跑，零依赖）
templates/        HTML/CSS 成片模板 + 装饰层（16:9 / 4:3）
samples/          sample-project.json —— CLI 的输入契约
frontend/         Wails 最小壳，目前只用于验证 bindings
data/  dist/      ⚠️ 构建产物，不入库
```

## 跑起来

```bash
# 渲染样例成片 → dist/sample/sample.mp4
go run ./cmd/pipeline

# 只渲一首，调设计时用这个
go run ./cmd/pipeline --limit=1 --force
```

需要 `ffmpeg` / `ffprobe`（Edge 用于 HTML 模板渲染，Windows 自带）。
找得到就自动用，也可以用 flag 显式指定。

## 校验

```bash
go build ./... && go vet ./... && go test ./... -count=1
go run ./cmd/pipeline --force          # 期望：3 段参数一致 + concat -c copy 成功
```

## 数据层

```bash
node scripts/build-song-db.ts        # ① 曲库
node scripts/build-arcaea-index.ts   # ② 视频索引（--self-test 跑解析器回归）
node scripts/build-jackets.ts        # ③ 曲绘 + 感知哈希
node scripts/build-backgrounds.ts    # ④ 游玩背景
node scripts/build-catalog.ts        # ⑤ 合并目录  ← 应用只读这个
```

详见 [`scripts/README.md`](scripts/README.md)。数据与代码分离发布：游戏加新曲只需重跑管线，用户不用更新应用。

## 版权

曲绘来自社区维基，谱面录像来自 B站「谱面保管室」（**明确可非商业使用**）。
因此**署名机制是必需功能，不是可选项**；本仓库不打包任何游戏原版素材与字体。
