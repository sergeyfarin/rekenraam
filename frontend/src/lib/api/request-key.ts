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
 * confirmed, or with a payee the lost save created. So the key keeps each
 * save whose outcome is unknown (no answer, or a server failure), identified
 * by the owner's entry rather than the body, and while the entry matches one
 * of them it sends that save again instead of the rebuilt one. Under one key
 * at most one of them committed, and none is forgotten until its own outcome
 * is known or the entry is saved: an edited save that also fails leaves the
 * original confirmed save to replay when the owner restores the entry. A save
 * the server reports as committed without a stored result yet
 * (IDEMPOTENCY_KEY_RESULT_UNAVAILABLE) is kept too: its result is stored
 * moments later, and only the same bytes under the same key can replay it.
 *
 * Inside a New transaction entry the key lives with the activity's working
 * copy (`useRequestKey`), so closing and reopening an activity keeps it.
 */
export class RequestKey {
  #value = newRequestKey();
  readonly #unresolved = new Map<string, (requestKey: string) => Promise<unknown>>();

  get value(): string {
    return this.#value;
  }

  /** Whether a save of exactly this entry has an unknown outcome. */
  isUnresolved(entry: unknown): boolean {
    return this.#unresolved.has(entryIdentity(entry));
  }

  /**
   * Saves the entry with this key: an earlier save of the same entry whose
   * outcome is unknown is sent again as it was, otherwise `save`. Success
   * starts a new entry. A refusal with a code settles only this entry's save:
   * it wrote nothing, and the other unresolved saves keep theirs. A committed
   * save whose result is not available yet stays unresolved. The key is
   * kept, including on IDEMPOTENCY_KEY_REUSED: the owner changed an entry
   * whose earlier save was recorded, and is told so rather than handed a key
   * that would record it twice.
   */
  async send<T>(entry: unknown, save: (requestKey: string) => Promise<T>): Promise<T> {
    const identity = entryIdentity(entry);
    const attempt = (this.#unresolved.get(identity) as ((requestKey: string) => Promise<T>) | undefined) ?? save;
    try {
      const result = await attempt(this.#value);
      this.#unresolved.clear();
      this.#value = newRequestKey();
      return result;
    } catch (error) {
      if (saveOutcomeUnknown(error) || resultNotYetStored(error)) this.#unresolved.set(identity, attempt);
      else this.#unresolved.delete(identity);
      throw error;
    }
  }
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

/**
 * The key's write committed but its response is not stored yet: the request
 * that committed it is still finishing, or storing failed. Sending the same
 * bytes again replays the result once it is stored; any other body is refused
 * as IDEMPOTENCY_KEY_REUSED.
 */
function resultNotYetStored(error: unknown): boolean {
  return error instanceof APIClientError && error.code === 'IDEMPOTENCY_KEY_RESULT_UNAVAILABLE';
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
