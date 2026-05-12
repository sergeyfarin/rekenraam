export function formatMinorWithScale(amountMinor: number, scale: number): string {
  const sign = amountMinor < 0 ? "-" : "";
  const abs = Math.abs(amountMinor);
  if (scale <= 0) {
    return `${sign}${abs}`;
  }
  const factor = 10 ** scale;
  const whole = Math.floor(abs / factor);
  const fraction = String(abs % factor).padStart(scale, "0");
  return `${sign}${whole}.${fraction}`;
}

export function parseAmountToMinor(value: string, scale: number): number | null {
  if (!value) return 0;
  const trimmed = value.trim();
  if (!/^[-+]?\d*(\.\d*)?$/.test(trimmed)) return null;
  const sign = trimmed.startsWith("-") ? -1 : 1;
  const [wholePart, fracPartRaw] = trimmed.replace("+", "").replace("-", "").split(".");
  const whole = wholePart ? parseInt(wholePart, 10) : 0;
  const fracPart = (fracPartRaw ?? "").padEnd(scale, "0").slice(0, scale);
  const fraction = fracPart ? parseInt(fracPart, 10) : 0;
  const factor = 10 ** scale;
  return sign * (whole * factor + fraction);
}
