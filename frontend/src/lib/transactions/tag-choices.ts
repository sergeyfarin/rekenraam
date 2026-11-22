import type { TagResponse } from '#lib/api/tags.ts';

/**
 * One tag the transaction form shows (#650). Archiving a tag stops new
 * assignments but keeps the ones it already has, so the form offers every
 * active tag, and an archived tag only where it is already assigned:
 *
 * - `retained`: the saved record (a transaction or a recurring template)
 *   carries it. The owner can keep it or clear it, and the server keeps the
 *   assignment while it is restated.
 * - `blocked`: the form was filled with it, but saving would be a fresh
 *   assignment (a new transaction or a correction). It is shown so nothing
 *   disappears unannounced, and it is never sent.
 */
export interface TagChoice {
  tag: TagResponse;
  archived: boolean;
  retained: boolean;
  blocked: boolean;
}

/**
 * The tags the form lists: the active choices in the server's order, then the
 * tags the form was filled with that the active list does not hold.
 *
 * Active choices and assigned tags come from separate reads (#657): the
 * active list never includes archived rows, which in a large book would
 * crowd it past the list limit, and assigned tags are resolved by ID, so an
 * assignment shows whatever its position in the catalog.
 *
 * @param activeTags the active tag list
 * @param assignedTags the tags the form was filled with, resolved by ID with
 *   archived ones included; undefined while that read is pending
 * @param initialTagIDs the tags the form was filled with
 * @param retainable the tags the saved record already carries; empty when
 *   saving creates a new record
 */
export function tagChoices(
  activeTags: readonly TagResponse[],
  assignedTags: readonly TagResponse[] | undefined,
  initialTagIDs: readonly number[],
  retainable: ReadonlySet<number>
): TagChoice[] {
  const choices: TagChoice[] = activeTags.map((tag) => ({ tag, archived: false, retained: false, blocked: false }));
  const listed = new Set(activeTags.map((tag) => tag.id));
  const initial = new Set(initialTagIDs);
  for (const tag of assignedTags ?? []) {
    if (listed.has(tag.id) || !initial.has(tag.id)) continue;
    listed.add(tag.id);
    if (tag.status === 'active') {
      choices.push({ tag, archived: false, retained: false, blocked: false });
    } else {
      const retained = retainable.has(tag.id);
      choices.push({ tag, archived: true, retained, blocked: !retained });
    }
  }
  return choices;
}

/**
 * The tag IDs a save sends: everything selected except an archived tag that
 * would be a fresh assignment, which the server refuses. Only an assigned tag
 * can be archived (the active list offers no other), so until the assigned
 * tags are resolved the selection is sent as is.
 */
export function submittableTagIDs(
  selected: readonly number[],
  assignedTags: readonly TagResponse[] | undefined,
  retainable: ReadonlySet<number>
): number[] {
  if (!assignedTags) return [...selected];
  const archived = new Set(assignedTags.filter((tag) => tag.status !== 'active').map((tag) => tag.id));
  return selected.filter((id) => !archived.has(id) || retainable.has(id));
}

/** Every tag the form shows or carries: transaction tags and posting-line tags. */
export function assignedTagIDs(transactionTagIDs: readonly number[], postingTagIDs: readonly (readonly number[] | undefined)[]): number[] {
  const ids = new Set(transactionTagIDs);
  for (const tags of postingTagIDs) for (const id of tags ?? []) ids.add(id);
  return [...ids].sort((a, b) => a - b);
}
