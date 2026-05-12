export function parseSmartDate(raw: string, todayIso: string): string | null {
  const s = raw.trim();
  if (!s) return null;

  if (s.toLowerCase() === "t" || s.toLowerCase() === "today") return todayIso;

  if (/^\d{4}-\d{1,2}-\d{1,2}$/.test(s)) {
    const [y, m, d] = s.split("-").map(Number);
    return toIso(y, m, d);
  }

  const today = new Date(todayIso + "T12:00:00");

  function toIso(y: number, m: number, d: number): string | null {
    if (m < 1 || m > 12 || d < 1 || d > 31) return null;
    const dt = new Date(y, m - 1, d);
    if (dt.getMonth() !== m - 1) return null;
    return `${y}-${String(m).padStart(2, "0")}-${String(d).padStart(2, "0")}`;
  }

  function guessYear(m: number, d: number): number {
    const y = today.getFullYear();
    const candidate = new Date(y, m - 1, d);
    const diffDays = (candidate.getTime() - today.getTime()) / 86400000;
    return diffDays > 60 ? y - 1 : y;
  }

  function resolveMonthDay(a: number, b: number, year: number | null): string | null {
    let month: number;
    let day: number;
    if (a > 12 && b <= 12) {
      day = a;
      month = b;
    } else if (b > 12 && a <= 12) {
      day = b;
      month = a;
    } else {
      month = a;
      day = b;
    }
    const y = year ?? guessYear(month, day);
    return toIso(y, month, day);
  }

  const sepMatch = s.match(/^(\d{1,4})[\/\-\.](\d{1,2})(?:[\/\-\.](\d{2,4}))?$/);
  if (sepMatch) {
    const [, a, b, c] = sepMatch;
    const aNum = parseInt(a, 10);
    const bNum = parseInt(b, 10);
    if (c) {
      const cNum = parseInt(c, 10);
      if (aNum > 31) {
        return toIso(aNum, bNum, cNum);
      }
      const year = cNum < 100 ? 2000 + cNum : cNum;
      return resolveMonthDay(aNum, bNum, year);
    }
    return resolveMonthDay(aNum, bNum, null);
  }

  if (/^\d{1,4}$/.test(s)) {
    const n = parseInt(s, 10);
    if (s.length <= 2) {
      const d = n;
      if (d < 1 || d > 31) return null;
      let m = today.getMonth() + 1;
      let y = today.getFullYear();
      const candidate = new Date(y, m - 1, d);
      if (isNaN(candidate.getTime()) || candidate.getMonth() !== m - 1) {
        m -= 1;
        if (m === 0) {
          m = 12;
          y -= 1;
        }
      } else {
        const diff = (candidate.getTime() - today.getTime()) / 86400000;
        if (diff > 2) {
          m -= 1;
          if (m === 0) {
            m = 12;
            y -= 1;
          }
        }
      }
      return toIso(y, m, d);
    }
    if (s.length === 3) {
      return resolveMonthDay(parseInt(s[0], 10), parseInt(s.slice(1), 10), null);
    }
    return resolveMonthDay(parseInt(s.slice(0, 2), 10), parseInt(s.slice(2), 10), null);
  }

  return null;
}
