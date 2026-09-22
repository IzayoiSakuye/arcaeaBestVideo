#!/usr/bin/env node
/**
 * spike-sample-video.ts —— P1 样片：用真实素材跑通整条成片管线
 * ============================================================================
 *
 * ⚠️ 这是 **spike（技术验证脚本）**，不是应用的实现。
 *    它存在的唯一意义是把管线跑通、把参数定死、把坑暴露出来 ——
 *    之后 Go 侧照着重写，然后**这个脚本就该删掉**。
 *    **不要往它上面加功能**（不做 UI、不做断点续跑、不做并发）。
 *
 * 跑通的链路（与 §7.4 的设计一致）：
 *
 *   ① 背景层   曲目背景 → 铺满 1920×1080 → 高斯模糊          （每首不同）
 *   ② 视频层   谱面录像片段 → 裁成 934×526                  （每首不同）
 *   ③ 装饰层   templates/bg16-9.png（RGBA，带洞口）               （固定）
 *   ④ 文字层   templates/text.html → Edge 无头截图 → RGBA PNG  （每首不同）
 *   ────────────────────────────────────────────────────────
 *   ffmpeg 四层叠加 → seg_NNN.mp4（参数锁死）
 *   全部完成后 → concat demuxer 无损拼接 → sample.mp4
 *
 * 用法
 *   node scripts/spike-sample-video.ts                       # 默认 3 首
 *   node scripts/spike-sample-video.ts --songs fractureray,aegleseeker
 *   node scripts/spike-sample-video.ts --limit 1 --no-download   # 复用已下的片段
 *   node scripts/spike-sample-video.ts --ytdlp <path>
 *
 * 依赖：ffmpeg、yt-dlp、Edge（都在 PATH 或常见位置，可用参数覆盖）。
 */

import fs from 'node:fs/promises';
import fsSync from 'node:fs';
import path from 'node:path';
import process from 'node:process';
import { execFile, execFileSync } from 'node:child_process';
import { promisify } from 'node:util';
import type {
  BackgroundManifest,
  CatalogChart,
  CatalogFile,
  CatalogSong,
  Difficulty,
  VideoSource,
} from './lib/types.ts';

const execFileAsync = promisify(execFile);

/**
 * ⚠️ **只是记录，不改变行为** —— 给 `--log-commands <file>` 用的。
 *
 * 为什么要有它：Go 侧要把这个 spike 的 ffmpeg / Edge 命令**逐字翻译**过去，
 * 而"防止翻译漂移"的唯一可靠手段是 golden test。golden 的期望值如果靠人肉抄，
 * 抄错就等于没测（真抄错过两次）。所以让 spike **自己把真实 argv 吐出来**。
 *
 * 输出格式：每行一个 JSON，`{"tool": "...", "argv": [...]}`。
 * 只在传了 --log-commands 时才写，平时完全无副作用。
 */
let commandLogFile: string | null = null;

function logCommand(tool: string, argv: readonly string[]): void {
  if (!commandLogFile) return;
  fsSync.appendFileSync(commandLogFile, JSON.stringify({ tool, argv }) + '\n');
}

/** 包一层：记录之后再真正执行 */
const run = async (
  file: string,
  argv: readonly string[],
  opts: { timeout?: number; maxBuffer?: number } = {}
): Promise<{ stdout: string; stderr: string }> => {
  logCommand(file, argv);
  return execFileAsync(file, argv as string[], opts) as Promise<{ stdout: string; stderr: string }>;
};

const ROOT = path.resolve(import.meta.dirname, '..');

// ---------------------------------------------------------------------------
// 这个 spike 自己的结构。数据契约（曲库/背景/来源）直接复用 lib/types.ts，
// 不在这里重抄一遍 —— 抄了就会和产物脱节。
// ---------------------------------------------------------------------------

interface WinRect { x: number; y: number; w: number; h: number }

