import { apiPut, invokeTauri } from "$lib/api/client";

export interface CommodityAutocompleteOption {
	id: number;
	book_id: number;
	kind: string;
	symbol: string | null;
	name: string;
	is_active: boolean;
	is_default: boolean;
	score: number;
}

export interface CommodityAutocompleteParams {
	bookId?: number;
	query: string;
	limit?: number;
	activeOnly?: boolean;
	minQueryLength?: number;
}

export async function autocompleteCommodities(
	params: CommodityAutocompleteParams
): Promise<CommodityAutocompleteOption[]> {
	const query = params.query.trim();
	const minQueryLength = params.minQueryLength ?? 1;

	if (query.length < minQueryLength) {
		return [];
	}

	return invokeTauri<CommodityAutocompleteOption[]>("autocomplete_commodities", {
		bookId: params.bookId ?? 1,
		query,
		limit: params.limit ?? 12,
		activeOnly: params.activeOnly ?? false
	});
}

export async function listCurrencies<T>(bookId = 1): Promise<T> {
	return invokeTauri<T>("list_currencies", { bookId });
}

export async function renameCommoditySymbol(input: Record<string, unknown>): Promise<void> {
	const commodityId = input["id"];
	if (typeof commodityId !== "number") {
		throw new Error("commodity id is required");
	}

	await apiPut(`/commodities/${commodityId}`, input);
}

export async function toggleCurrencyActive(currencyId: number): Promise<void> {
	await invokeTauri("toggle_currency_active", { currencyId });
}

export async function setDefaultCurrency(bookId: number, currencyId: number): Promise<void> {
	await invokeTauri("set_default_currency", { bookId, currencyId });
}

export async function saveCurrency(currency: Record<string, unknown>, isUpdate: boolean): Promise<void> {
	await invokeTauri(isUpdate ? "update_currency" : "create_currency", { currency });
}

export async function listFxRatesDaily<T>(bookId = 1, limit = 100): Promise<T> {
	return invokeTauri<T>("list_fx_rates_daily", { bookId, limit });
}

export async function listFxRatesOfficial<T>(bookId = 1): Promise<T> {
	return invokeTauri<T>("list_fx_rates_official", { bookId });
}

export async function listFxRateSources<T>(bookId = 1): Promise<T> {
	return invokeTauri<T>("list_fx_rate_sources", { bookId });
}

export async function getFxRateSettings<T>(bookId = 1): Promise<T> {
	return invokeTauri<T>("get_fx_rate_settings", { bookId });
}

export async function setFxRateSettings<T>(settings: Record<string, unknown>): Promise<T> {
	return invokeTauri<T>("set_fx_rate_settings", { settings });
}

export async function restartFxRateScheduler(): Promise<void> {
	await invokeTauri("restart_fx_rate_scheduler");
}

export async function listFxRateSourceAssignments<T>(bookId = 1): Promise<T> {
	return invokeTauri<T>("list_fx_rate_source_assignments", { bookId });
}

export async function saveFxRateSourceAssignment(assignment: Record<string, unknown>, isUpdate: boolean): Promise<void> {
	await invokeTauri(isUpdate ? "update_fx_rate_source_assignment" : "create_fx_rate_source_assignment", { assignment });
}

export async function deleteFxRateSourceAssignment(id: number): Promise<void> {
	await invokeTauri("delete_fx_rate_source_assignment", { id });
}

export async function listFxRateRefreshState<T>(bookId = 1): Promise<T> {
	return invokeTauri<T>("list_fx_rate_refresh_state", { bookId });
}

export async function refreshFxRatesNow<T>(): Promise<T> {
	return invokeTauri<T>("refresh_fx_rates_now");
}

export async function createFxRateDaily(input: Record<string, unknown>): Promise<void> {
	await invokeTauri("create_fx_rate_daily", { input });
}

export async function deleteFxRateDaily(id: number): Promise<void> {
	await invokeTauri("delete_fx_rate_daily", { id });
}

export async function createFxRateOfficial(input: Record<string, unknown>): Promise<void> {
	await invokeTauri("create_fx_rate_official", { input });
}

export async function deleteFxRateOfficial(id: number): Promise<void> {
	await invokeTauri("delete_fx_rate_official", { id });
}
