import { describe, expect, it } from 'vitest';
import { APIClientError } from '#lib/api/client.ts';
import { RequestKey, newRequestKey, saveOutcomeUnknown } from '#lib/api/request-key.ts';

const lost = () => new APIClientError({ status: 0 });

describe('request key (#660)', () => {
  it('is 32 lowercase hex characters, new each time', () => {
    const key = newRequestKey();
    expect(key).toMatch(/^[0-9a-f]{32}$/);
    expect(newRequestKey()).not.toBe(key);
  });

  it('knows which failures leave the outcome unknown', () => {
    expect(saveOutcomeUnknown(lost())).toBe(true);
    expect(saveOutcomeUnknown(new APIClientError({ status: 502 }))).toBe(true);
    expect(saveOutcomeUnknown(new TypeError('fetch failed'))).toBe(true);
    expect(saveOutcomeUnknown(new APIClientError({ status: 409, code: 'CONFLICT' }))).toBe(false);
  });

  /** A save whose first response is lost; the server answers the repeat. */
  function lostOnce(label: string, sent: Array<[string, string]>) {
    let calls = 0;
    return async (key: string) => {
      sent.push([label, key]);
      calls += 1;
      if (calls === 1) throw lost();
    };
  }

  it('sends the same key with every retry of one entry until it is saved, then a new one', async () => {
    const requestKey = new RequestKey();
    const sent: Array<[string, string]> = [];
    await expect(requestKey.send({ n: 1 }, lostOnce('first', sent))).rejects.toBeInstanceOf(APIClientError);
    await requestKey.send({ n: 1 }, lostOnce('rebuilt', sent));
    // Saved: the next entry, even with identical content, gets a new key.
    await requestKey.send({ n: 1 }, async (key) => { sent.push(['next', key]); });
    expect(sent[1][1]).toBe(sent[0][1]);
    expect(sent[2][1]).not.toBe(sent[0][1]);
  });

  it('resends the exact earlier save while the entry is unchanged', async () => {
    // The owner confirmed a checkpoint override; the save committed and its
    // response was lost. Save again rebuilds the body without the override.
    const requestKey = new RequestKey();
    const sent: Array<[string, string]> = [];
    await expect(requestKey.send({ n: 1 }, lostOnce('with override', sent))).rejects.toBeInstanceOf(APIClientError);
    expect(requestKey.isUnresolved({ n: 1 })).toBe(true);
    await requestKey.send({ n: 1 }, lostOnce('rebuilt', sent));
    expect(sent.map(([what]) => what)).toEqual(['with override', 'with override']);
    expect(requestKey.isUnresolved({ n: 1 })).toBe(false);
  });

  it('sends the new save once the owner changed the entry, under the same key', async () => {
    const requestKey = new RequestKey();
    const sent: Array<[string, string]> = [];
    await expect(requestKey.send({ n: 1 }, lostOnce('first', sent))).rejects.toBeInstanceOf(APIClientError);
    expect(requestKey.isUnresolved({ n: 2 })).toBe(false);
    await expect(requestKey.send({ n: 2 }, async (key) => {
      sent.push(['changed', key]);
      throw new APIClientError({ status: 409, code: 'IDEMPOTENCY_KEY_REUSED' });
    })).rejects.toBeInstanceOf(APIClientError);
    // Reused is a refusal: the key stays, so no later save can record the
    // entry a second time, and restoring the entry replays the first save.
    await requestKey.send({ n: 1 }, async (key) => { sent.push(['rebuilt', key]); });
    expect(sent.map(([what]) => what)).toEqual(['first', 'changed', 'first']);
    expect(new Set(sent.map(([, key]) => key)).size).toBe(1);
  });

  // Review of PR #669, round 2: the confirmed save committed and lost its
  // response, the owner changed the amount, and that edited save failed with
  // a 503. Restoring the amount must still replay the confirmed save.
  it('keeps the original unresolved save when an edited save also fails without an answer', async () => {
    const requestKey = new RequestKey();
    const sent: Array<[string, string]> = [];
    await expect(requestKey.send({ amount: '4.50' }, lostOnce('confirmed', sent))).rejects.toBeInstanceOf(APIClientError);
    await expect(requestKey.send({ amount: '5.00' }, async (key) => {
      sent.push(['edited', key]);
      throw new APIClientError({ status: 503, code: 'RESOURCE_BUSY' });
    })).rejects.toBeInstanceOf(APIClientError);
    expect(requestKey.isUnresolved({ amount: '4.50' })).toBe(true);
    expect(requestKey.isUnresolved({ amount: '5.00' })).toBe(true);
    await requestKey.send({ amount: '4.50' }, async (key) => { sent.push(['rebuilt', key]); });
    expect(sent.map(([what]) => what)).toEqual(['confirmed', 'edited', 'confirmed']);
    expect(new Set(sent.map(([, key]) => key)).size).toBe(1);
    // Saved: every unresolved save is settled, and the next entry is new.
    expect(requestKey.isUnresolved({ amount: '5.00' })).toBe(false);
  });

  // Review of PR #669, round 3: the write committed, but the server had not
  // stored its result yet when the retry arrived. Dropping the save then
  // rebuilt the body without the confirmed override, refused as REUSED.
  it('keeps the exact save while its committed result is not available yet, then replays it', async () => {
    const requestKey = new RequestKey();
    const sent: Array<[string, string]> = [];
    let calls = 0;
    const confirmed = async (key: string) => {
      sent.push(['confirmed', key]);
      calls += 1;
      if (calls === 1) throw lost();
      if (calls === 2) throw new APIClientError({ status: 409, code: 'IDEMPOTENCY_KEY_RESULT_UNAVAILABLE' });
      return 'replayed';
    };
    await expect(requestKey.send({ n: 1 }, confirmed)).rejects.toBeInstanceOf(APIClientError);
    await expect(requestKey.send({ n: 1 }, async (key) => { sent.push(['rebuilt', key]); })).rejects.toMatchObject({ code: 'IDEMPOTENCY_KEY_RESULT_UNAVAILABLE' });
    expect(requestKey.isUnresolved({ n: 1 })).toBe(true);
    await expect(requestKey.send({ n: 1 }, async (key) => { sent.push(['rebuilt', key]); })).resolves.toBe('replayed');
    expect(sent.map(([what]) => what)).toEqual(['confirmed', 'confirmed', 'confirmed']);
    expect(new Set(sent.map(([, key]) => key)).size).toBe(1);
    expect(requestKey.isUnresolved({ n: 1 })).toBe(false);
  });

  it('settles only the refused save, keeping the others', async () => {
    const requestKey = new RequestKey();
    await expect(requestKey.send({ n: 1 }, async () => { throw lost(); })).rejects.toBeInstanceOf(APIClientError);
    await expect(requestKey.send({ n: 2 }, async () => {
      throw new APIClientError({ status: 409, code: 'IDEMPOTENCY_KEY_REUSED' });
    })).rejects.toBeInstanceOf(APIClientError);
    expect(requestKey.isUnresolved({ n: 1 })).toBe(true);
    expect(requestKey.isUnresolved({ n: 2 })).toBe(false);
  });

  it('forgets the earlier save once it is refused, which wrote nothing', async () => {
    const requestKey = new RequestKey();
    let calls = 0;
    const save = async () => {
      calls += 1;
      throw calls === 1 ? lost() : new APIClientError({ status: 400, code: 'VALIDATION_FAILED' });
    };
    await expect(requestKey.send({ n: 1 }, save)).rejects.toBeInstanceOf(APIClientError);
    await expect(requestKey.send({ n: 1 }, async () => { throw new Error('not sent'); })).rejects.toBeInstanceOf(APIClientError);
    expect(calls).toBe(2);
    expect(requestKey.isUnresolved({ n: 1 })).toBe(false);
  });
});
