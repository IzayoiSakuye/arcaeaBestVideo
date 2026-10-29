import assert from 'node:assert/strict';
import test from 'node:test';
import { sameThought, thoughtFromProject } from './src/thought.ts';

test('首次保存后字段顺序变化仍判定为已保存', () => {
  const before = {
    name: '开场', pages: [{ text: '第一段', images: [] }], duration_ms: 10000,
    audio: { kind: 'silent', start_ms: 0, end_ms: 0 }, id: 'intro',
  };
  const saved = thoughtFromProject({
    id: 'intro', name: '开场', pages: before.pages, duration_ms: 10000,
    audio: { kind: 'silent', start_ms: 0, end_ms: 0 },
  });
  assert.equal(sameThought(before, saved), true);
  assert.equal(sameThought({ ...before, name: '改名' }, saved), false);
  assert.equal(sameThought({ ...before, pages: [{ text: '修改内容', images: [] }] }, saved), false);
});
