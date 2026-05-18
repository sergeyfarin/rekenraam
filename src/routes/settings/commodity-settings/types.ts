export type Commodity = {
  id: number;
  book_id: number;
  kind: string;
  symbol: string | null;
  name: string;
  scale: number;
  metadata: string | null;
  created_at: string;
  updated_at: string;
};

export type Currency = {
  id: number;
  book_id: number;
  symbol: string | null;
  display_symbol: string | null;
  name: string;
  scale: number;
  is_active: boolean;
  is_default: boolean;
  created_at: string;
  updated_at: string;
};

export type FxRateDaily = {
  id: number;
  book_id: number;
  from_currency_id: number;
  from_currency_symbol: string | null;
  to_currency_id: number;
  to_currency_symbol: string | null;
  rate_date: string;
  rate: number;
  source: string | null;
  source_id: number | null;
  is_derived: boolean;
  derived_via_currency_id: number | null;
  created_at: string;
};

export type FxRateOfficial = {
  id: number;
  book_id: number;
  from_currency_id: number;
  from_currency_symbol: string | null;
  to_currency_id: number;
  to_currency_symbol: string | null;
  period_type: string;
  period_year: number;
  period_month: number | null;
  rate: number;
  source_name: string;
  source_url: string | null;
  source_date: string | null;
  notes: string | null;
  created_at: string;
  updated_at: string;
};

export type FxRateSource = {
  id: number;
  book_id: number;
  name: string;
  country_code: string | null;
  website_url: string | null;
  notes: string | null;
  created_at: string;
  updated_at: string;
};

export type FxRateSettings = {
  book_id: number;
  base_currency_id: number;
  base_currency_symbol: string | null;
  default_source_id: number | null;
  default_source_name: string | null;
  refresh_enabled: boolean;
  refresh_hour_utc: number;
  refresh_minute_utc: number;
  max_backfill_days: number;
  weekend_policy: string;
  created_at: string;
  updated_at: string;
};

export type FxRateSourceAssignment = {
  id: number;
  book_id: number;
  from_currency_id: number;
  from_currency_symbol: string | null;
  to_currency_id: number;
  to_currency_symbol: string | null;
  source_id: number;
  source_name: string | null;
  effective_from: string;
  effective_to: string | null;
  created_at: string;
  updated_at: string;
};

export type FxRateRefreshState = {
  id: number;
  book_id: number;
  from_currency_id: number;
  from_currency_symbol: string | null;
  to_currency_id: number;
  to_currency_symbol: string | null;
  source_id: number;
  source_name: string | null;
  last_success_date: string | null;
  last_attempt_at: string | null;
  last_error: string | null;
  created_at: string;
  updated_at: string;
};

export type FxRefreshRunSummary = {
  book_id: number;
  trigger: string;
  started_at: string;
  finished_at: string;
  pairs_total: number;
  pairs_success: number;
  pairs_failed: number;
  rates_inserted: number;
  derived_inserted: number;
  last_error: string | null;
};

export type FxRefreshExecutionStatus = {
  book_id: number;
  scheduler_enabled: boolean;
  scheduler_poll_seconds: number;
  worker_started_at: string | null;
  is_running: boolean;
  active_book_ids: number[];
  next_scheduled_at: string | null;
  last_run: FxRefreshRunSummary | null;
};

export type CommoditySettingsTab =
  | "currencies"
  | "commodities"
  | "fx-daily"
  | "fx-official"
  | "fx-settings";

export type CurrencyForm = {
  symbol: string;
  display_symbol: string;
  name: string;
  scale: number;
};

export type FxDailyForm = {
  from_currency_id: number;
  to_currency_id: number;
  rate_date: string;
  rate: number;
  source: string;
};

export type FxOfficialForm = {
  from_currency_id: number;
  to_currency_id: number;
  period_type: string;
  period_year: number;
  period_month: number | null;
  rate: number;
  source_name: string;
  source_url: string;
  notes: string;
};

export type FxAssignmentForm = {
  from_currency_id: number;
  to_currency_id: number;
  source_id: number;
  effective_from: string;
  effective_to: string;
};

export function createEmptyCurrencyForm(): CurrencyForm {
  return { symbol: "", display_symbol: "", name: "", scale: 2 };
}

export function getActiveCurrencies(currencies: Currency[]): Currency[] {
  return currencies.filter((currency) => currency.is_active);
}

export function getDefaultCurrency(currencies: Currency[]): Currency | undefined {
  return currencies.find((currency) => currency.is_default);
}

export function createFxDailyDraft(currencies: Currency[]): FxDailyForm {
  const activeCurrencies = getActiveCurrencies(currencies);
  const defaultCurrency = getDefaultCurrency(currencies);
  return {
    from_currency_id: activeCurrencies[0]?.id || 0,
    to_currency_id: defaultCurrency?.id || 0,
    rate_date: new Date().toISOString().split("T")[0],
    rate: 1,
    source: "",
  };
}

export function createFxOfficialDraft(
  currencies: Currency[],
  sources: FxRateSource[]
): FxOfficialForm {
  const activeCurrencies = getActiveCurrencies(currencies);
  const defaultCurrency = getDefaultCurrency(currencies);
  return {
    from_currency_id: activeCurrencies[0]?.id || 0,
    to_currency_id: defaultCurrency?.id || 0,
    period_type: "yearly",
    period_year: new Date().getFullYear(),
    period_month: null,
    rate: 1,
    source_name: sources[0]?.name || "",
    source_url: "",
    notes: "",
  };
}

export function createFxAssignmentDraft(
  currencies: Currency[],
  sources: FxRateSource[]
): FxAssignmentForm {
  const activeCurrencies = getActiveCurrencies(currencies);
  const defaultCurrency = getDefaultCurrency(currencies);
  return {
    from_currency_id: activeCurrencies[0]?.id || 0,
    to_currency_id: defaultCurrency?.id || 0,
    source_id: sources[0]?.id || 0,
    effective_from: new Date().toISOString().split("T")[0],
    effective_to: "",
  };
}

export function formatOfficialPeriod(rate: FxRateOfficial): string {
  return rate.period_type === "monthly"
    ? `${rate.period_year}-${String(rate.period_month).padStart(2, "0")}`
    : String(rate.period_year);
}