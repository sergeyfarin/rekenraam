import { describe, expect, it } from 'vitest';
import { adjacentCalendarDate, formatCalendarDate, isCalendarDate } from './calendar-date';

describe('isCalendarDate', () => {
  it.each(['2026-02-31', '2026-02-29', '2026-04-31', '2026-13-01', '2026-00-01'])(
    'rejects impossible calendar dates: %s',
    (date) => expect(isCalendarDate(date)).toBe(false)
  );

  it.each(['2026-02-28', '2024-02-29'])(
    'accepts ordinary and leap calendar dates: %s',
    (date) => expect(isCalendarDate(date)).toBe(true)
  );
});

describe('adjacentCalendarDate', () => {
  it('steps one calendar day across month, year and leap edges', () => {
    expect(adjacentCalendarDate('2026-06-01', 1)).toBe('2026-06-02');
    expect(adjacentCalendarDate('2026-03-01', -1)).toBe('2026-02-28');
    expect(adjacentCalendarDate('2028-03-01', -1)).toBe('2028-02-29');
    expect(adjacentCalendarDate('2026-12-31', 1)).toBe('2027-01-01');
    expect(adjacentCalendarDate('2027-01-01', -1)).toBe('2026-12-31');
    expect(adjacentCalendarDate('2026-04-30', 1)).toBe('2026-05-01');
  });
});

describe('formatCalendarDate', () => {
  it('formats in the reader locale without moving the day across time zones', () => {
    expect(formatCalendarDate('2026-09-01', 'en-US')).toBe('Sep 1, 2026');
    expect(formatCalendarDate('2026-09-01', 'de')).toBe('01.09.2026');
    expect(formatCalendarDate('2026-01-01', 'en-US')).toBe('Jan 1, 2026');
  });

  it('shows anything that is not a calendar date as received', () => {
    expect(formatCalendarDate('2026-02-31', 'en-US')).toBe('2026-02-31');
    expect(formatCalendarDate('soon', 'en-US')).toBe('soon');
  });
});
