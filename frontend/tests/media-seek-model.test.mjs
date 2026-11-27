import assert from 'node:assert/strict';
import test from 'node:test';
import { connectMedia } from '../src/media.ts';

// 模拟属性和事件队列，记录生产连接器的响应，不模拟真实解码器。
class Media extends EventTarget {
  paused = true;
  ended = false;
  seeking = false;
  readyState = 4;
  time = 1;
  writes = 0;
  get currentTime() { return this.time; }
  set currentTime(value) { this.writes++; this.time = value; }
  emit(event) { this.dispatchEvent(new Event(event)); }
  play() {
    if (this.paused) { this.paused = false; queueMicrotask(() => this.dispatchEvent(new Event('play'))); }
    return Promise.resolve();
  }
  pause() {
    if (!this.paused) { this.paused = true; queueMicrotask(() => this.dispatchEvent(new Event('pause'))); }
  }
}

test('受控暂停定位：正负残差、同位置、旧事件和连续新目标保持40ms精度且无seek回环', async (t) => {
  t.mock.timers.enable({ apis: ['setInterval'] });
  for (const offset of [0.072, -0.072, 0.041, -0.041, 0.079, -0.079, 0.081, -0.081]) {
    for (const order of ['queued', 'settled']) {
      const video = new Media(), audio = new Media();
      const events = [];
      const snapshot = (event) => events.push({ event, video: { time: video.currentTime, paused: video.paused, seeking: video.seeking, readyState: video.readyState, ended: video.ended },
        audio: { time: audio.currentTime, paused: audio.paused, seeking: audio.seeking, readyState: audio.readyState, ended: audio.ended } });
      const controls = { start: 1, end: 2, invalid: false, scrubbing: false, onTime() {}, onPlaying() {}, onError() {} };
      const stop = connectMedia(video, audio, () => controls, () => {});
      try {
        // 只注入播放属性，不发play意图，隔离暂停传播与显式seek精度。
        video.paused = false; audio.paused = false;
        video.pause(); snapshot('pause-called');
        const waitCanReturnBeforeAudioPause = !video.seeking && !audio.seeking && !audio.paused;
        if (order === 'settled') await Promise.resolve();
        // 显式注入目标时刻附近的音频位置，并发出模拟seek事件。
        video.currentTime = 1; audio.currentTime = 1 + offset;
        snapshot('injected-before-seek');
        video.dispatchEvent(new Event('seeking'));
        video.dispatchEvent(new Event('seeked'));
        snapshot('after-seek'); await Promise.resolve();
        t.mock.timers.tick(300); snapshot('after-300ms');
        const residual = Math.abs(audio.currentTime - 1);
        assert.ok(residual < 0.04, `offset=${offset} order=${order} residual=${residual}`);
        assert.equal(video.paused, true); assert.equal(audio.paused, true);
        const writes = audio.writes;
        for (const target of [1.8, 1.1, 1.172, 2, 1]) {
          video.currentTime = target; video.seeking = true; video.emit('seeking');
          // 陈旧事件在当前seek完成前不得发布进度或覆盖最新目标。
          audio.emit('seeked'); video.emit('timeupdate');
          video.seeking = false; video.emit('seeked');
          assert.ok(Math.abs(audio.currentTime - target) < 0.04);
        }
        const completedWrites = audio.writes;
        assert.ok(completedWrites - writes <= 5);
        audio.emit('seeked'); video.emit('seeked'); audio.emit('canplay'); t.mock.timers.tick(300);
        assert.equal(audio.writes, completedWrites, '完成后不能持续重seek');
        controls.scrubbing = true; video.emit('timeupdate');
        controls.scrubbing = false; video.emit('seeking'); video.emit('seeked');
        assert.equal(audio.writes, completedWrites, '同位置完成不重复设相同音频目标');
        controls.scrubbing = true; video.time = 1.7; audio.time = 1.4;
        video.emit('seeked'); video.emit('timeupdate');
        assert.equal(audio.writes, completedWrites, '拖动中旧seeked不能重设音频');
        controls.scrubbing = false; video.emit('seeking'); video.emit('seeked');
        assert.ok(Math.abs(audio.currentTime - 1.7) < 0.04);
        console.log(JSON.stringify({ layer: 'simulated-events', offset, order, waitCanReturnBeforeAudioPause, residual, within40ms: residual < 0.04, events }));
      } finally { stop(); }
    }
  }
});

test('缓冲/seek交叠尊重主动暂停，正常播放漂移阈值和单流不变', async (t) => {
  t.mock.timers.enable({ apis: ['setInterval'] });
  const video = new Media(), audio = new Media();
  const controls = { start: 1, end: 2, invalid: false, scrubbing: false, onTime() {}, onPlaying() {}, onError() {} };
  const tick = () => new Promise((resolve) => setImmediate(resolve));
  const stop = connectMedia(video, audio, () => controls, () => {});
  try {
    await video.play(); await tick();
    video.time = 1.5; audio.time = 1.428; video.emit('timeupdate');
    assert.equal(audio.currentTime, 1.428, '正常播放72ms漂移不增加纠偏');
    audio.time = 1.3; video.emit('timeupdate');
    assert.equal(audio.currentTime, 1.5, '正常播放大于150ms仍纠偏');
    audio.readyState = 2; audio.emit('waiting'); await tick();
    video.currentTime = 1.4; video.seeking = true; video.emit('seeking');
    audio.readyState = 4; audio.emit('canplay'); await tick();
    assert.equal(video.paused, true); assert.equal(audio.paused, true);
    video.seeking = false; video.emit('seeked'); await tick();
    assert.equal(video.paused, false); assert.equal(audio.paused, false);
    video.pause(); await tick();
    audio.time = 1.472; video.currentTime = 1.4; video.emit('seeking'); video.emit('seeked');
    audio.emit('waiting'); audio.emit('canplay'); await tick();
    assert.equal(video.paused, true); assert.equal(audio.paused, true);
    assert.ok(Math.abs(audio.currentTime - 1.4) < 0.04);
  } finally { stop(); }
  const single = new Media();
  const stopSingle = connectMedia(single, null, () => controls, () => {});
  try {
    single.currentTime = 1.8; single.emit('seeking'); single.emit('seeked');
    assert.equal(single.currentTime, 1.8); assert.equal(single.paused, true);
  } finally { stopSingle(); }
});
