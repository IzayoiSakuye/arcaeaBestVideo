export function noteSongIndex(note: string): number {
  const match = /^#(\d+)\s/.exec(note);
  return match ? Number(match[1]) : 0;
}

export function pendingSongIndexes(problems: { song_index: number }[], notes: string[], acknowledged: number[], count: number): number[] {
  const indexes = new Set(problems.map(problem => problem.song_index));
  notes.forEach((note, index) => {
    if (!acknowledged.includes(index)) indexes.add(noteSongIndex(note));
  });
  return [...indexes].filter(index => index > 0 && index <= count).sort((a, b) => a - b);
}