/** null 一律表示「没指定，走自动探测」 */
interface Args {
  songs: string[] | null;
  limit: number;
  download: boolean;
  audio: boolean;
  quiet: boolean;
  ffmpeg: string | null;
  ytdlp: string | null;
  edge: string | null;
  /** 把每条外部命令的真实 argv 追加到这个文件（给 Go 侧的 golden test 当期望值） */
  logCommands: string | null;
}

interface Tools { ffmpeg: string; ffprobe: string; ytdlp: string; edge: string }

/** 挑中的一首曲 + 谱面 + 视频来源 + 背景 */
interface SongRow {
  song: CatalogSong;
  chart: CatalogChart;
  video: VideoSource;
  bg: string | null;
  /** 来源是不是 16:9 —— 决定用哪套模板 */
  wide: boolean;
}

/** 命中缓存的没有下载耗时，所以用可辨识联合而不是 `ms?: number` */
type ClipResult = { cached: true } | { cached: false; ms: number };

interface CompositeLayers {
  bg: string;
  clip: string;
  text: string;
  chrome: string;
  win: WinRect;
  /** 没有曲绘就是 null（那个口留空） */
  jacket: string | null;
}

// ---------------------------------------------------------------------------
// 出片参数 —— 全部锁死（§7.4：任一不一致，最后 concat 就会出问题）
// ---------------------------------------------------------------------------
const OUT = {
  W: 1920, H: 1080, FPS: 30,
  /**
   * 两套模板。洞口坐标是从装饰层的 alpha 通道**连通域分析**量出来的，不是估的。
   *
   * ⚠️ 为什么必须两套：catalog 的 871 个视频来源里 **37.5% 是 4:3**，
   *    按谱面算有 **321/645 根本没有 16:9 的来源**。
   *    4:3 塞进 16:9 窗口会裁掉 25% 高度，所以必须按来源画幅换窗口。
   */
  TEMPLATES: {
    169: { chrome: 'templates/bg16-9.png', win: { x: 20, y: 157, w: 934, h: 526 } },
    43: { chrome: 'templates/bg4-3.png', win: { x: 58, y: 95, w: 855, h: 641 } },
  },
  /**
   * 曲绘口。**两套模板的曲绘口完全一样**（4:3 那套是复制改的窗口），
   * 所以不放进 TEMPLATES 里，只留一份。
   * 坐标同样是从装饰层 alpha 通道量出来的。
   */
  JACKET: { x: 1289, y: 312, w: 160, h: 159 },
  /** 是否保留片段自带音频。关掉用 --no-audio（排查音画问题时有用） */
  audio: true,
  GOP: 60,
  CRF: 18,
  PRESET: 'medium',
  SEC_PER_SONG: 10,
  BLUR_SIGMA: 25,
};

const CFG: Args = {
  songs: null,
  limit: 3,
  download: true,
  // ⚠️ 这个默认值不能漏！漏了的话 args.audio 是 undefined，
  //    OUT.audio 被覆盖成 undefined → 走 -an 分支 → 成片静音。
  //    （第一次加音频时就踩了这个坑：参数、分支都写对了，就是默认值没给。）
  audio: true,
  quiet: false,
  ffmpeg: null,
  ytdlp: null,
  edge: null,
  logCommands: null,
};

