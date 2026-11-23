import { describe, expect, it } from 'vitest';
import { APIClientError } from '#lib/api/client.ts';
import type { GainImpact } from '#lib/api/investments.ts';
import { commitReview, type CommitImpact, type CommitReviewState } from './commit-review';
import type { ReviewGuards } from './preview-review';

type Subject = { n: number };

const checkpoint = (id: number) => ({
  checkpoint_id: id, account_id: 1, account_name: 'Checking', account_code: null, account_system_role: null,
  account_builtin_key: null, account_starter_key: null, account_currency_code: null, commodity_id: 2, commodity_code: 'EUR',
  statement_date: '2026-01-31', statement_account_sequence: 1, entry_date: '2026-01-31'
});

function gains(acknowledgement: string): GainImpact {
  return { acknowledgement, changes: [{} as GainImpact['changes'][number]] } as GainImpact;
}

const stale = () => new APIClientError({ status: 409, code: 'INVESTMENT_GAIN_IMPACT_ACKNOWLEDGEMENT_STALE' });

function harness(protocol: 'gains' | 'checkpoints', options: {
  impacts?: (() => Promise<CommitImpact>)[];
  writes?: (() => Promise<void>)[];
} = {}) {
  const impacts = [...(options.impacts ?? [])];
  const writes = [...(options.writes ?? [])];
  const state = {
    review: null as CommitReviewState<Subject> | null,
    pending: false,
    error: undefined as unknown,
    asked: [] as Subject[],
    written: [] as { subject: Subject; guards: ReviewGuards }[]
  };
  const controller = commitReview<Subject, CommitImpact>({
    protocol,
    get review() { return state.review; },
    async impact(subject) {
      state.asked.push(subject);
      const next = impacts.shift();
      if (!next) throw new Error('unexpected impact request');
      return next();
    },
    async write(subject, guards) {
      state.written.push({ subject, guards });
      const next = writes.shift();
      if (next) await next();
    },
    setReview(review) { state.review = review; },
    setPending(pending) { state.pending = pending; },
    setError(error) { state.error = error; }
  });
  return { state, controller };
}

