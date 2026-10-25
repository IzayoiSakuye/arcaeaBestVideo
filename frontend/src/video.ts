export function videoQueues<T extends { status: string; url: string }>(states: T[]) {
  const pending = states.filter((state) => state.status !== 'ready');
  const retryable = pending.filter(
    (state) => (state.status === 'download' || state.status === 'legacy') && state.url !== '',
  );
  return { pending, retryable, manualCount: pending.length - retryable.length };
}