function parseArgs(argv: string[]): Args {
  const a: Args = { ...CFG };
  for (let i = 2; i < argv.length; i++) {
    const v = argv[i];
    if (v === '--songs') a.songs = argv[++i].split(',').map((s) => s.trim()).filter(Boolean);
    else if (v === '--limit') a.limit = parseInt(argv[++i], 10);
    else if (v === '--no-download') a.download = false;
    else if (v === '--no-audio') a.audio = false;
    else if (v === '--quiet') a.quiet = true;
    else if (v === '--ffmpeg') a.ffmpeg = argv[++i];
    else if (v === '--ytdlp') a.ytdlp = argv[++i];
    else if (v === '--edge') a.edge = argv[++i];
    else if (v === '--log-commands') a.logCommands = argv[++i];
    else if (v === '-h' || v === '--help') {
      console.log(`用法: node scripts/spike-sample-video.ts [选项]

  --songs <列表>    指定 song_id，逗号分隔（默认自动挑 3 首）
  --limit <N>       自动挑时取前 N 首（默认 3）
  --no-download     跳过下载，复用 .cache/clips/ 里已有的片段
  --no-audio        不保留片段音频（排查音画问题时用）
  --ffmpeg <路径>   覆盖 ffmpeg
  --ytdlp <路径>    覆盖 yt-dlp
  --edge <路径>     覆盖 msedge
  --log-commands <文件>
                    把每条外部命令的真实 argv 追加进去（每行一个 JSON）。
                    **只记录、不改变行为** —— 给 Go 侧的 golden test 当期望值用。
  --quiet           少打印`);
      process.exit(0);
    }
  }
  return a;
}
const args = parseArgs(process.argv);
// --no-audio 覆盖配置（OUT 定义在 parseArgs 之前，所以在这里回填）
OUT.audio = args.audio;
// 开命令日志：先清空，避免上一轮的记录混进来
if (args.logCommands) {
  fsSync.writeFileSync(args.logCommands, '');
  commandLogFile = path.resolve(args.logCommands);
}
const log = (...a: unknown[]): void => { if (!args.quiet) console.log(...a); };
const toPosix = (p: string): string => p.split(path.sep).join('/');

// ---------------------------------------------------------------------------
// 工具定位
// ---------------------------------------------------------------------------

function firstExisting(cands: string[]): string | null {
  for (const c of cands) if (c && fsSync.existsSync(c)) return c;
  return null;
}
function which(cmd: string): string | null {
  try {
    const r = fsSync.existsSync(cmd) ? cmd : null;
    return r;
  } catch { return null; }
}

function resolveTools(): Tools {
  const ffmpeg = args.ffmpeg
    ?? firstExisting(['C:/ffmpeg/bin/ffmpeg.exe', 'E:/ffmpeg-8.0.1-full_build/bin/ffmpeg.exe'])
    ?? 'ffmpeg';
  // ffprobe 和 ffmpeg 同目录，用来判断片段有没有音轨
  const ffprobe = ffmpeg === 'ffmpeg' ? 'ffprobe' : path.join(path.dirname(ffmpeg), 'ffprobe.exe');
  const ytdlp = args.ytdlp
    ?? firstExisting([
      path.join(ROOT, 'tools/yt-dlp.exe'),
      path.join(process.env.TEMP ?? '', 'ytdlp-test/yt-dlp.exe'),
    ])
    ?? 'yt-dlp';
  const edge = args.edge
    ?? firstExisting([
      'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
      'C:/Program Files/Microsoft/Edge/Application/msedge.exe',
    ]);
  if (!edge) throw new Error('找不到 msedge.exe，用 --edge 指定');
  return { ffmpeg, ffprobe, ytdlp, edge };
}

// ---------------------------------------------------------------------------
// 假成绩 —— ⚠️ 仅用于样片。真实成绩来自成绩层（P2），这里只为把画面填满。
//            数值遵循 Arcaea 计分：PURE 记满、FAR 记一半、LOST 记 0，
//            所以 score = 1e7 × (pure + 0.5·far) / note，保证自洽不穿帮。
// ---------------------------------------------------------------------------

const COMMENTS = [
  '这首终于推上去了，感觉还能再压一点',
  '手感来了挡不住，第一次这么顺|下次试试关掉音押',
  '前段一直掉，后段反而稳了|还是得多练位移',
  '就差一点点理论值，可惜了',
  '打了一个月终于过线|谢谢陪我练的朋友',
];

