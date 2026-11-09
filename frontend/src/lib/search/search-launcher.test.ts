import { describe, expect, it, vi } from 'vitest';
import { openSearch, registerSearchOwner } from './search-launcher.ts';

describe('search launcher', () => {
  it('opens the registered owner with its scope and selection handler', () => {
    expect(openSearch({ scope: 'account' })).toBe(false);
    const owner = vi.fn(() => true);
    const release = registerSearchOwner(owner);
    const onSelect = vi.fn();
    expect(openSearch({ scope: 'account', onSelect })).toBe(true);
    expect(owner).toHaveBeenCalledWith({ scope: 'account', onSelect });
    release();
    expect(openSearch()).toBe(false);
  });

  it('keeps a newer owner when an older one unmounts later', () => {
    const older = registerSearchOwner(() => true);
    const newer = vi.fn(() => true);
    const releaseNewer = registerSearchOwner(newer);
    older();
    expect(openSearch()).toBe(true);
    expect(newer).toHaveBeenCalledOnce();
    releaseNewer();
  });

  it('reports a refusal from the owner, such as an open modal', () => {
    const release = registerSearchOwner(() => false);
    expect(openSearch({ scope: 'instrument' })).toBe(false);
    release();
  });
});
