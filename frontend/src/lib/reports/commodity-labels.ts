/**
 * One commodity label map for every report surface.
 *
 * A report can be filtered to any commodity that carries a balance, and that is
 * not only currencies: the commodity behind a held instrument is filterable
 * too, which the filter control has always known. The screens did not, so a
 * report narrowed to `AAPL` rendered its rows, chart caption, print sheet, and
 * CSV as "Commodity #123" — a filter you can set and then cannot read.
 *
 * Both surfaces read this map, so the name in the picker is the name in the
 * table by construction rather than by two implementations agreeing.
 */

export type CurrencyNamed = { id: number; code: string };
export type InstrumentNamed = {
  commodity_id: number;
  commodity_code?: string | null;
  display_name?: string | null;
};

/**
 * Builds the id → label map.
 *
 * A currency wins over an instrument on the same id: a commodity that is both
 * is a currency, and its code is the name people use for it.
 */
export function commodityLabelMap(
  currencies: readonly CurrencyNamed[] | undefined,
  instruments: readonly InstrumentNamed[] | undefined
): Map<number, string> {
  const labels = new Map<number, string>();
  for (const currency of currencies ?? []) {
    labels.set(currency.id, currency.code);
  }
  for (const instrument of instruments ?? []) {
    if (labels.has(instrument.commodity_id)) continue;
    const label = instrument.commodity_code || instrument.display_name;
    if (label) {
      labels.set(instrument.commodity_id, label);
    }
  }
  return labels;
}