function fakeScore(songId: string, note: number | null) {
  // 物量缺失时 catalog 给的是 null；JS 算术本来就把 null 当 0，这里只是显式写出来
  const n = note ?? 0;
  const h = [...songId].reduce((a, c) => (a * 31 + c.charCodeAt(0)) >>> 0, 7);
  const pick = (shift: number, mod: number): number => (h >>> shift) % mod;
  const lost = pick(3, 6) === 0 ? 1 + pick(5, 3) : 0;
  const far = pick(7, 9) + lost;
  const pure = Math.max(0, n - far - lost);
  const score = Math.round(1e7 * (pure + 0.5 * far) / n);
  return { pure, far, lost, score, comment: COMMENTS[pick(11, COMMENTS.length)] };
}

const DIFF_NAME: Record<Difficulty, string> = { PST: 'Past', PRS: 'Present', FTR: 'Future', BYD: 'Beyond', ETR: 'Eternal', INS: 'Insane' };
const PTT = '+12.34'; // 玩家总潜力值，整个视频固定

/**
 * 挑视频来源。
 *
 * ⚠️ 实测 871 个来源里 **37.5% 是 4:3**（1920×1440 / 2732×2048 / 3840×2880），
 *    按谱面算有 **321/645 根本没有 16:9 的来源**。
 *    成片的视频窗口是 16:9，4:3 塞进去会被裁掉 25% 高度。
 *    所以：**有 16:9 就优先用 16:9**；只有 4:3 时标记出来，
 *    让渲染层去选另一套模板（这个 spike 目前只会照 16:9 裁，属于已知不足）。
 */
function is169(s: VideoSource): boolean {
  // 上游可能只给了 width 没给 height：保留 NaN 比较（结果为 false），与原行为一致
  return s.video_width ? Math.abs(s.video_width / (s.video_height ?? NaN) - 16 / 9) < 0.02 : true;
}
function pickSource(chart: CatalogChart): VideoSource | null {
  const src = chart.video?.sources ?? [];
  if (!src.length) return null;
  const wide = src.filter(is169);
  return (wide.length ? wide : src)[0];
}

// ---------------------------------------------------------------------------
// 选曲
// ---------------------------------------------------------------------------

function pickSongs(catalog: CatalogFile, bgManifest: BackgroundManifest): SongRow[] {
  const rows: SongRow[] = [];
  for (const s of catalog.songs) {
    const b = bgManifest.songs[s.song_id];
    if (!b?.bg) continue;
    for (const ch of s.charts) {
      const v = pickSource(ch);
      if (!v || ch.constant == null || !ch.note) continue;
      if (!['FTR', 'ETR', 'BYD'].includes(ch.difficulty)) continue;
      if (ch.constant < 10.5 || v.duration_sec < 150) continue;
      rows.push({ song: s, chart: ch, video: v, bg: b.bg, wide: is169(v) });
    }
  }
  // 上面已过滤掉 constant == null 的行，断言安全
  rows.sort((a, b) => b.chart.constant! - a.chart.constant!);
  // 每首曲子只取一个谱面；默认拿定数最高的几首，且背景尽量不重复
  const seenSong = new Set<string>();
  // 这里 rows 的 bg 全部非空（上面 `if (!b?.bg) continue`），但 --songs 分支允许 null，
  // 所以集合按 string | null 收
  const seenBg = new Set<string | null>();
  const out: SongRow[] = [];
  for (const r of rows) {
    if (seenSong.has(r.song.song_id)) continue;
    if (seenBg.has(r.bg) && out.length < 8) continue;
    seenSong.add(r.song.song_id);
    seenBg.add(r.bg);
    out.push(r);
  }
  return out;
}

// ---------------------------------------------------------------------------

async function ensureDir(p: string): Promise<string> { await fs.mkdir(p, { recursive: true }); return p; }

/**
 * ① 下载片段
 *
 * ⚠️ 缓存在 `.cache/clips/<song_id>.mp4` 这个**固定位置**，不放在每次重建的 work 目录里 ——
 *    调设计的时候会重跑很多次，不能每次都重新下载（几百 MB 流量 + 对 B站 不礼貌）。
 */
