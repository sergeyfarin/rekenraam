import process from 'node:process';
import type { TraceMode } from '@playwright/test';

type TraceOptions = {
  mode: TraceMode;
  screenshots: boolean;
  snapshots: boolean;
  sources: boolean;
};

// Playwright's --trace=<mode> CLI override replaces only `mode`; the other
// flags in an object trace configuration survive. --trace=on therefore keeps
// the light trace's missing DOM snapshots and network records (#473).
// E2E_FULL_TRACE=1 selects a genuinely full local trace instead; CI and the
// normal wrappers leave it unset.
const fullTraceEnabled = process.env.E2E_FULL_TRACE === '1';

const fullTrace: TraceOptions = {
  mode: 'on',
  screenshots: true,
  snapshots: true,
  sources: true
};

/** CI and ordinary local runs: actionability logs and stacks only on failure. */
export const defaultTrace: TraceOptions = fullTraceEnabled
  ? fullTrace
  : { mode: 'retain-on-failure', screenshots: false, snapshots: false, sources: false };

/** #414's scoped spec keeps DOM/network evidence unless a full trace is requested. */
export const scopedDomTrace: TraceOptions = fullTraceEnabled
  ? fullTrace
  : { mode: 'retain-on-failure', screenshots: false, snapshots: true, sources: false };
