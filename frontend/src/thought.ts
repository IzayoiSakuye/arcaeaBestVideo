import type { score } from '../wailsjs/go/models';

export type ThoughtDraft = {
  id?: string;
  name: string;
  pages: { text: string; images: { path: string; x: number; y: number; width: number }[] }[];
  duration_ms: number;
  audio: { kind: string; path?: string; start_ms: number; end_ms: number };
};

export const emptyThought = (): ThoughtDraft => ({
  name: '', pages: [{ text: '', images: [] }], duration_ms: 10000, audio: { kind: 'silent', start_ms: 0, end_ms: 0 },
});

export function thoughtFromProject(source?: score.Thought): ThoughtDraft | null {
  if (!source) return null;
  const pages = source.pages?.length ? source.pages.map((page) => ({ text: page.text, images: page.images || [] }))
    : (source.texts || []).map((text) => ({ text, images: source.image ? [source.image] : [] }));
  return {
    id: source.id,
    name: source.name, pages: pages.length ? pages : [{ text: '', images: [] }], duration_ms: source.duration_ms,
    audio: { kind: source.audio.kind, path: source.audio.path, start_ms: source.audio.start_ms || 0, end_ms: source.audio.end_ms || 0 },
  };
}

export function sameThought(a: ThoughtDraft | null, b: ThoughtDraft | null): boolean {
  if (!a || !b) return a === b;
  const fields = (thought: ThoughtDraft) => JSON.stringify({
    id: thought.id || '', name: thought.name, duration_ms: thought.duration_ms,
    audio: [thought.audio.kind, thought.audio.path || '', thought.audio.start_ms, thought.audio.end_ms],
    pages: thought.pages.map((page) => [page.text, page.images.map((image) => [image.path, image.x, image.y, image.width])]),
  });
  return fields(a) === fields(b);
}
