import { describe, expect, it } from 'vitest';
import { unavailableAllocations, withoutAllocations } from './lot-allocations';

describe('restored lot allocations (#521 review)', () => {
  it('names entered quantities on lots the position does not hold now', () => {
    expect(unavailableAllocations({ '1': '1', '2': '1', '3': ' ', '4': '' }, [1, 3])).toEqual(['2']);
    expect(unavailableAllocations({ '1': '1' }, ['1'])).toEqual([]);
    expect(unavailableAllocations({}, [])).toEqual([]);
  });

  it('removes only the named lots', () => {
    expect(withoutAllocations({ '1': '1', '2': '1' }, ['2'])).toEqual({ '1': '1' });
  });
});
