import type { AuthSessionResponse } from '$lib/api/auth';
import type { SetupStatusResponse } from '$lib/api/setup';

export type InstallGateState =
  | 'loading'
  | 'error'
  | 'fresh'
  | 'login'
  | 'book_setup'
  | 'currency_setup'
  | 'authenticated'
  | 'recovery_required';

export type InstallGateSnapshot = {
  setupPending: boolean;
  setupError: boolean;
  setup?: SetupStatusResponse;
  sessionPending: boolean;
  sessionError: boolean;
  session?: AuthSessionResponse;
};

export type InstallGateTransition = {
  state: InstallGateState;
  reason: string;
  matches: (snapshot: InstallGateSnapshot) => boolean;
};

export const installGateTransitions: readonly InstallGateTransition[] = [
  {
    state: 'loading',
    reason: 'Setup or session data is still loading.',
    matches: (snapshot) => snapshot.setupPending || snapshot.sessionPending
  },
  {
    state: 'error',
    reason: 'Setup or session discovery failed.',
    matches: (snapshot) => snapshot.setupError || snapshot.sessionError
  },
  {
    state: 'loading',
    reason: 'A query completed without usable data yet.',
    matches: (snapshot) => snapshot.setup === undefined || snapshot.session === undefined
  },
  {
    state: 'fresh',
    reason: 'No owner exists yet.',
    matches: (snapshot) => snapshot.setup?.install_state === 'fresh'
  },
  {
    state: 'recovery_required',
    reason: 'Owner recovery is required before normal login.',
    matches: (snapshot) => snapshot.setup?.install_state === 'recovery_required'
  },
  {
    state: 'login',
    reason: 'The installation exists, but the browser has no authenticated session.',
    matches: (snapshot) => snapshot.session?.authenticated !== true
  },
  {
    state: 'book_setup',
    reason: 'The authenticated setup flow still needs the implicit personal book.',
    matches: (snapshot) => snapshot.setup?.current_step === 'book'
  },
  {
    state: 'currency_setup',
    reason: 'The authenticated setup flow still needs currencies.',
    matches: (snapshot) => snapshot.setup?.current_step === 'currencies'
  },
  {
    state: 'authenticated',
    reason: 'Setup is complete and the session is authenticated.',
    matches: () => true
  }
];

export function resolveInstallGateState(snapshot: InstallGateSnapshot): InstallGateState {
  return installGateTransitions.find((transition) => transition.matches(snapshot))?.state ?? 'error';
}

export function shouldRedirectToApp(browser: boolean, state: InstallGateState): boolean {
  return browser && state === 'authenticated';
}

export function shouldCreateDefaultBook({
  browser,
  state,
  pending,
  error
}: {
  browser: boolean;
  state: InstallGateState;
  pending: boolean;
  error: unknown;
}): boolean {
  return browser && state === 'book_setup' && !pending && !error;
}
