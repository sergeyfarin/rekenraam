import type { Snippet } from 'svelte';

export type Column<R> = {
  key: string;
  header: string;
  width?: string;
  align?: 'left' | 'right';
  /**
   * Priority controls responsive column hiding:
   * 1 = always shown
   * 2 = hidden below ~600px
   * 3 = hidden below ~900px
   */
  priority?: 1 | 2 | 3;
  cell: Snippet<[R]>;
};
