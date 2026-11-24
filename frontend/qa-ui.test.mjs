import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { createServer } from 'node:http';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import test from 'node:test';

// 使用真实构建和生成绑定，桥接响应受控；不替代 Wails/WebView2 实机验收。
function bootstrap() {
  localStorage.setItem('environment-ready', '1');
  const listeners = new Map();
  const clone = (value) => structuredClone(value);
  const calls = [];
  const songs = [1, 2].map((index) => ({ index, song_id: `qa${index}`, difficulty: { name: 'FTR', constant: 10 },
    score: { achievement: 99, rank: 'EX', pure: 100, far: 0, lost: 0 }, user: { comment: '' },
    video: { url: `https://www.bilibili.com/video/BV1op4y1W7M2?p=${index}`, owner_name: 'QA 发布者' },
    clip: { start_ms: 0, duration_ms: 1000, confirmed_key: `saved${index}` } }));
  const project = { schema_version: 1, game: 'arcaea', player: { name: 'QA' }, source: { type: 'manual' },
    defaults: { clips: { start_ms: 0, duration_ms: 1000 } }, songs, thoughts: [], play_order: songs.map((song) => ({ kind: 'song', song_index: song.index })) };
  const states = songs.map((song) => ({ index: song.index, song_id: song.song_id, difficulty: 'FTR', url: song.video.url,
    request_id: `${100 + song.index}`, status: song.index === 1 ? 'failed' : 'ready', error: song.index === 1 ? 'QA 缓存丢失' : '',
    preview_ready: true, preview_url: `/preview/${song.index}.mp4`, preview_audio_url: `/preview/${song.index}.m4a`,
    duration_ms: 3000, owner_name: 'QA 发布者', confirmed: true, material_ready: song.index === 2, progress: -1 }));
  const qa = window.qa = { calls, project, states, previewCount: 1, emit(name, value) { for (const cb of listeners.get(name) || []) cb(clone(value)); } };
  const functions = {
    Status: () => ({ tools: [], song_count: 2 }), Version: () => 'qa',
    CheckResourceUpdate: () => ({ update_available: false }), CheckOPAudioUpdate: () => ({ files: [] }), ThoughtAudioOptions: () => [],
    Import: () => ({ Project: clone(project), Notes: [] }),
    Review: (value) => ({ project: clone(value), renderable: true, problems: [], rows: value.songs.map((song) => ({ song_id: song.song_id, title: song.song_id, difficulty: 'FTR', difficulties: ['FTR'], problems: [] })) }),
    VideoInfos: () => {
      const snapshot = clone(states);
      return location.search.includes('race') ? new Promise((resolve) => { qa.resolveQuery = () => resolve(snapshot); }) : snapshot;
    }, Jacket: () => '', PickOutput: () => '/qa/result.mp4',
    RetryVideoPreparation: (id, difficulty, request) => {
      const state = states.find((state) => state.song_id === id);
      if (state.request_id !== request) throw Error('素材任务已改变');
      Object.assign(state, { request_id: `${Number(request) + 10}`, status: 'queued', error: '', progress: -1 });
      return clone(state);
    },
    CancelVideoPreparation: (id) => { const state = states.find((state) => state.song_id === id); state.status = 'cancelled'; qa.emit('video:event', state); },
    ConfirmVideoRange: (song, clip) => {
      const saved = { ...clone(song), clip: { ...clip, confirmed_key: 'new-saved' } };
      const state = states.find((item) => item.song_id === song.song_id);
      Object.assign(state, { request_id: `${Number(state.request_id) + 10}`, confirmed: true, material_ready: true, status: 'ready' });
      return { song: saved, state: clone(state) };
    },
    ReleasePreview: () => { qa.previewCount = 0; },
    CacheInfo: () => ({ preview_count: qa.previewCount, video_bytes: 20, render_bytes: 10, total_bytes: 30, rendering: false, downloading: false, in_use: false }),
    ClearCache: () => { if (qa.previewCount) throw Error('缓存正在预览'); },
    StartRender: () => ({ job_id: 'qa-job', output: '/qa/result.mp4' }),
  };
  window.go = { app: { App: new Proxy({}, { get(_, name) { return async (...args) => {
    calls.push({ name, args: clone(args) });
    if (!(name in functions)) throw Error(`未配置的桥接：${name}`);
    return functions[name](...args);
  }; } }) } };
  window.runtime = { EventsOnMultiple(name, callback) { const set = listeners.get(name) || new Set(); set.add(callback); listeners.set(name, set); return () => set.delete(callback); } };
}

