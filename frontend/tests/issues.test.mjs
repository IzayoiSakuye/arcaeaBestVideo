import assert from 'node:assert/strict';
import test from 'node:test';
import { noteSongIndex, pendingSongIndexes } from '../src/issues.ts';

test('识别提示与校验问题合并为待校对曲目', () => {
  const notes = ['#27 曲绘无法准确识别', '表头校验和有误', '#3 定数读成「?」'];
  assert.equal(noteSongIndex(notes[0]), 27);
  assert.equal(noteSongIndex(notes[1]), 0);
  assert.deepEqual(pendingSongIndexes([{ song_index: 3 }, { song_index: 8 }], notes, [], 50), [3, 8, 27]);
  assert.deepEqual(pendingSongIndexes([], notes, [0, 2], 50), []);
});
