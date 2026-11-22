import type { PayeeDirectorySelection } from '#lib/payees/payee-directory-navigation.ts';

declare global {
  namespace App {
    interface PageState {
      payeeDirectory?: PayeeDirectorySelection;
    }
  }
}

export {};
