import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { once } from 'node:events';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import { createServer } from 'node:http';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';
import { build } from 'vite';
import { diagnosePlayback } from './media-seek-diagnostics.mjs';

// 真实 Chromium 媒体事件；不替代 Windows WebView2 和线上取流验收。
async function checkPlayback(connectMedia, edge, modes) {
  const assert = (condition, message) => { if (!condition) throw Error(message); };
  const delay = (ms) => new Promise((done) => setTimeout(done, ms));
  const until = async (check) => {
    for (let i = 0; i < 300; i++) { if (check()) return; await delay(20); }
    throw Error('等待媒体状态超时');
  };
  const passed = [];
  for (const mode of modes) {
    const video = document.createElement('video');
    const audio = document.createElement('audio');
    document.body.append(video, audio);
    const times = [];
    let error = '';
    const controls = { start: 1, end: 2, invalid: false, scrubbing: false, muted: false, volume: 1,
      onTime(value) { times.push(value); }, onPlaying() {}, onError(value) { error = value; } };
    const stop = connectMedia(video, audio, () => controls, () => {});
    try {
      video.src = `/${mode}/video.mp4`; audio.src = `/${mode}/audio.wav`;
      await until(() => video.readyState >= 3 && audio.readyState >= 3);
      if (edge) {
        const snapshot = () => JSON.stringify({ video: video.currentTime, audio: audio.currentTime, videoPaused: video.paused, audioPaused: audio.paused, error });
        if (edge === 'endpoint') {
          controls.start = 4; controls.end = video.duration;
          video.currentTime = 4;
          await until(() => !video.seeking);
          await video.play();
          await delay(1800);
          assert(!video.paused && !audio.paused, `${mode}: 选区终点等于媒体末尾时必须继续循环 ${snapshot()}`);
        } else if (edge === 'hold') {
          await video.play(); await until(() => video.currentTime > 1.2);
          controls.scrubbing = true;
          await delay(4500);
          controls.scrubbing = false; video.currentTime = 1.6;
          await delay(400);
          assert(!video.paused && !audio.paused, `${mode}: 长按跨循环边界后松开必须恢复播放 ${snapshot()}`);
        } else if (edge === 'buffer-same') {
          await video.play(); await until(() => video.currentTime > 1.2);
          controls.scrubbing = true;
          audio.dispatchEvent(new Event('waiting'));
          await delay(50);
          assert(video.paused && audio.paused, `${mode}: 缓冲应同时暂停 ${snapshot()}`);
          const target = video.currentTime;
          audio.dispatchEvent(new Event('canplay'));
          controls.scrubbing = false; video.currentTime = target;
          await delay(400);
          assert(!video.paused && !audio.paused, `${mode}: 缓冲后松开到相同时间必须恢复 ${snapshot()}`);
        } else if (edge === 'repeated') {
          await video.play(); await until(() => video.currentTime > 1.2); video.pause();
          for (const target of [1, 1.9, 1.1, 2, 1.4, 1.8]) {
            controls.scrubbing = true; const count = times.length;
            video.dispatchEvent(new Event('timeupdate')); await delay(120);
            assert(times.length === count, `${mode}: 连续拖动不得发布旧进度`);
            controls.scrubbing = false; video.currentTime = target;
            await until(() => !video.seeking && !audio.seeking);
            assert(Math.abs(video.currentTime - target) < 0.04 && Math.abs(audio.currentTime - target) < 0.04, `${mode}: 连续拖动终点不一致 ${snapshot()}`);
          }
        } else throw Error(`未知边界用例：${edge}`);
        passed.push(mode); continue;
      }
      await video.play();
      await until(() => times.some((value) => value > 0.2));
      video.pause(); await delay(50);
      assert(audio.paused, `${mode}: 暂停必须同步`);
      controls.scrubbing = true;
      const before = times.length;
      video.currentTime = 1.9;
      await until(() => !video.seeking);
      await delay(150);
      assert(times.length === before, `${mode}: 拖动时不能发布旧进度`);
      controls.scrubbing = false;
      video.currentTime = 1.65;
      await until(() => !video.seeking && !audio.seeking && Math.abs(audio.currentTime - 1.65) < 0.04);
      assert(Math.abs(video.currentTime - 1.65) < 0.04, `${mode}: 视频 seek 目标错误`);
      assert(Math.abs(times.at(-1) - 0.65) < 0.04, `${mode}: 进度回跳`);
      video.currentTime = 2;
      await until(() => !video.seeking && !audio.seeking);
      assert(video.currentTime === 2, `${mode}: 暂停时终点不能被循环拉回`);
      audio.dispatchEvent(new Event('canplay'));
      await delay(200);
      assert(video.paused && audio.paused && video.currentTime === 2, `${mode}: 主动暂停后 seek 和缓冲事件不得恢复播放`);
      times.length = 0;
      await video.play();
      await until(() => times.some((value) => value > 0.75));
      await until(() => times.at(-1) < 0.3);
      assert(!error, `${mode}: ${error}`);
      assert(times.every((value) => value >= 0 && value <= 1), `${mode}: 进度不能超出选区`);
      assert(video.muted && !audio.muted && audio.volume === 1, `${mode}: 双流声音来源错误`);
      passed.push(mode);
    } finally {
      stop();
      for (const media of [video, audio]) { media.removeAttribute('src'); media.load(); media.remove(); }
    }
    assert(video.paused && audio.paused, `${mode}: 卸载必须停止播放`);
  }
  return passed;
}

