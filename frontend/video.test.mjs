import assert from 'node:assert/strict';
import test from 'node:test';
import { videoSourceKey } from './src/video.ts';

test('成绩变化不触发录像状态重查，来源变化会触发', () => {
  const song = { song_id: 'test', difficulty: { name: 'FTR' }, score: { achievement: 99 } };
  const before = videoSourceKey([song]);
  assert.equal(videoSourceKey([{ ...song, score: { achievement: 100 } }]), before);
  assert.notEqual(videoSourceKey([{ ...song, difficulty: { name: 'PRS' } }]), before);
  assert.notEqual(videoSourceKey([{ ...song, video: { url: 'https://example.com/video' } }]), before);
});

const { clipSaved, usedSongIndexes, mergeVideoSnapshot } = await import('./src/video.ts');

test('初始十秒不能算已保存，确认键与范围必须同时匹配', () => {
  assert.equal(clipSaved(undefined, 0, 10), false);
  assert.equal(clipSaved({ start_ms: 0, duration_ms: 10000 }, 0, 10), false);
  const clip = { start_ms: 12345, duration_ms: 2000, confirmed_key: 'saved-by-go' };
  assert.equal(clipSaved(clip, 12.345, 14.345), true);
  assert.equal(clipSaved(clip, 12.345, 15), false);
  assert.equal(clipSaved(clip, null, 14.345), false);
});

test('生成门禁只检查播放编排使用的歌曲，空编排不能回退为全部歌曲', () => {
  const songs = [{}, {}, {}];
  assert.deepEqual(usedSongIndexes({ songs }), [1, 2, 3]);
  assert.deepEqual(usedSongIndexes({ songs, play_order: [{ kind: 'thought' }, { kind: 'song', song_index: 2 }] }), [2]);
  assert.deepEqual(usedSongIndexes({ songs, play_order: [] }), []);
});

test('迟到的查询不会覆盖确认新任务或相同任务的下载事件，难度彼此隔离', () => {
  const state = { song_id: 'test', difficulty: 'FTR', request_id: '100', status: 'queued' };
  const event = { ...state, status: 'downloading' };
  assert.deepEqual(mergeVideoSnapshot([state], [event]), [event]);
  const confirmed = { ...state, request_id: '101', status: 'ready' };
  assert.deepEqual(mergeVideoSnapshot([state], [confirmed]), [confirmed]);
  assert.deepEqual(mergeVideoSnapshot([confirmed], [event]), [confirmed]);
  assert.deepEqual(mergeVideoSnapshot([state], [{ ...event, difficulty: 'BYD' }]), [state]);
});

const { connectMedia } = await import('./src/media.ts');
class Media extends EventTarget {
  paused = true;
  ended = false;
  readyState = 4;
  currentTime = 0;
  seeking = false;
  volume = 1;
  muted = false;
  emit(type) { this.dispatchEvent(new Event(type)); }
  play() {
    if (this.paused) {
      this.paused = false;
      queueMicrotask(() => { this.emit('play'); this.emit('playing'); });
    }
    return Promise.resolve();
  }
  pause() {
    if (!this.paused) { this.paused = true; queueMicrotask(() => this.emit('pause')); }
  }
}
const tick = () => new Promise((resolve) => setImmediate(resolve));

test('双流统一暂停、缓冲、恢复、拖动、循环，并在错误或卸载时停止', async () => {
  const video = new Media();
  const audio = new Media();
  let error = '';
  let waiting = false;
  const controls = { start: 10, end: 20, invalid: false,
    onTime() {}, onPlaying() {}, onError(value) { error = value; } };
  const stop = connectMedia(video, audio, () => controls, (value) => { waiting = value; });
  try {
    await video.play(); await tick();
    assert.equal(video.currentTime, 10);
    assert.equal(audio.currentTime, 10);
    assert.equal(video.muted, true);
    assert.equal(audio.muted, false);
    assert.equal(audio.volume, 1);
    assert.equal(audio.paused, false);
    video.pause(); await tick();
    assert.equal(audio.paused, true);
    await video.play(); await tick();
    audio.readyState = 2; audio.emit('waiting'); await tick();
    assert.equal(waiting, true);
    assert.equal(video.paused, true);
    assert.equal(audio.paused, true);
    audio.readyState = 4; audio.emit('canplay'); await tick();
    assert.equal(waiting, false);
    assert.equal(video.paused, false);
    assert.equal(audio.paused, false);
    video.currentTime = 15; audio.currentTime = 11; video.emit('seeking'); await tick();
    assert.equal(audio.currentTime, 15);
    video.emit('seeked'); await tick();
    video.currentTime = 20; video.emit('timeupdate');
    assert.equal(video.currentTime, 10);
    assert.equal(audio.currentTime, 10);
    audio.emit('error'); await tick();
    assert.match(error, /预览播放失败/);
    assert.equal(video.paused, true);
    assert.equal(audio.paused, true);
    await video.play(); await tick();
  } finally { stop(); }
  assert.equal(video.paused, true);
  assert.equal(audio.paused, true);
});

