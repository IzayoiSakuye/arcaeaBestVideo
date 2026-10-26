import assert from 'node:assert/strict';
import test from 'node:test';
import { clipRangeError, formatClipTime, parseClipTime } from './src/clip.ts';

test('分:秒输入与显示保留小数精度', () => {
  assert.equal(formatClipTime(83.25), '1:23.25');
  assert.equal(parseClipTime('1:23.25'), 83.25);
  assert.equal(parseClipTime('1:60'), null);
});

test('起点晚于终点和零时长分别报错', () => {
  assert.equal(clipRangeError(12, 10, 60), '开始时间不能晚于结束时间');
  assert.equal(clipRangeError(10, 10, 60), '片段时长不能为 0');
  assert.equal(clipRangeError(10, 61, 60), '片段超出录像时长');
  assert.equal(clipRangeError(10, 12, 60), '');
});
