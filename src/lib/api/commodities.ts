import { apiDelete, apiGet, apiPost, apiPut, hasApiBaseUrl } from "$lib/api/client";

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

	const searchParams = new URLSearchParams({
		book_id: String(params.bookId ?? 1),
		query,
		limit: String(params.limit ?? 12),
		active_only: String(params.activeOnly ?? false)
	});
	return apiGet<CommodityAutocompleteOption[]>(`/commodities/autocomplete?${searchParams.toString()}`);
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
	return apiGet<T>(`/pricing/rates/daily?book_id=${bookId}&limit=${limit}`);
}

export async function listFxRatesOfficial<T>(bookId = 1): Promise<T> {
	return apiGet<T>(`/pricing/rates/official?book_id=${bookId}`);
}

export async function listFxRateSources<T>(bookId = 1): Promise<T> {
	return apiGet<T>(`/pricing/sources?book_id=${bookId}`);
}

export async function getFxRateSettings<T>(bookId = 1): Promise<T> {
	return apiGet<T>(`/pricing/policy?book_id=${bookId}`);
}

export async function setFxRateSettings<T>(settings: Record<string, unknown>): Promise<T> {
	return apiPut<T, Record<string, unknown>>("/pricing/policy", settings);
}

export async function restartFxRateScheduler(): Promise<void> {
	if (hasApiBaseUrl()) {
		return;
	}
	throw new Error("Client-side FX scheduler restarts are not supported in this build.");
}

export async function listFxRateSourceAssignments<T>(bookId = 1): Promise<T> {
	return apiGet<T>(`/pricing/source-assignments?book_id=${bookId}`);
}

export async function saveFxRateSourceAssignment(assignment: Record<string, unknown>, isUpdate: boolean): Promise<void> {
	if (isUpdate) {
		const assignmentId = assignment["id"];
		if (typeof assignmentId !== "number") {
			throw new Error("pricing source assignment id is required");
		}
		await apiPut(`/pricing/source-assignments/${assignmentId}`, assignment);
		return;
	}
	await apiPost("/pricing/source-assignments", assignment);
}

export async function deleteFxRateSourceAssignment(id: number): Promise<void> {
	await apiDelete(`/pricing/source-assignments/${id}`);
}

export async function listFxRateRefreshState<T>(bookId = 1): Promise<T> {
	return apiGet<T>(`/pricing/refresh-state?book_id=${bookId}`);
}

export async function getFxRefreshExecutionStatus<T>(bookId = 1): Promise<T> {
	if (hasApiBaseUrl()) {
		return apiGet<T>(`/pricing/refresh/execution-status?book_id=${bookId}`);
	}
	requireDesktopPricingAutomation("Backend-owned FX execution status");
	throw new Error("Backend-owned FX execution status is not supported in this build.");
}

export async function listFxRefreshRunHistory<T>(bookId = 1, limit = 10): Promise<T> {
	if (hasApiBaseUrl()) {
		return apiGet<T>(`/pricing/refresh/history?book_id=${bookId}&limit=${limit}`);
	}
	requireDesktopPricingAutomation("Backend-owned FX refresh history");
	throw new Error("Backend-owned FX refresh history is not supported in this build.");
}

export async function refreshFxRatesNow<T>(bookId = 1): Promise<T> {
	if (hasApiBaseUrl()) {
		return apiPost<T, { book_id: number }>("/pricing/refresh/run", { book_id: bookId });
	}
	requireDesktopPricingAutomation("Manual FX refresh");
	throw new Error("Manual FX refresh is not supported in this build.");
}

export async function createFxRateDaily(input: Record<string, unknown>): Promise<void> {
	await apiPost("/pricing/rates/daily", input);
}

export async function deleteFxRateDaily(id: number): Promise<void> {
	await apiDelete(`/pricing/rates/daily/${id}`);
}

export async function createFxRateOfficial(input: Record<string, unknown>): Promise<void> {
	await apiPost("/pricing/rates/official", input);
}

export async function deleteFxRateOfficial(id: number): Promise<void> {
	await apiDelete(`/pricing/rates/official/${id}`);
}

export async function listMarketPrices<T>(bookId = 1, limit = 100): Promise<T> {
	return apiGet<T>(`/pricing/market-prices?book_id=${bookId}&limit=${limit}`);
}

export async function createMarketPrice(input: Record<string, unknown>): Promise<void> {
	await apiPost("/pricing/market-prices", input);
}

export async function deleteMarketPrice(id: number): Promise<void> {
	await apiDelete(`/pricing/market-prices/${id}`);
}

export async function listPricingSourceHealth<T>(bookId = 1): Promise<T> {
	return apiGet<T>(`/pricing/source-health?book_id=${bookId}`);
}
