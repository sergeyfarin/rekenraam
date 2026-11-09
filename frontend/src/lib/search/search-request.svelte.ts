import type { SearchRequest } from './search-launcher.ts';
import type { SearchResult, SearchScope } from './search-model.ts';

type Select = (item: SearchResult) => void | Promise<void>;

/**
 * Which scope the shared search shows and who handles a chosen result (#490).
 * An openSearch request owns the interaction from its opening until it is
 * dismissed or its selection succeeds: its handler applies only while its own
 * scope is shown, so leaving for All restores ordinary selection, and a failed
 * selection keeps the request for a retry unless a newer interaction has
 * taken over.
 */
export class SearchRequestState {
  scope = $state<SearchScope>('all');
  request = $state<{ scope: SearchScope; onSelect?: Select }>();

  constructor(private readonly base: () => SearchScope, private readonly fallback: () => Select | undefined) {
    this.scope = base();
  }

  /** Back to the mounted scope with no request (mount, prop change, leaving a page). */
  reset() {
    this.request = undefined;
    this.scope = this.base();
  }

  start(next: SearchRequest) {
    this.request = { scope: next.scope ?? this.base(), onSelect: next.onSelect };
    this.scope = this.request.scope;
  }

  /** Ends the request, if any; ordinary search keeps its chosen scope. */
  dispose() {
    if (this.request) this.reset();
  }

  setScope(value: SearchScope) {
    this.scope = value;
  }

  /** The handler for a result chosen now; undefined means the default action. */
  handler(): Select | undefined {
    return this.request && this.scope === this.request.scope ? this.request.onSelect : this.fallback();
  }

  /**
   * Runs a chosen result. `release` closes the search before the destination
   * takes focus, without ending the request; `act` performs the selection.
   * On failure `failed` reopens the search, only while the same interaction
   * still owns it.
   */
  async choose(item: SearchResult, steps: {
    release: () => void;
    act: (select: Select | undefined, item: SearchResult) => void | Promise<void>;
    failed: () => void | Promise<void>;
  }): Promise<boolean> {
    const owner = this.request;
    const select = this.handler();
    steps.release();
    try {
      await steps.act(select, item);
    } catch {
      if (this.request === owner) await steps.failed();
      return false;
    }
    if (owner && this.request === owner) this.reset();
    return true;
  }
}
