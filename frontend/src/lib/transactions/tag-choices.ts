import type { TagResponse } from '#lib/api/tags.ts';

/**
 * One tag the transaction form shows (#650). Archiving a tag stops new
 * assignments but keeps the ones it already has, so the form offers every
 * active tag, and an archived tag only where it is already assigned:
 *
 * - `retained`: the saved transaction carries it. The owner can keep it or
 *   clear it, and the server keeps the assignment while it is restated.
 * - `blocked`: the form was filled with it, but saving would be a fresh
 *   assignment (a new transaction, a correction or a template). It is shown
 *   so nothing disappears unannounced, and it is never sent.
 */
export interface TagChoice {
  tag: TagResponse;
  archived: boolean;
  retained: boolean;
  blocked: boolean;
}

/**
 * The tags the form lists, in the server's order: active tags, then the
 * archived tags the form was filled with.
 *
 * @param tags every tag, archived included
 * @param initialTagIDs the tags the form was filled with
 * @param retainable the tags the saved record already carries; empty when
 *   saving creates a new record
 */
export function tagChoices(
  tags: readonly TagResponse[],
  initialTagIDs: readonly number[],
  retainable: ReadonlySet<number>
): TagChoice[] {
  const initial = new Set(initialTagIDs);
  const choices: TagChoice[] = [];
  for (const tag of tags) {
    if (tag.status === 'active') {
      choices.push({ tag, archived: false, retained: false, blocked: false });
    } else if (initial.has(tag.id)) {
      const retained = retainable.has(tag.id);
      choices.push({ tag, archived: true, retained, blocked: !retained });
    }
  }
  return choices;
}

/**
 * The tag IDs a save sends: everything selected except an archived tag that
 * would be a fresh assignment, which the server refuses. Until the tag list
 * has loaded nothing is known to be archived, so the selection is sent as is.
 */
export function submittableTagIDs(
  selected: readonly number[],
  tags: readonly TagResponse[] | undefined,
  retainable: ReadonlySet<number>
): number[] {
  if (!tags) return [...selected];
  const archived = new Set(tags.filter((tag) => tag.status !== 'active').map((tag) => tag.id));
  return selected.filter((id) => !archived.has(id) || retainable.has(id));
}
