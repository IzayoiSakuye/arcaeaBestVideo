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
  const editing = location.search.includes('editing');
  const songs = (editing ? Array.from({ length: 50 }, (_, i) => i + 1) : [1, 2]).map((index) => ({ index, song_id: `qa${index}`, difficulty: { name: 'FTR', constant: 10 },
    score: { achievement: 99, rank: 'EX', pure: 100, far: 0, lost: 0 }, user: { comment: '' },
    video: { url: `https://www.bilibili.com/video/BV1op4y1W7M2?p=${index}`, owner_name: 'QA 发布者' },
    clip: { start_ms: 0, duration_ms: 1000, confirmed_key: `saved${index}` } }));
  const project = { schema_version: 1, game: 'arcaea', player: { name: 'QA' }, source: { type: 'manual' },
    defaults: { clips: { start_ms: 0, duration_ms: 1000 } }, songs, thoughts: [], play_order: songs.map((song) => ({ kind: 'song', song_index: song.index })) };
  if (location.search.includes('order')) project.play_order = [{ kind: 'song', song_index: 2 }];
  if (editing) { songs[0].clip = undefined; songs[11].clip = undefined; songs[0].video.url = 'https://www.youtube.com/watch?v=qa1'; }
  const states = songs.map((song) => ({ index: song.index, song_id: song.song_id, difficulty: 'FTR', url: song.video.url,
    request_id: `${100 + song.index}`, status: song.index === 1 ? 'failed' : 'ready', error: song.index === 1 ? 'QA 缓存丢失' : '',
    preview_ready: true, preview_url: `/preview/${song.index}.mp4`, preview_audio_url: `/preview/${song.index}.m4a`,
    duration_ms: 3000, owner_name: 'QA 发布者', confirmed: true, material_ready: song.index === 2, progress: -1 }));
  if (editing) {
    for (const state of states) { state.confirmed = Boolean(songs[state.index - 1].clip); state.material_ready = state.confirmed; state.status = state.confirmed ? 'ready' : 'preview'; state.error = ''; }
    states[11].duration_ms = 15000;
    Object.assign(states[0], { status: 'downloading', preview_ready: false, progress: 0.1 });
  }
  const qa = window.qa = { calls, project, states, previewCount: 1, emit(name, value) { for (const cb of listeners.get(name) || []) cb(clone(value)); } };
  qa.cache = { rendering: false, downloading: false, in_use: false };
  qa.dialogOpens = 0;
  const showModal = HTMLDialogElement.prototype.showModal;
  HTMLDialogElement.prototype.showModal = function () { qa.dialogOpens++; return showModal.call(this); };
  const functions = {
    Status: () => ({ tools: [], song_count: 2 }), Version: () => 'qa',
    CheckResourceUpdate: () => ({ update_available: false }), CheckOPAudioUpdate: () => ({ files: [] }), ThoughtAudioOptions: () => [],
    Import: () => ({ Project: clone(project), Notes: [] }),
    Review: (value) => ({ project: clone(value), renderable: true, problems: [], rows: value.songs.map((song) => ({ song_id: song.song_id, title: location.search.includes('long') ? '超长标题LongTitle'.repeat(18) + song.song_id : song.song_id, difficulty: 'FTR', difficulties: ['FTR'], problems: [] })) }),
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
    CacheInfo: () => ({ preview_count: qa.previewCount, video_bytes: 20, render_bytes: 10, total_bytes: 30, ...qa.cache }),
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

test(process.env.QA_EDITING ? '真实组件：数字提交、下载中导航与草稿保留、长列表布局' : process.env.QA_QUERY_RACE ? '查询返回前到达的完成事件必须解除素材门禁' : '真实组件：重试、旧事件、进度、确认、切歌、清理和生成门禁', { timeout: 60000 }, async () => {
  const directory = await mkdtemp(join(tmpdir(), 'arcb9-ui-'));
  const frontend = resolve(import.meta.dirname);
  const server = createServer(async (request, response) => {
    try {
      const path = new URL(request.url, 'http://localhost').pathname;
      if (path === '/qa-font' && process.env.QA_FONT) { response.setHeader('Content-Type', 'font/collection'); response.end(await readFile(process.env.QA_FONT)); return; }
      const file = join(frontend, 'dist', path === '/' ? 'index.html' : path);
      let body = await readFile(file);
      if (path === '/') body = Buffer.from(body.toString().replace('<head>', `<head><script>(${bootstrap.toString()})();</script>`));
      if (path === '/' && process.env.QA_FONT) body = Buffer.from(body.toString().replace('</head>', '<style>@font-face{font-family:QA;src:url(/qa-font)}:root{font-family:QA,sans-serif}</style></head>'));
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
    await send('Page.navigate', { url: `http://127.0.0.1:${server.address().port}/${process.env.QA_EDITING ? '?editing=1' : process.env.QA_QUERY_RACE ? '?race=1' : process.env.QA_ORDER ? '?order=1' : '?qa=1'}${process.env.QA_LONG_TITLES ? '&long=1' : ''}` });
    await until(`!!document.querySelector('.nav-item')`);
    await click('JSON 导入');
    await evaluate(`(() => { const input = document.querySelector('textarea'); Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set.call(input, '{}'); input.dispatchEvent(new Event('input', { bubbles: true })); })()`);
    await click('导入');
    await until(`window.qa.calls.some((call) => call.name === 'VideoInfos')`);
    if (process.env.QA_ORDER) {
      await click('生成视频'); await click('选择路径');
      const disabled = `[...document.querySelectorAll('button')].find(b=>b.textContent.trim()==='开始生成').disabled`;
      await until(`!(${disabled})`);
      await evaluate(`Object.assign(window.qa.states[1],{status:'failed',material_ready:false,error:'编排内缓存失效'}); window.qa.emit('video:event',window.qa.states[1]);`);
      await until(disabled);
      await evaluate(`Object.assign(window.qa.states[1],{status:'ready',material_ready:true,error:''}); window.qa.emit('video:event',window.qa.states[1]);`);
      await until(`!(${disabled})`);
      console.log('PASS：未编排歌曲的失败不阻止生成，编排内素材失效会撤销 ready，恢复后重新允许生成');
      return;
    }
    if (process.env.QA_LAYOUT_ONLY) {
      await click('生成视频');
      await evaluate(`for (const state of window.qa.states) { Object.assign(state, { confirmed: false, material_ready: false }); window.qa.emit('video:event', state); }`);
      for (const width of [1600, 1280, 800, 480]) {
        await send('Emulation.setDeviceMetricsOverride', { width, height: 900, deviceScaleFactor: 1, mobile: false });
        await delay(100);
        await evaluate('document.fonts.ready');
        console.log('布局：', width, await evaluate(`JSON.stringify({overflow:document.documentElement.scrollWidth>innerWidth, nav:[...document.querySelectorAll('.nav-item')].map(b=>({text:b.textContent, width:b.clientWidth, scroll:b.scrollWidth}))})`));
        const screenshot = await send('Page.captureScreenshot');
        await writeFile(process.env.QA_SCREENSHOT.replace('.png', '-' + width + '.png'), Buffer.from(screenshot.result.data, 'base64'));
      }
      return;
    }
    if (process.env.QA_EDITING) {
      const input = async (selector, value) => {
        await evaluate(`(() => { const input = document.querySelector(${JSON.stringify(selector)}); input.focus(); input.select(); })()`);
        if (value) await send('Input.insertText', { text: value });
        else { await send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Backspace', windowsVirtualKeyCode: 8 }); await send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Backspace', windowsVirtualKeyCode: 8 }); }
      };
      const jump = '.review-toolbar input[type=number]';
      const enter = async () => {
        await send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 13 });
        await send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 13 });
      };
      const at = (index) => until(`document.querySelector('.heading-tag')?.textContent === '第 ${index} / 50 首'`);
      const generateDisabled = `[...document.querySelectorAll('button')].find((b) => b.textContent.trim() === '开始生成').disabled`;
      await click('识别信息'); await at(1);
      if (process.env.QA_NAV_GESTURES) {
        const key = async (name, code, vk) => {
          await send('Input.dispatchKeyEvent', { type: 'keyDown', key: name, code, windowsVirtualKeyCode: vk, ...(name === 'Enter' ? {text:'\r'} : name === ' ' ? {text:' '} : {}) });
          await send('Input.dispatchKeyEvent', { type: 'keyUp', key: name, code, windowsVirtualKeyCode: vk });
        };
        const pointFor = (label) => evaluate(`(() => { const b=[...document.querySelectorAll('button')].find(b=>b.textContent.trim()===${JSON.stringify(label)}); b.scrollIntoView({block:'center'}); const r=b.getBoundingClientRect(); return {x:r.x+r.width/2,y:r.y+r.height/2}; })()`);
        const pointer = async (label) => {
          const point = await pointFor(label);
          await send('Input.dispatchMouseEvent', { type: 'mousePressed', ...point, button: 'left', clickCount: 1 });
          await send('Input.dispatchMouseEvent', { type: 'mouseReleased', ...point, button: 'left', clickCount: 1 });
          await delay(60);
        };
        const next = '下一首 →', previous = '← 上一首';
        await input(jump, '10'); await enter(); await at(10);
        const scenario = process.env.QA_NAV_GESTURES;
        if (scenario === 'pointer') {
          await input(jump, '11'); await pointer(previous); await at(11);
          await pointer(previous); await at(10);
          for (const [label, index] of [[next,11],[previous,10],[previous,9],[next,10]]) { await pointer(label); await at(index); }
        } else if (scenario === 'keyboard') {
          await evaluate(`window.qa.keys=[]; for(const name of ['keydown','keyup','click']) document.addEventListener(name,e=>window.qa.keys.push({type:e.type,key:e.key,target:e.target.textContent?.trim().slice(0,30),trusted:e.isTrusted}),true);`);
          await input(jump, '10');
          await key('Tab', 'Tab', 9); await key('Tab', 'Tab', 9);
          assert.equal(await evaluate('document.activeElement.textContent.trim()'), previous);
          await key('Enter', 'Enter', 13); console.log('键盘事件：', await evaluate('window.qa.keys')); await at(9);
          await key('Tab', 'Tab', 9);
          assert.equal(await evaluate('document.activeElement.textContent.trim()'), next);
          await key(' ', 'Space', 32); await at(10);
        } else if (scenario === 'cancel') {
          await input(jump, '8');
          const point = await pointFor(previous);
          await send('Input.dispatchMouseEvent', { type: 'mousePressed', ...point, button: 'left', clickCount: 1 });
          await at(8);
          await send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: point.x, y: point.y + 100, buttons: 1 });
          await send('Input.dispatchMouseEvent', { type: 'mouseReleased', x: point.x, y: point.y + 100, button: 'left', clickCount: 1 });
          await pointer(next); await at(9); await pointer(previous); await at(8);
        } else if (scenario === 'touch-cancel') {
          await evaluate(`window.qa.pointerCancels=0; document.addEventListener('pointercancel',()=>window.qa.pointerCancels++);`);
          await input(jump, '8'); const point = await pointFor(previous);
          await send('Emulation.setTouchEmulationEnabled', { enabled: true });
          await send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{...point,id:1}] });
          await send('Input.dispatchTouchEvent', { type: 'touchCancel', touchPoints: [] });
          await delay(80);
          assert.equal(await evaluate('window.qa.pointerCancels'), 1, '必须产生真实 pointercancel');
          const moved = await evaluate(`document.querySelector('.heading-tag').textContent === '第 8 / 50 首'`);
          await pointer(next); await at(moved ? 9 : 8);
          await pointer(next); await at(moved ? 10 : 9);
        } else if (scenario === 'draft') {
          const value = process.env.QA_NAV_DRAFT === 'empty' ? '' : process.env.QA_NAV_DRAFT;
          const label = process.env.QA_NAV_DIRECTION === 'previous' ? previous : next;
          await input(jump, value); await pointer(label);
          console.log('无有效数字跳转时点击下一首：', JSON.stringify({value, heading:await evaluate(`document.querySelector('.heading-tag').textContent`)}));
          if (process.env.QA_SCREENSHOT) { const shot = await send('Page.captureScreenshot'); await writeFile(process.env.QA_SCREENSHOT, Buffer.from(shot.result.data, 'base64')); }
          assert.equal(await evaluate(`document.querySelector('.heading-tag').textContent`), `第 ${label === next ? 11 : 9} / 50 首`, '无效或当前项未发生跳转时，相邻按钮仍应执行有效导航');
        } else throw Error(`未知导航专项：${scenario}`);
        assert.equal(await evaluate(`!!document.querySelector('dialog[open]')`), false);
        console.log(`PASS：独立导航专项 ${scenario}`);
        return;
      }
      if (process.env.QA_NAV_CLICK) {
        const target = process.env.QA_NAV_TARGET || '12';
        const label = process.env.QA_NAV_CLICK === 'previous' ? '← 上一首' : '下一首 →';
        await input(jump, target);
        const point = await evaluate(`(() => { const b = [...document.querySelectorAll('button')].find(b => b.textContent.trim() === ${JSON.stringify(label)}); b.scrollIntoView({block:'center'}); const r=b.getBoundingClientRect(); return {x:r.x+r.width/2,y:r.y+r.height/2}; })()`);
        await send('Input.dispatchMouseEvent', { type: 'mousePressed', ...point, button: 'left', clickCount: 1 });
        await send('Input.dispatchMouseEvent', { type: 'mouseReleased', ...point, button: 'left', clickCount: 1 });
        await delay(100);
        console.log(`失焦并点击${label}：`, await evaluate(`JSON.stringify({heading:document.querySelector('.heading-tag')?.textContent, dialog:document.querySelector('dialog[open]')?.innerText, calls:window.qa.calls.filter(c=>['ConfirmVideoRange','CancelVideoPreparation'].includes(c.name))})`));
        if (process.env.QA_SCREENSHOT) { const shot = await send('Page.captureScreenshot'); await writeFile(process.env.QA_SCREENSHOT, Buffer.from(shot.result.data, 'base64')); }
        assert.equal(await evaluate(`!!document.querySelector('dialog[open]')`), false, '下载中无修改：输入失焦并点击下一首只能切换一次，不能在跳转后再次触发未保存提示');
        assert.equal(await evaluate(`document.querySelector('.heading-tag').textContent`), `第 ${target} / 50 首`);
        // 下一次独立手势仍应导航；同一次数字提交也不能绕过真实修改保护。
        await input(jump, '1'); await enter();
        if (target === '12') { await until(`!!document.querySelector('dialog[open]')`); await click('应用并切换'); }
        await at(1);
        await input('.clip-times label:nth-child(2) input', '0:02');
        const opens = await evaluate('window.qa.dialogOpens');
        await input(jump, target);
        await send('Input.dispatchMouseEvent', { type: 'mousePressed', ...point, button: 'left', clickCount: 1 });
        await send('Input.dispatchMouseEvent', { type: 'mouseReleased', ...point, button: 'left', clickCount: 1 });
        await until(`!!document.querySelector('dialog[open]')`);
        assert.equal(await evaluate('window.qa.dialogOpens'), opens + 1);
        assert.equal(await evaluate(`document.querySelector('.heading-tag').textContent`), '第 1 / 50 首');
        await click('继续编辑');
        assert.equal(await evaluate(`document.querySelector('${jump}').value`), '1');
        assert.equal(await evaluate(`document.querySelector('.clip-times label:nth-child(2) input').value`), '0:02');
        console.log('PASS：后续独立导航可用，真实修改只触发一次保护，取消后输入和草稿保留');
        return;
      }
      await input(jump, '1'); await input(jump, '12');
      assert.equal(await evaluate(`document.querySelector('.heading-tag').textContent`), '第 1 / 50 首');
      assert.equal(await evaluate(`!!document.querySelector('dialog[open]')`), false);
      await enter(); await at(12);
      assert.equal(await evaluate(`window.qa.calls.filter((c) => ['ConfirmVideoRange', 'CancelVideoPreparation'].includes(c.name)).length`), 0);
      for (const value of ['', '0', '51', '12']) {
        await input(jump, value); await enter(); await at(12);
        assert.equal(await evaluate(`document.querySelector('${jump}').value`), '12');
        assert.equal(await evaluate(`!!document.querySelector('dialog[open]')`), false);
      }
      await input(jump, '5'); await input(jump, '50');
      assert.equal(await evaluate(`document.querySelector('.heading-tag').textContent`), '第 12 / 50 首');
      const opens = await evaluate('window.qa.dialogOpens');
      await enter(); await until(`!!document.querySelector('dialog[open]')`);
      assert.equal(await evaluate('window.qa.dialogOpens'), opens + 1);
      await click('继续编辑'); await at(12);
      assert.equal(await evaluate(`document.querySelector('${jump}').value`), '12');
      await input(jump, '50'); await evaluate(`document.querySelector('${jump}').blur()`);
      await click('应用并切换'); await at(50);
      assert.equal(await evaluate(`window.qa.calls.filter((c) => c.name === 'ConfirmVideoRange').at(-1).args[0].song_id`), 'qa12');
      await input(jump, '1'); await enter(); await at(1);
      assert.equal(await evaluate(`document.querySelector('.video-empty').textContent`), '下载素材中');
      await input('.clip-times label:nth-child(2) input', '0:02');
      await input('.right-stack > .video-card:nth-child(2) .field input', 'https://www.youtube.com/watch?v=qa-draft');
      await click('下一首 →'); await click('继续编辑'); await at(1);
      await click('下一首 →'); await click('保留草稿并切换'); await at(2);
      await click('生成视频'); await click('选择路径');
      assert.equal(await evaluate(generateDisabled), true, '保留草稿不能绕过生成确认');
      await click('识别信息');
      await input(jump, '1'); await enter(); await at(1);
      assert.equal(await evaluate(`document.querySelector('.right-stack > .video-card:nth-child(2) .field input').value`), 'https://www.youtube.com/watch?v=qa-draft');
      await input('.right-stack > .video-card:nth-child(2) .field input', 'https://www.youtube.com/watch?v=qa1');
      assert.equal(await evaluate(`document.querySelector('.clip-times label:nth-child(2) input').value`), '0:02');
      assert.equal(await evaluate(`window.qa.calls.filter((c) => c.name === 'ConfirmVideoRange' && c.args[0].song_id === 'qa1').length`), 0);
      await evaluate(`Object.assign(window.qa.states[0], { status: 'preview', preview_ready: true, confirmed: false }); window.qa.emit('video:event', window.qa.states[0])`);
      await until(`[...document.querySelectorAll('button')].some((b) => b.textContent.trim() === '应用片段' && !b.disabled)`);
      await click('应用片段');
      await until(`window.qa.calls.some((c) => c.name === 'ConfirmVideoRange' && c.args[0].song_id === 'qa1')`);
      await click('下一首 →'); await at(2);
      await evaluate(`document.querySelector('[aria-label="片段预览进度"]').scrollIntoView({ block: 'center' })`);
      const slider = await evaluate(`(() => { const r = document.querySelector('[aria-label="片段预览进度"]').getBoundingClientRect(); return { x: r.x + r.width * 0.8, y: r.y + r.height / 2 }; })()`);
      await send('Input.dispatchMouseEvent', { type: 'mousePressed', ...slider, button: 'left', clickCount: 1 });
      await evaluate(`const video = document.querySelector('.video-card video'); video.currentTime = 0.1; video.dispatchEvent(new Event('timeupdate'));`);
      assert.ok(await evaluate(`Number(document.querySelector('[aria-label="片段预览进度"]').value) >= 0.6`), '拖动中旧 timeupdate 不得拉回滑条');
      await send('Input.dispatchMouseEvent', { type: 'mouseReleased', ...slider, button: 'left', clickCount: 1 });
      assert.ok(await evaluate(`document.querySelector('.video-card video').currentTime >= 0.6`), '松开后提交用户目标');
      await click('生成视频'); await click('选择路径'); await until(`!(${generateDisabled})`);
      assert.equal(await evaluate(`document.querySelectorAll('.material-row').length`), 0);
      await evaluate(`Object.assign(window.qa.states[0], { status: 'failed', material_ready: false, error: '已应用素材下载失败' }); window.qa.emit('video:event', window.qa.states[0])`);
      await until(`!!document.querySelector('.material-status')`);
      assert.equal(await evaluate(generateDisabled), true);
      assert.match(await evaluate(`document.querySelector('.material-status').textContent`), /已应用素材下载失败/);
      assert.equal(await evaluate(`document.querySelectorAll('.material-row').length`), 0);
      await click('重试');
      assert.equal(await evaluate(`window.qa.calls.filter((c) => c.name === 'RetryVideoPreparation').at(-1).args[0]`), 'qa1');
      assert.equal(await evaluate(generateDisabled), true, '重试排队期间仍禁止生成');
      await click('取消');
      await until(`document.querySelector('.material-status')?.textContent.includes('已取消')`);
      assert.equal(await evaluate(generateDisabled), true, '已应用但取消下载仍禁止生成');
      await click('重试');
      await evaluate(`Object.assign(window.qa.states[0], { status: 'ready', material_ready: true, error: '' }); window.qa.emit('video:event', window.qa.states[0])`);
      await until(`!(${generateDisabled})`);
      assert.equal(await evaluate(`document.querySelectorAll('.material-row').length`), 0);
      for (const width of [1600, 1280, 800, 480]) {
        await evaluate(`for (const state of window.qa.states) { Object.assign(state, { confirmed: false, material_ready: false }); window.qa.emit('video:event', state); }`);
        await send('Emulation.setDeviceMetricsOverride', { width, height: 900, deviceScaleFactor: 1, mobile: false });
        await until(`document.querySelectorAll('.material-row').length === 50`);
        const layout = await evaluate(`(() => { const panel = document.querySelector('.material-summary'), log = document.querySelector('.log-card'), main = document.querySelector('.content-grid > section'); const a = panel.getBoundingClientRect(), b = log.getBoundingClientRect(), c = main.getBoundingClientRect(); return { overflow: document.documentElement.scrollWidth > innerWidth || panel.scrollWidth > panel.clientWidth, count: document.querySelectorAll('.material-summary').length, above: a.bottom <= b.top, side: a.left >= c.right, scroll: panel.scrollHeight > panel.clientHeight }; })()`);
        assert.equal(layout.overflow, false, `横向溢出：${width}`);
        assert.equal(layout.count, 1); assert.equal(layout.above, true); assert.equal(layout.scroll, true);
        if (width > 1150) assert.equal(layout.side, true);
        await evaluate(`document.querySelector('.main-area').scrollTop = 0; document.querySelector('.material-summary').scrollTop = 0`);
        if (process.env.QA_SCREENSHOT) {
          await evaluate('document.fonts.ready');
          const screenshot = await send('Page.captureScreenshot');
          await writeFile(process.env.QA_SCREENSHOT.replace('.png', `-${width}.png`), Buffer.from(screenshot.result.data, 'base64'));
        }
      }
      await click('设置');
      assert.doesNotMatch(await evaluate('document.body.innerText'), /缓存：|素材 .* MB · 渲染/);
      console.log('PASS：12/50 延迟提交、Enter 不重复、失焦提交、取消/空值/越界/当前条目、下载中导航、链接与范围草稿保留、确认回写与生成门禁、已应用失败重试、50条列表四种窗口布局');
      return;
    }
    if (process.env.QA_QUERY_RACE) {
      await until(`typeof window.qa.resolveQuery === 'function'`);
      await evaluate(`Object.assign(window.qa.states[0], { status: 'ready', material_ready: true, progress: 1, error: '' }); window.qa.emit('video:event', window.qa.states[0]); window.qa.resolveQuery()`);
      await click('生成视频');
      await until(`!!document.querySelector('.material-summary')`);
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
    await until(`!!document.querySelector('.material-summary')`);
    await click('选择路径');
    assert.equal(await evaluate(`[...document.querySelectorAll('button')].find((b) => b.textContent.trim() === '开始生成').disabled`), true);
    await click('重试');
    await until(`!!document.querySelector('progress')`);
    assert.equal(await evaluate(`document.querySelector('progress').hasAttribute('value')`), false);
    await evaluate(`window.qa.emit('video:event', { ...window.qa.states[0], request_id: '101', status: 'failed', error: '过期失败' })`);
    await delay(50);
    assert.equal(await evaluate(`document.querySelector('.material-summary').innerText.includes('过期失败')`), false);
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
    await click('应用片段');
    await until(`[...document.querySelectorAll('button')].find((b) => b.textContent.trim() === '应用片段')?.disabled && window.qa.calls.some((call) => call.name === 'ConfirmVideoRange')`);
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
    for (const flag of ['downloading', 'rendering', 'in_use']) {
      await evaluate(`window.qa.previewCount = 0; window.qa.cache = {rendering:false,downloading:false,in_use:false,${flag}:true}`);
      await until(`[...document.querySelectorAll('button')].find(b=>b.textContent.trim()==='清理渲染缓存').disabled`);
      assert.equal(await evaluate(`[...document.querySelectorAll('button')].find(b=>b.textContent.trim()==='清理全部缓存').disabled`), true, `${flag} 时禁止清理`);
    }
    await evaluate(`window.qa.cache = {rendering:false,downloading:false,in_use:false}`);
    await until(`![...document.querySelectorAll('button')].find(b=>b.textContent.trim()==='清理渲染缓存').disabled`);
    console.log('PASS：真实组件保存回写、未保存切歌提醒、署名、预览释放和清理互斥');
    await click('生成视频');
    await click('开始生成');
    assert.equal(await evaluate(`window.qa.calls.filter((call) => call.name === 'StartRender').at(-1).args[0].songs[0].clip.duration_ms`), 2000);
    assert.equal(await evaluate(`window.qa.calls.filter((call) => call.name === 'StartRender').at(-1).args[0].songs[0].clip.confirmed_key`), 'new-saved');
    assert.equal(await evaluate(`window.qa.calls.filter((call) => call.name === 'StartRender').at(-1).args[0].songs[0].video.owner_name`), 'QA 发布者');
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