async function downloadClip(tools: Tools, row: SongRow, workDir: string, delayMs: number): Promise<ClipResult> {
  const cacheDir = await ensureDir(path.join(ROOT, '.cache/clips'));
  const cached = path.join(cacheDir, `${row.song.song_id}.mp4`);
  const dest = path.join(workDir, 'clip.mp4');

  if (fsSync.existsSync(cached) && fsSync.statSync(cached).size > 0) {
    await fs.copyFile(cached, dest);
    return { cached: true };
  }
  if (!args.download) throw new Error(`--no-download 但缓存里没有 ${row.song.song_id}`);

  const start = 55;
  const dur = Math.min(OUT.SEC_PER_SONG + 4, Math.max(6, row.video.duration_sec - start - 5));
  const url = `https://www.bilibili.com/video/${row.video.bvid}?p=${row.video.p}`;
  log(`     ↳ 下载 ${row.video.bvid}?p=${row.video.p} 的 ${start}~${start + dur}s`);
  const t0 = Date.now();
  await run(tools.ytdlp, [
    '--no-warnings', '--newline',
    '-f', 'bv*[height<=1080][vcodec^=avc1]+ba[ext=m4a]',
    '--download-sections', `*${start}-${start + dur}`,
    '--ffmpeg-location', tools.ffmpeg,
    '--merge-output-format', 'mp4',
    '-o', cached, url,
  ], { timeout: 300_000, maxBuffer: 8 << 20 });
  await new Promise((s) => setTimeout(s, delayMs)); // ⚠️ 限速，别轰炸 B站
  await fs.copyFile(cached, dest);
  return { cached: false, ms: Date.now() - t0 };
}

/** ② 背景层：铺满 + 模糊 */
async function buildBackground(tools: Tools, bgName: string, dest: string): Promise<void> {
  const src = path.join(ROOT, 'data/backgrounds/960', `Background_${bgName}.jpg`);
  if (!fsSync.existsSync(src)) throw new Error(`背景图不存在：${src}`);
  await run(tools.ffmpeg, [
    '-y', '-v', 'error', '-i', src,
    '-vf', `scale=${OUT.W}:${OUT.H}:force_original_aspect_ratio=increase,crop=${OUT.W}:${OUT.H},gblur=sigma=${OUT.BLUR_SIGMA}`,
    dest,
  ], { timeout: 60_000 });
}

/** ③ 文字层：Edge 无头截图，真实值走查询参数注入 */
async function renderText(tools: Tools, values: Record<string, string | number>, dest: string): Promise<void> {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(values)) qs.set(k, String(v));
  const url = `file:///${toPosix(path.join(ROOT, 'templates/text.html'))}?${qs}`;
  if (fsSync.existsSync(dest)) await fs.unlink(dest);
  await run(tools.edge, [
    '--headless=new', '--disable-gpu', '--hide-scrollbars',
    '--force-device-scale-factor=1',
    `--window-size=${OUT.W},${OUT.H}`,
    '--default-background-color=00000000',   // ← 透明底，不能省
    '--virtual-time-budget=4000',
    `--user-data-dir=${path.join(ROOT, '.cache/spike/edge-profile')}`,
    `--screenshot=${dest}`,
    url,
  ], { timeout: 120_000, maxBuffer: 8 << 20 });
  if (!fsSync.existsSync(dest)) throw new Error('Edge 没有产出截图');
}