test(process.env.QA_QUERY_RACE ? '查询返回前到达的完成事件必须解除素材门禁' : '真实组件：重试、旧事件、进度、确认、切歌、清理和生成门禁', { timeout: 60000 }, async () => {
  const directory = await mkdtemp(join(tmpdir(), 'arcb9-ui-'));
  const frontend = resolve(import.meta.dirname);
  const server = createServer(async (request, response) => {
    try {
      const path = new URL(request.url, 'http://localhost').pathname;
      const file = join(frontend, 'dist', path === '/' ? 'index.html' : path);
      let body = await readFile(file);
      if (path === '/') body = Buffer.from(body.toString().replace('<head>', `<head><script>(${bootstrap.toString()})();</script>`));
      response.setHeader('Content-Type', path.endsWith('.js') ? 'text/javascript' : path.endsWith('.css') ? 'text/css' : 'text/html');
      response.end(body);
    } catch { response.writeHead(404); response.end(); }
  });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  const chromium = spawn(process.env.QA_CHROMIUM || '/usr/bin/chromium', ['--headless=new', '--no-sandbox', '--disable-gpu', '--remote-debugging-port=0', `--user-data-dir=${directory}`, 'about:blank'], { stdio: ['ignore', 'ignore', 'pipe'] });
  let socket;
  let browserLog = '';
  chromium.stderr.on('data', (data) => { browserLog += data; });
  const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  try {
    let port;
    for (let i = 0; i < 100; i++) {
      try { port = (await readFile(join(directory, 'DevToolsActivePort'), 'utf8')).split('\n')[0]; break; } catch { await delay(50); }
    }
    assert.ok(port, browserLog);
    const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
    socket = new WebSocket(targets.find((target) => target.type === 'page').webSocketDebuggerUrl);
    await once(socket, 'open');
    let nextID = 0;
    const pending = new Map();
    socket.addEventListener('message', (event) => { const result = JSON.parse(event.data); if (result.id) { pending.get(result.id)?.(result); pending.delete(result.id); } });
    const send = (method, params = {}) => new Promise((resolve) => { const id = ++nextID; pending.set(id, resolve); socket.send(JSON.stringify({ id, method, params })); });
    const evaluate = async (expression) => {
      const reply = await send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true });
      assert.ok(!reply.result?.exceptionDetails, JSON.stringify(reply.result?.exceptionDetails));
      return reply.result.result.value;
    };
    const until = async (expression) => {
      for (let i = 0; i < 100; i++) { if (await evaluate(expression)) return; await delay(30); }
      assert.fail(`等待失败：${expression}\n${await evaluate('document.body.innerText')}`);
    };
    const click = async (text) => {
      assert.equal(await evaluate(`(() => { const button = [...document.querySelectorAll('button')].find((b) => (b.querySelector('.nav-label')?.textContent || b.textContent).trim() === ${JSON.stringify(text)}); if (!button || button.disabled) return false; button.click(); return true; })()`), true, `按钮不可达：${text}`);
      await delay(50);
    };
    await send('Page.navigate', { url: `http://127.0.0.1:${server.address().port}/${process.env.QA_QUERY_RACE ? '?race=1' : ''}` });
    await until(`!!document.querySelector('.nav-item')`);
    await click('JSON 导入');
    await evaluate(`(() => { const input = document.querySelector('textarea'); Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(input, '{}'); input.dispatchEvent(new Event('input', { bubbles: true })); })()`);
    await click('导入');
    await until(`window.qa.calls.some((call) => call.name === 'VideoInfos')`);
    if (process.env.QA_QUERY_RACE) {
      await until(`typeof window.qa.resolveQuery === 'function'`);
      await evaluate(`Object.assign(window.qa.states[0], { status: 'ready', material_ready: true, progress: 1, error: '' }); window.qa.emit('video:event', window.qa.states[0]); window.qa.resolveQuery()`);
      await click('生成视频');
      await until(`!!document.querySelector('.material-row')`);
      await click('选择路径');
      console.log(await evaluate(`document.querySelector('.material-summary').innerText`));
      if (process.env.QA_SCREENSHOT) {
        const screenshot = await send('Page.captureScreenshot');
        await writeFile(process.env.QA_SCREENSHOT, Buffer.from(screenshot.result.data, 'base64'));
      }
      assert.equal(await evaluate(`[...document.querySelectorAll('button')].find((b) => b.textContent.trim() === '开始生成').disabled`), false, '同 request_id 的完成事件被旧查询的 material_ready=false 覆盖');
      return;
    }
    await click('生成视频');
    await until(`!!document.querySelector('.material-row')`);
    await click('选择路径');
    assert.equal(await evaluate(`[...document.querySelectorAll('button')].find((b) => b.textContent.trim() === '开始生成').disabled`), true);
    await click('重试');
    await until(`!!document.querySelector('progress')`);
    assert.equal(await evaluate(`document.querySelector('progress').hasAttribute('value')`), false);
    await evaluate(`window.qa.emit('video:event', { ...window.qa.states[0], request_id: '101', status: 'failed', error: '过期失败' })`);
    await delay(50);
    assert.equal(await evaluate(`document.querySelector('.material-row').innerText.includes('过期失败')`), false);
    await click('取消');
    await until(`!![...document.querySelectorAll('button')].find((b) => b.textContent.trim() === '重试')`);
    await click('重试');
    await evaluate(`Object.assign(window.qa.states[0], { status: 'ready', material_ready: true, progress: 1 }); window.qa.emit('video:event', window.qa.states[0])`);
    await until(`![...document.querySelectorAll('button')].find((b) => b.textContent.trim() === '开始生成').disabled`);
    console.log('PASS：failed 重试可达；-1 不定进度；旧事件隔离；取消重试；素材门禁');
    await click('识别信息');
    await until(`!!document.querySelector('.clip-times input')`);
    assert.match(await evaluate(`document.querySelector('.video-owner').innerText`), /QA 发布者/);
    await evaluate(`(() => { const input = document.querySelectorAll('.clip-times input')[1]; Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(input, '0:02'); input.dispatchEvent(new Event('input', { bubbles: true })); })()`);
    await click('下一首 →');
    await until(`!!document.querySelector('dialog[open]')`);
    await click('继续编辑');
    await click('保存 / 应用片段');
    await until(`document.querySelector('.clip-footer')?.textContent.includes('已保存')`);
    assert.equal(await evaluate(`window.qa.calls.filter((call) => call.name === 'ConfirmVideoRange').at(-1).args[1].duration_ms`), 2000);
    await click('下一首 →');
    await until(`document.querySelector('.song-title')?.textContent.includes('qa2')`);
    assert.equal(await evaluate(`!!document.querySelector('dialog[open]')`), false);
    await click('设置');
    await until(`!!document.querySelector('.cache-actions') || document.body.innerText.includes('清理渲染缓存')`);
    await until(`window.qa.calls.some((call) => call.name === 'ReleasePreview')`);
    await until(`![...document.querySelectorAll('button')].find((b) => b.textContent.trim() === '清理渲染缓存').disabled`);
    await evaluate(`window.qa.previewCount = 1`);
    await until(`[...document.querySelectorAll('button')].find((b) => b.textContent.trim() === '清理渲染缓存').disabled`);
    assert.equal(await evaluate(`[...document.querySelectorAll('button')].find((b) => b.textContent.trim() === '清理全部缓存').disabled`), true);
    console.log('PASS：真实组件保存回写、未保存切歌提醒、署名、预览释放和清理互斥');
    await click('生成视频');
    await click('开始生成');
    assert.equal(await evaluate(`window.qa.calls.filter((call) => call.name === 'StartRender').at(-1).args[0].songs[0].clip.duration_ms`), 2000);
    await evaluate(`window.qa.emit('job:event', { stage: 'done', status: 'done' })`);
    await until(`document.body.innerText.includes('视频已生成')`);
    assert.match(await evaluate(`document.querySelector('.completed-file').innerText`), /result.mp4/);
    console.log('PASS：生成使用已保存范围，完成页显示产物；桥接替身未产生真实成片');
    const screenshot = await send('Page.captureScreenshot');
    if (process.env.QA_SCREENSHOT) await writeFile(process.env.QA_SCREENSHOT, Buffer.from(screenshot.result.data, 'base64'));
  } finally {
    socket?.close();
    chromium.kill('SIGTERM');
    await once(chromium, 'exit').catch(() => {});
    server.closeAllConnections();
    await new Promise((resolve) => server.close(resolve));
    await rm(directory, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
  }
});
