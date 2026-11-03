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