test(`真实媒体：${process.env.QA_MEDIA_PRECISION ? '定位精度回归（受控前提）' : process.env.QA_MEDIA_DIAG ? '定向诊断（含显式注入）' : process.env.QA_MEDIA_EDGE || '一秒选区、双流 seek 和循环'}，普通及延迟分块响应`, { timeout: 45000 }, async () => {
  const directory = await mkdtemp(join(tmpdir(), 'media-playback-'));
  let browser;
  let server;
  let timer;
  const transfers = new Set();
  try {
    for (const [name, args] of [
      ['video.mp4', ['-f', 'lavfi', '-i', 'testsrc=size=160x90:rate=20', '-an', '-c:v', 'libx264', '-preset', 'ultrafast', '-pix_fmt', 'yuv420p', '-movflags', '+faststart']],
      ['audio.wav', ['-f', 'lavfi', '-i', 'sine=frequency=440:sample_rate=44100']],
    ]) {
      const result = spawnSync(process.env.QA_FFMPEG || 'ffmpeg', ['-loglevel', 'error', ...args, '-t', '5', join(directory, name)], { encoding: 'utf8' });
      assert.equal(result.status, 0, result.error?.message || result.stderr);
    }
    const entry = '\0media-playback-test';
    const frontend = resolve(import.meta.dirname, '..');
    const bundle = await build({ configFile: false, root: frontend, logLevel: 'silent', plugins: [{
      name: 'media-playback-test', resolveId: (id) => id === entry ? entry : undefined,
      load: (id) => id === entry ? `import { connectMedia } from ${JSON.stringify(join(frontend, 'src/media.ts'))};
        (${(process.env.QA_MEDIA_DIAG || process.env.QA_MEDIA_PRECISION ? diagnosePlayback : checkPlayback).toString()})(connectMedia, ${JSON.stringify(process.env.QA_MEDIA_PRECISION ? 'precision' : process.env.QA_MEDIA_EDGE || '')}, ${JSON.stringify(process.env.QA_MEDIA_MODE ? [process.env.QA_MEDIA_MODE] : ['local', 'slow'])}).then(result => fetch('/result', { method: 'POST', body: JSON.stringify(Array.isArray(result) ? { passed: result } : result) })).catch(error => fetch('/result', { method: 'POST', body: JSON.stringify({ error: error.stack }) }));` : undefined,
    }], build: { write: false, rollupOptions: { input: entry } } });
    const script = bundle.output.find((item) => item.type === 'chunk').code;
    const files = { 'video.mp4': await readFile(join(directory, 'video.mp4')), 'audio.wav': await readFile(join(directory, 'audio.wav')) };
    let finish;
    const outcome = new Promise((done) => { finish = done; });
    server = createServer(async (request, response) => {
      if (request.url === '/result') {
        let body = ''; for await (const chunk of request) body += chunk;
        finish(JSON.parse(body)); response.end(); return;
      }
      const name = request.url.split('/').at(-1);
      const body = files[name];
      if (body) {
        const range = /bytes=(\d+)-(\d*)/.exec(request.headers.range || '');
        const start = range ? Number(range[1]) : 0;
        const end = range?.[2] ? Math.min(Number(range[2]), body.length - 1) : body.length - 1;
        response.writeHead(range ? 206 : 200, { 'Content-Type': name.endsWith('mp4') ? 'video/mp4' : 'audio/wav', 'Accept-Ranges': 'bytes',
          'Content-Length': end - start + 1, ...(range ? { 'Content-Range': `bytes ${start}-${end}/${body.length}` } : {}) });
        if (request.url.startsWith('/slow/')) {
          let offset = start;
          const transfer = setInterval(() => {
            response.write(body.subarray(offset, Math.min(end + 1, offset + 4096))); offset += 4096;
            if (offset > end) { clearInterval(transfer); transfers.delete(transfer); response.end(); }
          }, 30);
          transfers.add(transfer);
          response.on('close', () => { clearInterval(transfer); transfers.delete(transfer); });
        } else response.end(body.subarray(start, end + 1));
      } else if (request.url === '/test.js') {
        response.setHeader('Content-Type', 'text/javascript'); response.end(script);
      } else response.end('<!doctype html><script type="module" src="/test.js"></script>');
    });
    server.listen(0, '127.0.0.1'); await once(server, 'listening');
    browser = spawn(process.env.QA_CHROMIUM || '/usr/bin/chromium', ['--headless=new', '--no-sandbox', '--disable-gpu', '--autoplay-policy=no-user-gesture-required', `--user-data-dir=${join(directory, 'browser')}`, `http://127.0.0.1:${server.address().port}`], { stdio: 'ignore' });
    const result = await Promise.race([outcome,
      once(browser, 'exit').then(([code]) => { throw Error(`Chromium 提前退出：${code}`); }),
      new Promise((_, reject) => { timer = setTimeout(() => reject(Error('媒体断言超时')), 35000); }),
    ]);
    for (const event of result.diagnostics || []) console.log(JSON.stringify(event));
    assert.equal(result.error, undefined, result.error);
    assert.deepEqual(result.passed, process.env.QA_MEDIA_MODE ? [process.env.QA_MEDIA_MODE] : ['local', 'slow']);
  } finally {
    clearTimeout(timer); for (const transfer of transfers) clearInterval(transfer);
    if (browser && browser.exitCode === null && browser.signalCode === null) { browser.kill('SIGTERM'); await once(browser, 'exit'); }
    if (server) { server.closeAllConnections(); await new Promise((done) => server.close(done)); }
    await rm(directory, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  }
});
