import { describe, expect, it } from 'vitest';
import { mergeResync } from './resync.ts';

const parse = (row: { id: number }) => `server-${row.id}`;

describe('mergeResync', () => {
  it('keeps resolutions and match choices for rows still pending', () => {
    const previous = [{ id: 1, commit_status: 'pending' }, { id: 2, commit_status: 'failed' }];
    const refreshed = [{ id: 1, commit_status: 'pending' }, { id: 2, commit_status: 'failed' }];
    const result = mergeResync(previous, refreshed, new Map([[1, 'mine-1'], [2, 'mine-2']]), new Map([[1, 'match-a'], [2, 'new']]), parse);
    expect(result.changed).toBe(false);
    expect([...result.resolutions]).toEqual([[1, 'mine-1'], [2, 'mine-2']]);
    expect([...result.choices]).toEqual([[1, 'match-a'], [2, 'new']]);
  });

  it('drops the choice and resolution of a row recorded elsewhere and reports the change', () => {
    const previous = [{ id: 1, commit_status: 'pending' }, { id: 2, commit_status: 'pending' }];
    const refreshed = [{ id: 1, commit_status: 'pending' }, { id: 2, commit_status: 'committed' }];
    const result = mergeResync(previous, refreshed, new Map([[1, 'mine-1'], [2, 'mine-2']]), new Map([[1, 'match-a'], [2, 'match-b']]), parse);
    expect(result.changed).toBe(true);
    expect([...result.resolutions]).toEqual([[1, 'mine-1'], [2, 'server-2']]);
    expect([...result.choices]).toEqual([[1, 'match-a']]);
  });

  it('reads a resolution from the server for a pending row it has none for', () => {
    const result = mergeResync([], [{ id: 3, commit_status: 'pending' }], new Map(), new Map(), parse);
    expect(result.changed).toBe(false);
    expect([...result.resolutions]).toEqual([[3, 'server-3']]);
  });

  it('does not treat an already committed row as a change', () => {
    const rows = [{ id: 4, commit_status: 'committed' }];
    expect(mergeResync(rows, rows, new Map(), new Map(), parse).changed).toBe(false);
  });
});
