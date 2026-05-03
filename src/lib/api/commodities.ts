import { apiGet, apiPost, apiPut, hasApiBaseUrl, invokeTauri } from "$lib/api/client";

function requireDesktopPricingAutomation(feature: string): void {
	if (hasApiBaseUrl()) {
		throw new Error(`${feature} is not migrated yet. In the web app, FX and market price refresh will run from a backend schedule rather than the browser.`);
	}
}

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
	return apiGet<T>(`/currencies?book_id=${bookId}`);
}

export async function renameCommoditySymbol(input: Record<string, unknown>): Promise<void> {
	const commodityId = input["id"];
	if (typeof commodityId !== "number") {
		throw new Error("commodity id is required");
	}

	await apiPut(`/commodities/${commodityId}`, input);
}

export async function toggleCurrencyActive(bookId: number, currencyId: number, isActive: boolean): Promise<void> {
	await apiPost(`/currencies/${currencyId}/activation`, { book_id: bookId, is_active: isActive });
}

export async function setDefaultCurrency(bookId: number, currencyId: number): Promise<void> {
	await apiPost(`/currencies/${currencyId}/default?book_id=${bookId}`, {});
}

export async function saveCurrency(currency: Record<string, unknown>, isUpdate: boolean): Promise<void> {
	if (isUpdate) {
		const currencyId = currency["id"];
		if (typeof currencyId !== "number") {
			throw new Error("currency id is required");
		}
		await apiPut(`/currencies/${currencyId}`, currency);
		return;
	}

	await apiPost("/currencies", currency);
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
	requireDesktopPricingAutomation("FX refresh settings");
	return invokeTauri<T>("get_fx_rate_settings", { bookId });
}

export async function setFxRateSettings<T>(settings: Record<string, unknown>): Promise<T> {
	requireDesktopPricingAutomation("FX refresh settings");
	return invokeTauri<T>("set_fx_rate_settings", { settings });
}

export async function restartFxRateScheduler(): Promise<void> {
	requireDesktopPricingAutomation("Client-side FX scheduler restarts");
	await invokeTauri("restart_fx_rate_scheduler");
}

export async function listFxRateSourceAssignments<T>(bookId = 1): Promise<T> {
	requireDesktopPricingAutomation("FX source assignments");
	return invokeTauri<T>("list_fx_rate_source_assignments", { bookId });
}

export async function saveFxRateSourceAssignment(assignment: Record<string, unknown>, isUpdate: boolean): Promise<void> {
	requireDesktopPricingAutomation("FX source assignments");
	await invokeTauri(isUpdate ? "update_fx_rate_source_assignment" : "create_fx_rate_source_assignment", { assignment });
}

export async function deleteFxRateSourceAssignment(id: number): Promise<void> {
	requireDesktopPricingAutomation("FX source assignments");
	await invokeTauri("delete_fx_rate_source_assignment", { id });
}

export async function listFxRateRefreshState<T>(bookId = 1): Promise<T> {
	requireDesktopPricingAutomation("FX refresh status");
	return invokeTauri<T>("list_fx_rate_refresh_state", { bookId });
}

export async function refreshFxRatesNow<T>(): Promise<T> {
	requireDesktopPricingAutomation("Manual FX refresh");
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
