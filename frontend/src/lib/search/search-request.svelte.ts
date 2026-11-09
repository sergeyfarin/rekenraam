import type { SearchRequest } from './search-launcher.ts';
import type { SearchResult, SearchScope } from './search-model.ts';

type Select = (item: SearchResult) => void | Promise<void>;

/**
 * Which scope the shared search shows and who handles a chosen result (#490).
 * Every opening is an interaction, ordinary or an openSearch request, and
 * lasts until it is dismissed, reset or its selection succeeds. A request's
 * handler applies only while its own scope is shown, so leaving for All
 * restores ordinary selection. A failed selection reopens search, request and
 * all, only while its interaction is still the current one: never after a
 * reset, a dismissal or a newer opening.
 */
export class SearchRequestState {
  scope = $state<SearchScope>('all');
  request = $state<{ scope: SearchScope; onSelect?: Select }>();
  // Which interaction is current; a pending selection compares against it.
  private interaction = 0;

  constructor(private readonly base: () => SearchScope, private readonly fallback: () => Select | undefined) {
    this.scope = base();
  }

  /** Back to the mounted scope with no request (mount, prop change, leaving a page). */
  reset() {
    this.interaction += 1;
    this.request = undefined;
    this.scope = this.base();
  }

  /** An openSearch request: a new interaction with its scope and handler. */
  start(next: SearchRequest) {
    this.interaction += 1;
    this.request = { scope: next.scope ?? this.base(), onSelect: next.onSelect };
    this.scope = this.request.scope;
  }

  /**
   * Ends the current interaction (dismissal, or before an ordinary opening).
   * A request goes with it; ordinary search keeps its chosen scope.
   */
  dispose() {
    if (this.request) this.reset();
    else this.interaction += 1;
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
    const owner = this.interaction;
    const select = this.handler();
    steps.release();
    try {
      await steps.act(select, item);
    } catch {
      if (this.interaction === owner) await steps.failed();
      return false;
    }
    if (this.interaction === owner) this.dispose();
    return true;
  }
}
