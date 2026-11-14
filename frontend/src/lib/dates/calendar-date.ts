const ISO_DATE = /^(\d{4})-(\d{2})-(\d{2})$/;

/**
 * A real calendar date, not merely a date-shaped string.
 *
 * `2026-99-99` and `2026-02-31` both satisfy the shape, and sending either
 * produces a permanent validation error the screen cannot recover from — the
 * repair exists precisely so a malformed link never reaches the API. The check
 * is arithmetic rather than `new Date(...)`, whose parsing rolls February 31st
 * forward to March 3rd instead of rejecting it.
 */
export function isCalendarDate(value: string): boolean {
  const match = ISO_DATE.exec(value);
  if (!match) return false;
  const [, year, month, day] = match;
  const monthNumber = Number(month);
  const dayNumber = Number(day);
  if (monthNumber < 1 || monthNumber > 12 || dayNumber < 1) return false;
  return dayNumber <= daysInMonth(Number(year), monthNumber);
}

/**
 * The calendar date one day before or after an ISO date, by arithmetic for the
 * same reason as above. Used to name the day a posting moves to (#187); the
 * server computes the move itself.
 */
export function adjacentCalendarDate(value: string, step: 1 | -1): string {
  const match = ISO_DATE.exec(value);
  if (!match) throw new Error(`not a calendar date: ${value}`);
  let year = Number(match[1]);
  let month = Number(match[2]);
  let day = Number(match[3]) + step;
  if (day < 1) {
    month -= 1;
    if (month < 1) {
      month = 12;
      year -= 1;
    }
    day = daysInMonth(year, month);
  } else if (day > daysInMonth(year, month)) {
    day = 1;
    month += 1;
    if (month > 12) {
      month = 1;
      year += 1;
    }
  }
  return `${String(year).padStart(4, '0')}-${String(month).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
}

function daysInMonth(year: number, month: number): number {
  if (month === 2) {
    const leap = (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0;
    return leap ? 29 : 28;
  }
  return month === 4 || month === 6 || month === 9 || month === 11 ? 30 : 31;
}

/**
 * A calendar date for display, in the reader's locale. Formatted in UTC from
 * its parts so the reader's time zone cannot move it to the previous day.
 * Anything that is not a calendar date is shown as received.
 */
export function formatCalendarDate(value: string, locale: string): string {
  const match = ISO_DATE.exec(value);
  if (!match || !isCalendarDate(value)) return value;
  const instant = Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeZone: 'UTC' }).format(instant);
}
