import { describe, expect, it } from 'vitest';
import type { TagResponse } from '#lib/api/tags.ts';
import { submittableTagIDs, tagChoices } from './tag-choices';

const tag = (id: number, status: TagResponse['status'] = 'active'): TagResponse => ({
  id, book_id: 1, name: `Tag ${id}`, kind: 'flag', status, metadata: {}, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z'
});
const tags = [tag(1), tag(2, 'archived'), tag(3), tag(4, 'archived')];

describe('tagChoices', () => {
  it('offers every active tag and only the archived tags already assigned', () => {
    const choices = tagChoices(tags, [2], new Set([2]));
    expect(choices.map((choice) => [choice.tag.id, choice.archived, choice.retained, choice.blocked])).toEqual([
      [1, false, false, false],
      [2, true, true, false],
      [3, false, false, false]
    ]);
  });

  it('shows an archived prefill on a new record as blocked, not retained', () => {
    const choices = tagChoices(tags, [4, 1], new Set());
    expect(choices.find((choice) => choice.tag.id === 4)).toMatchObject({ archived: true, retained: false, blocked: true });
  });
});

describe('submittableTagIDs', () => {
  it('keeps a retained archived tag and drops a fresh archived assignment', () => {
    expect(submittableTagIDs([1, 2, 4], tags, new Set([2]))).toEqual([1, 2]);
  });

  it('sends the selection unchanged until the tag list has loaded', () => {
    expect(submittableTagIDs([1, 4], undefined, new Set())).toEqual([1, 4]);
  });
});
