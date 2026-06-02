import { m } from '$lib/paraglide/messages.js';
import type { InstallGateState } from './install-gate-state';

export type InstallGateStateCopy = {
  badge: string;
  title: string;
  copy: string;
};

export function installGateStateCopy(state: InstallGateState): InstallGateStateCopy {
  switch (state) {
    case 'fresh':
      return {
        badge: m.install_gate_state_fresh(),
        title: m.install_gate_fresh_title(),
        copy: m.install_gate_fresh_copy()
      };
    case 'login':
      return {
        badge: m.install_gate_state_configured(),
        title: m.install_gate_login_title(),
        copy: m.install_gate_login_copy()
      };
    case 'book_setup':
      return {
        badge: m.install_gate_state_preparing(),
        title: m.install_gate_preparing_title(),
        copy: m.install_gate_preparing_copy()
      };
    case 'currency_setup':
      return {
        badge: m.install_gate_state_currencies(),
        title: m.install_gate_currencies_title(),
        copy: m.install_gate_currencies_copy()
      };
    case 'authenticated':
      return {
        badge: m.install_gate_state_authenticated(),
        title: m.install_gate_authenticated_title(),
        copy: m.install_gate_authenticated_copy()
      };
    case 'recovery_required':
      return {
        badge: m.install_gate_state_recovery(),
        title: m.install_gate_recovery_title(),
        copy: m.install_gate_recovery_copy()
      };
    case 'error':
      return {
        badge: m.install_gate_error_badge(),
        title: m.install_gate_error_title(),
        copy: m.install_gate_error_copy()
      };
    default:
      return {
        badge: m.install_gate_loading_badge(),
        title: m.install_gate_loading_title(),
        copy: m.install_gate_loading_copy()
      };
  }
}
