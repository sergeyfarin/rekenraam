/**
 * Tracks which automatic preview request is the current one (#508). A form
 * whose preview follows its inputs starts a request per settled change; when
 * the inputs change again, or a later request starts, the earlier response
 * describes a sale the owner is no longer entering. Its success, error and
 * completion must all be ignored: settling it would show an old review and
 * enable the confirm button while the current request is still pending.
 */
export type LatestRequest = {
  /** The inputs changed: every request started so far is superseded. */
  invalidate(): void;
  /** Starts a request; the returned check is true while it is still current. */
  start(): () => boolean;
};

export function latestRequest(): LatestRequest {
  let current = 0;
  return {
    invalidate() { current += 1; },
    start() {
      current += 1;
      const id = current;
      return () => id === current;
    }
  };
}