/** ④ 四层合成（+ 曲绘，共五层） */
async function composite(tools: Tools, layers: CompositeLayers, dest: string): Promise<void> {
  const { bg, clip, text, chrome, win, jacket } = layers;
  const { x, y, w, h } = win;
  const J = OUT.JACKET;

  // 图层顺序（下 → 上）：
  //   背景 → 视频 → 曲绘 → 装饰层 → 文字层
  // ⚠️ 曲绘必须在**装饰层下面** —— 那个方口是装饰层上挖的洞，
  //    曲绘放上面会盖住洞口周围的边框。
  const inputs = [
    '-i', clip,
    '-loop', '1', '-framerate', String(OUT.FPS), '-i', bg,
  ];
  const fc = [
    `[0:v]fps=${OUT.FPS},scale=${w}:${h}:force_original_aspect_ratio=increase,crop=${w}:${h},setsar=1[vid]`,
    `[1:v]format=rgba[bg]`,
    `[bg][vid]overlay=${x}:${y}:format=auto[o1]`,
  ];

  let next = 2;      // 下一个输入的下标
  let base = 'o1';

  if (jacket) {
    inputs.push('-loop', '1', '-framerate', String(OUT.FPS), '-i', jacket);
    fc.push(`[${next}:v]scale=${J.w}:${J.h}:force_original_aspect_ratio=increase,` +
      `crop=${J.w}:${J.h},setsar=1,format=rgba[jk]`);
    fc.push(`[${base}][jk]overlay=${J.x}:${J.y}:format=auto[o2]`);
    base = 'o2';
    next++;
  }

  inputs.push('-loop', '1', '-framerate', String(OUT.FPS), '-i', chrome);
  fc.push(`[${next}:v]format=rgba[ch]`);
  fc.push(`[${base}][ch]overlay=0:0:format=auto[o3]`);
  base = 'o3';
  next++;

  inputs.push('-loop', '1', '-framerate', String(OUT.FPS), '-i', text);
  fc.push(`[${next}:v]format=rgba[tx]`);
  fc.push(`[${base}][tx]overlay=0:0:format=auto[v]`);
  next++;

  // ── 音频 ──
  // 片段自带音频（yt-dlp 下的是 DASH 音视频合并，aac 48k 立体声）。
  // ⚠️ 统一编码参数是 concat -c copy 的前提，音视频都一样。
  //    个别来源可能没有音轨 —— 那就补一条等长静音，否则 concat 会因为
  //    各段流数不一致而失败。
  const mapAudio: string[] = [];
  if (OUT.audio && hasAudio(tools, clip)) {
    mapAudio.push('-map', '0:a:0');
  } else if (OUT.audio) {
    inputs.push('-f', 'lavfi', '-t', String(OUT.SEC_PER_SONG), '-i', 'anullsrc=r=48000:cl=stereo');
    mapAudio.push('-map', `${next}:a`);
  }
  const audioArgs = OUT.audio
    ? [...mapAudio, '-c:a', 'aac', '-b:a', '192k', '-ar', '48000', '-ac', '2']
    : ['-an'];

  await run(tools.ffmpeg, [
    '-y', '-v', 'error',
    ...inputs,
    '-filter_complex', fc.join(';'), '-map', '[v]',
    ...audioArgs,
    '-t', String(OUT.SEC_PER_SONG),
    '-c:v', 'libx264', '-preset', OUT.PRESET, '-crf', String(OUT.CRF),
    '-pix_fmt', 'yuv420p',
    '-g', String(OUT.GOP), '-keyint_min', String(OUT.GOP), '-sc_threshold', '0',
    dest,
  ], { timeout: 600_000, maxBuffer: 8 << 20 });
}

/**
 * 片段有没有音轨（没有就补静音，否则 concat 会因流数不一致失败）
 *
 * ⚠️ 这里踩过一个很隐蔽的坑：一开始写成了 `fsSync.execFileSync(...)`
 *    —— `execFileSync` 属于 `node:child_process`，**不在 `node:fs` 里**。
 *    于是它是 undefined，一调用就抛 TypeError，**被下面这个 catch 吞掉**，
 *    函数永远返回 false → 永远判定"没有音轨" → 永远走静音分支。
 *    表现是：参数、分支、编码全对，成片就是没声音。
 *
 *    教训：**catch-all 的 try/catch 会把 bug 藏起来**。所以探针出错时要吭声，
 *    不能默默返回一个"看起来合理"的值。
 */
