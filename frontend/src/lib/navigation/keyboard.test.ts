import { afterEach, describe, expect, it, vi } from 'vitest';
import { shellShortcut } from './keyboard';

// Node has no DOM; a stand-in element is editable when it says so.
class FakeElement {
  constructor(private readonly editable = false) {}
  isContentEditable = false;
  closest() { return this.editable ? this : null; }
}

function key(init: Partial<KeyboardEvent> & { key: string }, target: unknown = null): KeyboardEvent {
  return { defaultPrevented: false, isComposing: false, repeat: false, altKey: false, ctrlKey: false, metaKey: false, shiftKey: false, target, ...init } as KeyboardEvent;
}

describe('shell shortcuts', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('maps N and ? outside fields', () => {
    expect(shellShortcut(key({ key: 'n' }))).toBe('add-transaction');
    expect(shellShortcut(key({ key: 'N' }))).toBe('add-transaction');
    expect(shellShortcut(key({ key: '?', shiftKey: true }))).toBe('help');
  });

  it('never acts in an editable field', () => {
    vi.stubGlobal('HTMLElement', FakeElement);
    expect(shellShortcut(key({ key: 'n' }, new FakeElement(true)))).toBeUndefined();
    expect(shellShortcut(key({ key: '?', shiftKey: true }, new FakeElement(true)))).toBeUndefined();
    expect(shellShortcut(key({ key: 'n' }, new FakeElement(false)))).toBe('add-transaction');
  });

  it('leaves modified, repeated, composed and handled keys alone', () => {
    for (const init of [{ ctrlKey: true }, { metaKey: true }, { altKey: true }, { shiftKey: true }, { repeat: true }, { isComposing: true }, { defaultPrevented: true }]) {
      expect(shellShortcut(key({ key: 'n', ...init })), JSON.stringify(init)).toBeUndefined();
    }
    expect(shellShortcut(key({ key: '?', shiftKey: true, ctrlKey: true }))).toBeUndefined();
    expect(shellShortcut(key({ key: '/' }))).toBeUndefined();
  });
});
