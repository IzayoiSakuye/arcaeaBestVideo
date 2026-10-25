import assert from 'node:assert/strict';
import test from 'node:test';
import { videoQueues } from '../src/video.ts';

test('无录像来源的曲目仍计入待补，不计入可自动重试', () => {
  const states = [
    { index: 1, status: 'ready', url: 'https://www.bilibili.com/video/BV1op4y1W7M2' },
    { index: 2, status: 'missing', url: '' },
    { index: 3, status: 'download', url: 'https://www.bilibili.com/video/BV1op4y1W7M2?p=2' },
    { index: 4, status: 'invalid', url: 'bad-url' },
  ];
  const queues = videoQueues(states);
  assert.deepEqual(queues.pending.map((state) => state.index), [2, 3, 4]);
  assert.deepEqual(queues.retryable.map((state) => state.index), [3]);
  assert.equal(queues.manualCount, 2);
});
