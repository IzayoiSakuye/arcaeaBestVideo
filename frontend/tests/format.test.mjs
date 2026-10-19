import assert from 'node:assert/strict';
import test from 'node:test';
import { eventText, nameOf, scoreText } from '../src/format.ts';

test('文件名、分数和任务事件显示真实值', () => {
  assert.equal(nameOf('D:\\Videos\\result.mp4'), 'result.mp4');
  assert.equal(scoreText(99.87654), '9987654');
  assert.equal(eventText({ stage: 'render', song_index: 2, song_id: 'song-b', status: 'skipped', message: '' }), '第 2 首 使用缓存 · song-b');
  assert.equal(eventText({ stage: 'done', status: 'failed', error: '缺少录像', message: '' }), '缺少录像');
});
