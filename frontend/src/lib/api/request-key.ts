import { APIClientError } from '#lib/api/client.ts';

/**
 * A create's request key (#660). One form entry keeps one key for every
 * retry, so a save whose response was lost and is clicked again returns the
 * first result instead of recording the entry twice. The key is replaced once
 * the entry is saved, so a deliberate second entry with the same content is a
 * second record. A refused save stored nothing, so its key stays and a
 * corrected save may reuse it.
 */
export class RequestKey {
  #value = newRequestKey();

  get value(): string {
    return this.#value;
  }

  /** Starts a new entry: the next save is a new record. */
  rotate(): void {
    this.#value = newRequestKey();
  }

  /**
   * Sends one save with this entry's key. A save that succeeds starts a new
   * entry. So does IDEMPOTENCY_KEY_REUSED: an earlier save of this entry was
   * recorded with other content, the owner is told so, and saving again
   * records the changed content as a separate entry.
   */
  async send<T>(save: (requestKey: string) => Promise<T>): Promise<T> {
    try {
      const result = await save(this.#value);
      this.rotate();
      return result;
    } catch (error) {
      if (isRequestKeyReused(error)) this.rotate();
      throw error;
    }
  }
}

export function isRequestKeyReused(error: unknown): boolean {
  return error instanceof APIClientError && error.code === 'IDEMPOTENCY_KEY_REUSED';
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
