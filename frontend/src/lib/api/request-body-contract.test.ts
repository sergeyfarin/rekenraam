import { describe, expectTypeOf, it } from 'vitest';
import type { components, paths } from './schema.js';

// These fail type checking if generation drops the actual 413 consumer branch.
describe('generated capped request status contract', () => {
  it('includes public, authenticated, optional and backup-policy JSON refusals', () => {
    type Refusal = components['responses']['JSONBodyTooLarge'];
    expectTypeOf<paths['/api/v1/setup/owner']['post']['responses'][413]>().toEqualTypeOf<Refusal>();
    expectTypeOf<paths['/api/v1/accounts']['post']['responses'][413]>().toEqualTypeOf<Refusal>();
    expectTypeOf<paths['/api/v1/accounts/{account_id}/archive']['post']['responses'][413]>().toEqualTypeOf<Refusal>();
    expectTypeOf<paths['/api/v1/maintenance/backups/policy']['put']['responses'][413]>().toEqualTypeOf<Refusal>();
  });

  it('retains the shared error envelope for both multipart upload operations', () => {
    type Envelope = components['schemas']['ErrorResponse'];
    expectTypeOf<paths['/api/v1/imports']['post']['responses'][413]['content']['application/json']>().toEqualTypeOf<Envelope>();
    expectTypeOf<paths['/api/v1/imports/analyze']['post']['responses'][413]['content']['application/json']>().toEqualTypeOf<Envelope>();
  });
});