test('只有一路媒体时仍能播放；音频未缓冲时不能单独播放视频', async () => {
  const controls = { start: 0, end: 10, invalid: false, onTime() {}, onPlaying() {}, onError() {} };
  const video = new Media();
  const stopSingle = connectMedia(video, null, () => controls, () => {});
  try { await video.play(); await tick(); assert.equal(video.paused, false); assert.equal(video.muted, false); }
  finally { stopSingle(); }
  const audio = new Media(); audio.readyState = 1;
  const stopPair = connectMedia(video, audio, () => controls, () => {});
  try { await video.play(); await tick(); assert.equal(video.paused, true); assert.equal(audio.paused, true); }
  finally { stopPair(); }
});


test('拖动和未完成的 seek 不发布旧进度、不循环、不纠偏；暂停可定位到片段终点', async (t) => {
  t.mock.timers.enable({ apis: ['setInterval'] });
  const video = new Media();
  const audio = new Media();
  const times = [];
  const controls = { start: 10, end: 11, invalid: false, scrubbing: false,
    onTime(value) { times.push(value); }, onPlaying() {}, onError() {} };
  const stop = connectMedia(video, audio, () => controls, () => {});
  try {
    await video.play(); await tick();
    controls.scrubbing = true;
    times.length = 0;
    video.currentTime = 11;
    audio.currentTime = 10;
    video.emit('timeupdate'); t.mock.timers.tick(500);
    assert.deepEqual(times, []);
    assert.equal(video.currentTime, 11);
    assert.equal(audio.currentTime, 10);
    controls.scrubbing = false;
    video.currentTime = 10.8; video.seeking = true; video.emit('seeking'); await tick();
    times.length = 0;
    video.emit('timeupdate'); t.mock.timers.tick(500);
    assert.deepEqual(times, []);
    assert.equal(video.currentTime, 10.8);
    video.seeking = false; audio.seeking = true; video.emit('seeked');
    assert.deepEqual(times, []);
    audio.seeking = false; audio.emit('seeked'); await tick();
    assert.equal(video.currentTime, 10.8);
    assert.equal(audio.currentTime, 10.8);
    assert.ok(Math.abs(times.at(-1) - 0.8) < 0.001);
    assert.equal(video.paused, false);
    video.currentTime = 10.9; video.emit('timeupdate');
    assert.ok(times.at(-1) > 0.8);
    video.pause(); await tick();
    video.currentTime = 11; video.emit('timeupdate'); video.emit('seeked');
    assert.equal(video.currentTime, 11);
    assert.equal(audio.currentTime, 11);
    assert.equal(times.at(-1), 1);
    await video.play(); await tick();
    assert.equal(video.currentTime, 10);
    video.currentTime = 11; video.paused = true; video.emit('ended'); await tick();
    assert.equal(video.currentTime, 10);
    assert.equal(audio.currentTime, 10);
  } finally { stop(); }
});

test('自然 pause→ended 保留循环与拖动恢复意图，主动暂停不能被 seek 或缓冲恢复覆盖', async () => {
  for (const scenario of ['loop', 'hold', 'paused']) {
    const video = new Media();
    const audio = new Media();
    const controls = { start: 4, end: 5, invalid: false, scrubbing: false,
      onTime() {}, onPlaying() {}, onError() {} };
    const stop = connectMedia(video, audio, () => controls, () => {});
    try {
      await video.play(); await tick();
      controls.scrubbing = scenario !== 'loop';
      video.currentTime = 5;
      video.ended = scenario !== 'paused';
      video.pause(); await tick();
      video.emit('ended'); await tick();
      if (scenario === 'loop') assert.equal(video.currentTime, 4);
      else {
        assert.equal(video.currentTime, 5);
        controls.scrubbing = false;
        video.ended = false;
        video.currentTime = 4.6;
        video.emit('seeking'); video.emit('seeked'); audio.emit('canplay'); await tick();
        assert.equal(video.currentTime, 4.6);
        assert.equal(audio.currentTime, 4.6);
      }
      assert.equal(video.paused, scenario === 'paused', scenario);
      assert.equal(audio.paused, scenario === 'paused', scenario);
    } finally { stop(); }
  }
});
