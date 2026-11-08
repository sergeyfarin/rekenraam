import { expect, test } from './support/fixtures';
import { apiJSON, csrfTokenFor } from './support/api';
import { ensureBrowserSession } from './support/session';
import { activateBrowserMFA } from './support/mfa-enrollment';
import { currentStep, secretFromOTPAuthURI } from './support/totp';

const activationPath = '/api/v1/auth/mfa/totp/activate';

// Only fixture samples are stale. Every submission reaches the ordinary app's
// real enrollment consumer, with its production clock and replay/skew rules.
test.describe('MFA enrollment fixture controls', () => {
  test.beforeEach(async ({ page }) => {
    await ensureBrowserSession(page);
    await page.goto('/app/settings/security');
    await page.getByLabel('Your password', { exact: true }).fill('test-password');
    await page.getByRole('button', { name: 'Set up', exact: true }).click();
    await expect(page.getByText(/^otpauth:\/\//)).toBeVisible();
  });

  test.afterEach(async ({ page }) => {
    // Keep this original owner session even when activation is refused.
    const status = await apiJSON<{ status: string }>(page, 'GET', '/api/v1/auth/mfa');
    if (status.status !== 'disabled') {
      await apiJSON(page, 'POST', '/api/v1/auth/mfa/disable', await csrfTokenFor(page),
        { password: 'test-password' }, [200, 204]);
    }
  });

  for (const mode of ['current', 'previous', 'boundary', 'no advancement', 'attempt limit', 'CSRF refusal', 'unrelated 401'] as const) {
    test(`${mode} through the real enrollment consumer`, async ({ page }) => {
      const secret = secretFromOTPAuthURI(await page.getByText(/^otpauth:\/\//).innerText());
      const controls = {
        code: page.getByLabel('Code from your app', { exact: true }),
        submit: page.getByRole('button', { name: 'Turn on', exact: true })
      };
      const statuses: number[] = [];
      page.on('response', response => {
        if (new URL(response.url()).pathname === activationPath) statuses.push(response.status());
      });
      let samples = 0;
      let sampledStep = 0;
      const staleStep = currentStep() - 100;
      const stepNow = () => {
        samples++;
        sampledStep = mode === 'attempt limit' ? staleStep + samples
          : mode === 'no advancement' ? staleStep
          : ['boundary', 'CSRF refusal', 'unrelated 401'].includes(mode) && samples === 1 ? currentStep() - 1
          : currentStep();
        return sampledStep;
      };

      if (mode === 'CSRF refusal') {
        await page.route(`**${activationPath}`, route => route.continue({
          headers: { ...route.request().headers(), 'x-csrf-token': 'invalid-fixture-control' }
        }));
      } else if (mode === 'unrelated 401') {
        await page.route(`**${activationPath}`, async route => {
          // The consumer really refuses an invalid code. Change only its
          // envelope to prove that status 401 alone cannot authorize a refresh.
          const response = await route.fetch({
            postData: { code: 'invalid-fixture-control' }
          });
          expect(response.status()).toBe(401);
          await route.fulfill({ response, json: {
            error: { code: 'UNAUTHENTICATED', message: 'unrelated fixture refusal' }
          } });
        });
      }

      const activation = activateBrowserMFA(page, secret, controls, {
        reserveCurrentStep: mode !== 'current', stepNow
      });
      if (mode === 'no advancement') {
        await expect(activation).rejects.toThrow('observed step advancement');
        expect(statuses).toEqual([401]);
        expect(samples).toBe(2);
      } else if (mode === 'attempt limit') {
        await expect(activation).rejects.toThrow('fixture attempt limit');
        expect(statuses).toEqual([401, 401, 401]);
        expect(samples).toBe(3);
      } else if (mode === 'CSRF refusal' || mode === 'unrelated 401') {
        await expect(activation).rejects.toThrow();
        expect(statuses).toEqual([mode === 'CSRF refusal' ? 403 : 401]);
        expect(samples).toBe(1);
      } else {
        const spentStep = await activation;
        expect(spentStep).toBe(sampledStep - (mode === 'current' ? 0 : 1));
        // A previous-step code is refused if the real clock crosses a 30 s
        // step between sampling and the server's check; the helper proves the
        // advancement before resampling. Allow that one natural refusal, so
        // only the injected boundary is mandatory (#384 follow-up).
        const injected = mode === 'boundary' ? 1 : 0;
        const natural = samples - 1 - injected;
        expect(natural).toBeGreaterThanOrEqual(0);
        expect(natural).toBeLessThanOrEqual(mode === 'current' ? 0 : 1);
        expect(statuses).toEqual([...Array<number>(samples - 1).fill(401), 200]);
        await expect(page.getByRole('heading', { name: 'Recovery codes', exact: true })).toBeVisible();
        await expect(page.locator('ul.font-mono li')).toHaveCount(10);
      }
      const status = await apiJSON<{ status: string; recovery_codes_remaining: number }>(page, 'GET', '/api/v1/auth/mfa');
      const activated = ['current', 'previous', 'boundary'].includes(mode);
      expect(status.status).toBe(activated ? 'active' : 'pending');
      expect(status.recovery_codes_remaining).toBe(activated ? 10 : 0);
    });
  }
});
