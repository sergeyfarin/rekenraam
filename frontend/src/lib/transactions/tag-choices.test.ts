import { describe, expect, it } from 'vitest';
import type { TagResponse } from '#lib/api/tags.ts';
import { assignedTagIDs, submittableTagIDs, tagChoices } from './tag-choices';

const tag = (id: number, status: TagResponse['status'] = 'active'): TagResponse => ({
  id, book_id: 1, name: `Tag ${id}`, kind: 'flag', status, metadata: {}, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z'
});
const active = [tag(1), tag(3)];

describe('tagChoices', () => {
  it('offers every active tag and only the archived tags already assigned', () => {
    const choices = tagChoices(active, [tag(2, 'archived'), tag(1)], [1, 2], new Set([1, 2]));
    expect(choices.map((choice) => [choice.tag.id, choice.archived, choice.retained, choice.blocked])).toEqual([
      [1, false, false, false],
      [3, false, false, false],
      [2, true, true, false]
    ]);
  });

  it('shows an archived prefill on a new record as blocked, not retained', () => {
    const choices = tagChoices(active, [tag(4, 'archived')], [4, 1], new Set());
    expect(choices.find((choice) => choice.tag.id === 4)).toMatchObject({ archived: true, retained: false, blocked: true });
  });

  it('keeps an assigned active tag the capped active list left out (#657)', () => {
    const choices = tagChoices([tag(1)], [tag(9)], [9], new Set([9]));
    expect(choices.map((choice) => [choice.tag.id, choice.archived])).toEqual([[1, false], [9, false]]);
  });

  it('lists an archived tag only once it is resolved, and never one the form was not filled with', () => {
    expect(tagChoices(active, undefined, [2], new Set([2])).map((choice) => choice.tag.id)).toEqual([1, 3]);
    expect(tagChoices(active, [tag(5, 'archived')], [], new Set()).map((choice) => choice.tag.id)).toEqual([1, 3]);
  });
});

describe('submittableTagIDs', () => {
  it('keeps a retained archived tag and drops a fresh archived assignment', () => {
    expect(submittableTagIDs([1, 2, 4], [tag(2, 'archived'), tag(4, 'archived')], new Set([2]))).toEqual([1, 2]);
  });

  it('sends the selection unchanged until the assigned tags are resolved', () => {
    expect(submittableTagIDs([1, 4], undefined, new Set())).toEqual([1, 4]);
  });
});

describe('assignedTagIDs', () => {
  it('collects transaction and posting tags once, in a stable order for the query key', () => {
    expect(assignedTagIDs([5, 2], [[2, 7], undefined, []])).toEqual([2, 5, 7]);
  });
});