describe('commitReview', () => {
  it('needs no review when the write invalidates nothing', async () => {
    const { state, controller } = harness('gains', { impacts: [async () => ({ affected_checkpoints: [], gain_impact: null })] });
    expect(await controller.open({ n: 1 }, false)).toBe(false);
    expect(state.review).toBeNull();
  });

  it('opens a fresh review and confirms exactly the reviewed set once', async () => {
    const { state, controller } = harness('gains', {
      impacts: [async () => ({ affected_checkpoints: [checkpoint(4)], gain_impact: gains('ack-1') })]
    });
    expect(await controller.open({ n: 1 }, false)).toBe(true);
    expect(state.review).toMatchObject({ gainRefreshed: false, subject: { n: 1 } });
    await controller.confirm();
    expect(state.review).toBeNull();
    expect(state.written).toEqual([{ subject: { n: 1 }, guards: {
      reconciliation_override: true, reconciliation_override_checkpoint_ids: [4], gain_impact_acknowledgement: 'ack-1'
    } }]);
    expect(state.pending).toBe(false);
    expect(state.error).toBeUndefined();
  });

  it('cancel closes the review without writing', async () => {
    const { state, controller } = harness('gains', { impacts: [async () => ({ affected_checkpoints: [checkpoint(4)] })] });
    await controller.open({ n: 1 }, false);
    controller.cancel();
    expect(state.review).toBeNull();
    expect(state.written).toEqual([]);
  });

  it('a stale refusal opens a refreshed review without retrying the write', async () => {
    const { state, controller } = harness('gains', {
      impacts: [
        async () => ({ affected_checkpoints: [], gain_impact: gains('ack-1') }),
        async () => ({ affected_checkpoints: [checkpoint(7)], gain_impact: gains('ack-2') })
      ],
      writes: [() => Promise.reject(stale())]
    });
    await controller.open({ n: 1 }, false);
    await controller.confirm();
    expect(state.written).toHaveLength(1);
    expect(state.asked).toEqual([{ n: 1 }, { n: 1 }]);
    expect(state.review).toMatchObject({ impacts: [checkpoint(7)], gainRefreshed: true });
    expect(state.error).toBeUndefined();

    await controller.confirm();
    expect(state.written[1].guards).toEqual({
      reconciliation_override: true, reconciliation_override_checkpoint_ids: [7], gain_impact_acknowledgement: 'ack-2'
    });
  });

  it('a stale refusal whose impact needs no review shows the refusal', async () => {
    const refusal = stale();
    const { state, controller } = harness('gains', {
      impacts: [
        async () => ({ affected_checkpoints: [checkpoint(4)] }),
        async () => ({ affected_checkpoints: [] })
      ],
      writes: [() => Promise.reject(refusal)]
    });
    await controller.open({ n: 1 }, false);
    await controller.confirm();
    expect(state.review).toBeNull();
    expect(state.error).toBe(refusal);
    expect(state.written).toHaveLength(1);
  });

  it('a failed impact refresh shows the refresh error', async () => {
    const failure = new Error('impact failed');
    const { state, controller } = harness('gains', {
      impacts: [async () => ({ affected_checkpoints: [checkpoint(4)] }), () => Promise.reject(failure)],
      writes: [() => Promise.reject(stale())]
    });
    await controller.open({ n: 1 }, false);
    await controller.confirm();
    expect(state.error).toBe(failure);
    expect(state.review).toBeNull();
    expect(state.pending).toBe(false);
  });

  it('any other refusal is shown without asking for the impact again', async () => {
    const refusal = new APIClientError({ status: 422, code: 'VALIDATION_FAILED' });
    const { state, controller } = harness('gains', {
      impacts: [async () => ({ affected_checkpoints: [checkpoint(4)] })],
      writes: [() => Promise.reject(refusal)]
    });
    await controller.open({ n: 1 }, false);
    await controller.confirm();
    expect(state.error).toBe(refusal);
    expect(state.asked).toHaveLength(1);
  });

  it('the checkpoint-only protocol never acquires a gain requirement', async () => {
    const { state, controller } = harness('checkpoints', {
      impacts: [
        async () => ({ affected_checkpoints: [], gain_impact: gains('ack-1') }),
        async () => ({ affected_checkpoints: [checkpoint(3)], gain_impact: gains('ack-1') })
      ]
    });
    expect(await controller.open({ n: 1 }, false)).toBe(false);
    expect(await controller.open({ n: 1 }, true)).toBe(true);
    expect(state.review).toMatchObject({ gainImpact: null, gainRefreshed: false });
    await controller.confirm();
    expect(state.written[0].guards).toEqual({ reconciliation_override: true, reconciliation_override_checkpoint_ids: [3] });
  });

  it('confirm without a review does nothing', async () => {
    const { state, controller } = harness('gains');
    await controller.confirm();
    expect(state.written).toEqual([]);
    expect(state.pending).toBe(false);
  });

  describe('a write whose outcome is unknown (#660)', () => {
    const none = async () => ({ affected_checkpoints: [], gain_impact: null });
    const lost = async () => { throw new APIClientError({ status: 0 }); };

    it('sends the unchanged write again without asking for its impact', async () => {
      // The first sale committed and its response was lost: its impact would
      // now be refused, because the units are gone.
      const { state, controller } = harness('gains', { impacts: [none], writes: [lost] });
      await expect(controller.submit({ n: 1 })).rejects.toBeInstanceOf(APIClientError);
      await controller.submit({ n: 1 });
      expect(state.asked).toEqual([{ n: 1 }]);
      expect(state.written).toEqual([{ subject: { n: 1 }, guards: {} }, { subject: { n: 1 }, guards: {} }]);
    });

    it('resends the guards the confirmed review gave', async () => {
      const { state, controller } = harness('gains', {
        impacts: [async () => ({ affected_checkpoints: [checkpoint(7)], gain_impact: gains('ack-1') })],
        writes: [async () => { throw new APIClientError({ status: 502 }); }]
      });
      expect(await controller.open({ n: 1 }, false)).toBe(true);
      await controller.confirm();
      await controller.submit({ n: 1 });
      expect(state.asked).toHaveLength(1);
      expect(state.written[1]).toEqual(state.written[0]);
      expect(state.written[1].guards).not.toEqual({});
    });

    it('asks for the impact again once the owner changed the entry', async () => {
      const { state, controller } = harness('gains', { impacts: [none, none], writes: [lost] });
      await expect(controller.submit({ n: 1 })).rejects.toBeInstanceOf(APIClientError);
      await controller.submit({ n: 2 });
      expect(state.asked).toEqual([{ n: 1 }, { n: 2 }]);
    });

    it('asks for the impact again after a refusal, which wrote nothing', async () => {
      const { state, controller } = harness('gains', {
        impacts: [none, none],
        writes: [async () => { throw new APIClientError({ status: 409, code: 'CONFLICT' }); }]
      });
      await expect(controller.submit({ n: 1 })).rejects.toBeInstanceOf(APIClientError);
      await controller.submit({ n: 1 });
      expect(state.asked).toEqual([{ n: 1 }, { n: 1 }]);
    });
  });
});
