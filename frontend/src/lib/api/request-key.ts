import { APIClientError } from '#lib/api/client.ts';

/**
 * A create's request key (#660). One entry keeps one key for every retry, so
 * a save whose response was lost and is clicked again returns the first
 * result instead of recording the entry twice. The key is replaced only once
 * the entry is saved, so a deliberate second entry with the same content is a
 * second record.
 *
 * A retry must also repeat the request byte for byte, or the server refuses
 * it as a different one. A form rebuilds its body on every Save and may build
 * it differently the second time: without the checkpoint override the owner
 * confirmed, or with a payee the lost save created. So the key keeps the save
 * whose outcome is unknown (no answer, or a server failure), identified by
 * the owner's entry rather than the body, and while that entry is unchanged
 * it sends that save again instead of the rebuilt one.
 *
 * Inside a New transaction entry the key lives with the activity's working
 * copy (`useRequestKey`), so closing and reopening an activity keeps it.
 */
export class RequestKey {
  #value = newRequestKey();
  #unresolved: { entry: string; save: (requestKey: string) => Promise<unknown> } | null = null;

  get value(): string {
    return this.#value;
  }

  /** Whether the last save of this unchanged entry has an unknown outcome. */
  isUnresolved(entry: unknown): boolean {
    return this.#unresolved?.entry === entryIdentity(entry);
  }

  /**
   * Saves the entry with this key: the earlier save again when the entry is
   * unchanged since a save with an unknown outcome, otherwise `save`. Success
   * starts a new entry. A refusal with a code stored nothing, so the key is
   * kept for the corrected save, including IDEMPOTENCY_KEY_REUSED: the owner
   * changed an entry whose earlier save was recorded, and is told so rather
   * than handed a key that would record it twice.
   */
  async send<T>(entry: unknown, save: (requestKey: string) => Promise<T>): Promise<T> {
    const identity = entryIdentity(entry);
    const attempt = this.#unresolved?.entry === identity
      ? (this.#unresolved.save as (requestKey: string) => Promise<T>)
      : save;
    try {
      const result = await attempt(this.#value);
      this.#unresolved = null;
      this.#value = newRequestKey();
      return result;
    } catch (error) {
      if (saveOutcomeUnknown(error)) this.#unresolved = { entry: identity, save: attempt };
      // Reused: the earlier save committed under this key and the changed
      // entry is refused. Keep that save, so restoring the entry replays it.
      else if (!isRequestKeyReused(error)) this.#unresolved = null;
      throw error;
    }
  }
}

export function isRequestKeyReused(error: unknown): boolean {
  return error instanceof APIClientError && error.code === 'IDEMPOTENCY_KEY_REUSED';
}

/**
 * A save whose outcome is unknown: no answer arrived, or the server failed
 * after it may have committed. A refusal with a code is known: nothing was
 * written.
 */
export function saveOutcomeUnknown(error: unknown): boolean {
  if (!(error instanceof APIClientError)) return true;
  return error.status === 0 || error.status >= 500;
}

function entryIdentity(entry: unknown): string {
  return JSON.stringify(entry) ?? '';
}

/**
 * 128 random bits as hex. crypto.getRandomValues works on plain HTTP too;
 * crypto.randomUUID needs a secure context, and a self-hosted install may be
 * reached over HTTP on a home network.
 */
export function newRequestKey(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('');
}
