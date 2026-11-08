import { describe, expect, it } from 'vitest';
import { latestRequest } from './latest-request.ts';

describe('latestRequest (#508)', () => {
  it('keeps the only request current', () => {
    const requests = latestRequest();
    const current = requests.start();
    expect(current()).toBe(true);
  });

  it('supersedes an earlier request when a later one starts', () => {
    const requests = latestRequest();
    const first = requests.start();
    const second = requests.start();
    expect(first()).toBe(false);
    expect(second()).toBe(true);
  });

  it('supersedes every started request when the inputs change', () => {
    const requests = latestRequest();
    const first = requests.start();
    requests.invalidate();
    expect(first()).toBe(false);
    expect(requests.start()()).toBe(true);
  });
});
