import { describe, expect, it } from 'vitest';
import { APIClientError } from '#lib/api/client.ts';
import { RequestKey, newRequestKey } from '#lib/api/request-key.ts';

describe('request key (#660)', () => {
  it('is 32 lowercase hex characters, new each time', () => {
    const key = newRequestKey();
    expect(key).toMatch(/^[0-9a-f]{32}$/);
    expect(newRequestKey()).not.toBe(key);
  });

  it('sends the same key with every retry of one entry until it is saved', async () => {
    const requestKey = new RequestKey();
    const sent: string[] = [];
    const lost = async (key: string) => {
      sent.push(key);
      throw new APIClientError({ status: 0 });
    };
    await expect(requestKey.send(lost)).rejects.toBeInstanceOf(APIClientError);
    await expect(requestKey.send(lost)).rejects.toBeInstanceOf(APIClientError);
    await requestKey.send(async (key) => sent.push(key));
    expect(new Set(sent).size).toBe(1);

    // Saved: the next entry, even with identical content, gets a new key.
    await requestKey.send(async (key) => sent.push(key));
    expect(sent[3]).not.toBe(sent[0]);
  });

  it('keeps the key after a refusal, which stored nothing', async () => {
    const requestKey = new RequestKey();
    const before = requestKey.value;
    await expect(requestKey.send(async () => {
      throw new APIClientError({ status: 400, code: 'VALIDATION_FAILED' });
    })).rejects.toBeInstanceOf(APIClientError);
    expect(requestKey.value).toBe(before);
  });

  it('starts a new key once told an earlier save of the entry was recorded with other content', async () => {
    const requestKey = new RequestKey();
    const before = requestKey.value;
    await expect(requestKey.send(async () => {
      throw new APIClientError({ status: 409, code: 'IDEMPOTENCY_KEY_REUSED' });
    })).rejects.toBeInstanceOf(APIClientError);
    expect(requestKey.value).not.toBe(before);
  });
});
