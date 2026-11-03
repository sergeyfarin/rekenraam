import { describe, expect, it } from 'vitest';
import { suggestionKey } from './suggestion-keys.ts';

describe('suggestionKey', () => {
  it('opens on the first or last option and wraps while moving', () => {
    expect(suggestionKey('ArrowDown', false, 3, -1)).toEqual({ kind: 'move', active: 0 });
    expect(suggestionKey('ArrowUp', false, 3, -1)).toEqual({ kind: 'move', active: 2 });
    expect(suggestionKey('ArrowDown', true, 3, 2)).toEqual({ kind: 'move', active: 0 });
    expect(suggestionKey('ArrowUp', true, 3, 0)).toEqual({ kind: 'move', active: 2 });
    // A list that shrank under the active option starts again.
    expect(suggestionKey('ArrowDown', true, 2, 5)).toEqual({ kind: 'move', active: 0 });
  });

  it('chooses only an active option, so Enter otherwise submits the form', () => {
    expect(suggestionKey('Enter', true, 3, 1)).toEqual({ kind: 'choose', index: 1 });
    expect(suggestionKey('Enter', true, 3, -1)).toEqual({ kind: 'none' });
    expect(suggestionKey('Enter', false, 3, 1)).toEqual({ kind: 'none' });
  });

  it('closes an open list on Escape and leaves Escape to the dialog otherwise', () => {
    expect(suggestionKey('Escape', true, 3, -1)).toEqual({ kind: 'close' });
    expect(suggestionKey('Escape', false, 3, -1)).toEqual({ kind: 'none' });
    expect(suggestionKey('Escape', true, 0, -1)).toEqual({ kind: 'none' });
    expect(suggestionKey('ArrowDown', true, 0, -1)).toEqual({ kind: 'none' });
  });
});