function hasAudio(tools: Tools, file: string): boolean {
  try {
    const out = execFileSync(tools.ffprobe, [
      '-v', 'error', '-select_streams', 'a',
      '-show_entries', 'stream=codec_type', '-of', 'csv=p=0', file,
    ], { encoding: 'utf8', timeout: 30_000 });
    return out.trim().length > 0;
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    console.warn(`   ⚠️ 探测音轨失败（按"无音轨"处理）：${msg.slice(0, 90)}`);
    return false;
  }
}

/** ⑤ concat demuxer 无损拼接 */
async function concat(tools: Tools, segs: string[], outFile: string): Promise<void> {
  const listFile = path.join(path.dirname(segs[0]), 'concat.txt');
  // ⚠️ 列表里的路径用正斜杠 + 单引号包住；反斜杠在 concat demuxer 里是转义符
  const body = segs.map((s) => `file '${toPosix(s).replace(/'/g, "'\\''")}'`).join('\n') + '\n';
  await fs.writeFile(listFile, body);
  await run(tools.ffmpeg, [
    '-y', '-v', 'error', '-f', 'concat', '-safe', '0', '-i', listFile,
    '-c', 'copy', '-movflags', '+faststart', outFile,
  ], { timeout: 300_000, maxBuffer: 8 << 20 });
}

// ---------------------------------------------------------------------------

async function main() {
  const t0 = Date.now();
  const tools = resolveTools();
  log(`工具：ffmpeg=${tools.ffmpeg}`);
  log(`      yt-dlp=${tools.ytdlp}`);
  log(`      edge  =${tools.edge}`);

  const catalog = JSON.parse(await fs.readFile(path.join(ROOT, 'data/arcaea-catalog.json'), 'utf8')) as CatalogFile;
  const bgManifest = JSON.parse(await fs.readFile(path.join(ROOT, 'data/arcaea-backgrounds.json'), 'utf8')) as BackgroundManifest;

  let rows: SongRow[];
  if (args.songs) {
    rows = [];
    for (const id of args.songs) {
      const song = catalog.songs.find((s) => s.song_id === id);
      if (!song) { console.warn(`   ⚠️ 曲库里没有 ${id}`); continue; }
      const b = bgManifest.songs[id];
      const chart = song.charts.find((c) => pickSource(c) && c.constant != null && c.note)
        ?? song.charts.find((c) => pickSource(c));
      const v = chart ? pickSource(chart) : null;
      if (!chart || !v) { console.warn(`   ⚠️ ${id} 没有视频来源`); continue; }
      rows.push({ song, chart, video: v, bg: b?.bg ?? null, wide: is169(v) });
    }
  } else {
    rows = pickSongs(catalog, bgManifest).slice(0, args.limit);
  }
  if (!rows.length) throw new Error('没有可用的曲目');

  console.log('\n' + '='.repeat(70));
  console.log(`P1 样片：${rows.length} 首`);
  console.log('='.repeat(70));
  rows.forEach((r, i) => {
    const f = fakeScore(r.song.song_id, r.chart.note);
    const ar = r.video.video_width ? `${r.video.video_width}x${r.video.video_height}` : '未知';
    console.log(`  ${i + 1}. ${r.song.title_en}  [${r.chart.difficulty} ${r.chart.constant}]`);
    console.log(`     ${r.song.artist || '—'}  ·  bg=${r.bg}`);
    console.log(`     ${r.video.bvid}?p=${r.video.p}  画幅 ${ar}${r.wide ? '' : '  ⚠️ 非 16:9，会被裁'}`);
    console.log(`     分数 ${f.score.toLocaleString('en-US')}  PURE ${f.pure} FAR ${f.far} LOST ${f.lost}`);
  });

  const workRoot = await ensureDir(path.join(ROOT, '.cache/spike/work'));
  const distDir = await ensureDir(path.join(ROOT, 'dist/sample'));
  const segs: string[] = [];

  for (let i = 0; i < rows.length; i++) {
    const r = rows[i];
    const idx = String(i + 1).padStart(3, '0');
    const dir = await ensureDir(path.join(workRoot, `${idx}-${r.song.song_id}`));
    console.log(`\n[${i + 1}/${rows.length}] ${r.song.title_en}`);

    // ① 片段
    const res = await downloadClip(tools, r, dir, 600);
    if (!res.cached) log(`     ↳ 下载完成 ${(res.ms / 1000).toFixed(1)}s  ${(fsSync.statSync(path.join(dir, 'clip.mp4')).size / 1048576).toFixed(2)} MB`);
    else log(`     ↳ 片段命中缓存`);

    // ② 背景
    const bg = path.join(dir, 'bg.png');
    if (!r.bg) throw new Error(`${r.song.song_id} 没有背景`);
    await buildBackground(tools, r.bg, bg);
    log(`     ↳ 背景 bg=${r.bg} + blur(${OUT.BLUR_SIGMA})`);

    // ③ 文字层
    const f = fakeScore(r.song.song_id, r.chart.note);
    const text = path.join(dir, 'text.png');
    await renderText(tools, {
      ptt: PTT,
      score: f.score.toLocaleString('en-US'),
      pure: f.pure, far: f.far, lost: f.lost,
      title: r.song.title_en,
      author: r.song.artist || '',
      diff: `${DIFF_NAME[r.chart.difficulty] ?? r.chart.difficulty} ${r.chart.constant}`,
      diffClass: r.chart.difficulty.toLowerCase(),   // → .diff.d-ftr 等，见 text.html 的配色
      comment: f.comment,
    }, text);
    log(`     ↳ 文字层已渲染`);

    // ④ 合成（按来源画幅选模板 + 曲绘）
    const tpl = r.wide ? OUT.TEMPLATES[169] : OUT.TEMPLATES[43];
    const seg = path.join(dir, `seg_${idx}.mp4`);
    const jacketRel = r.song.jacket?.local_256;
    const jacketAbs = jacketRel ? path.join(ROOT, jacketRel) : null;
    if (!jacketAbs || !fsSync.existsSync(jacketAbs)) {
      log(`     ⚠️ ${r.song.song_id} 没有曲绘，那个口会是空的`);
    }
    await composite(tools, {
      bg, clip: path.join(dir, 'clip.mp4'), text,
      chrome: path.join(ROOT, tpl.chrome), win: tpl.win,
      jacket: jacketAbs && fsSync.existsSync(jacketAbs) ? jacketAbs : null,
    }, seg);
    const st = fsSync.statSync(seg);
    log(`     ↳ 合成 ${(st.size / 1048576).toFixed(2)} MB`);
    segs.push(seg);
  }

  // ⑤ 拼接
  const outFile = path.join(distDir, 'sample.mp4');
  console.log(`\n拼接 ${segs.length} 段…`);
  await concat(tools, segs, outFile);

  const st = fsSync.statSync(outFile);
  console.log('\n' + '='.repeat(70));
  console.log('✅ 成片');
  console.log('='.repeat(70));
  console.log(`  ${toPosix(outFile)}`);
  console.log(`  ${(st.size / 1048576).toFixed(2)} MB`);
  console.log(`\n总耗时 ${((Date.now() - t0) / 1000).toFixed(1)}s`);
}

main().catch((e: unknown) => {
  console.error('\n❌ 失败:', e instanceof Error ? e.message : e);
  // execFile 的错误对象挂着 stderr，类型里没有，所以这里结构化收窄一次
  const stderr = e instanceof Error ? (e as Error & { stderr?: unknown }).stderr : undefined;
  if (stderr) console.error(String(stderr).slice(-1500));
  process.exit(1);
});
