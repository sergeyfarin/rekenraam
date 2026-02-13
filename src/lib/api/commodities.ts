import { invoke } from "@tauri-apps/api/core";

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

	return invoke<CommodityAutocompleteOption[]>("autocomplete_commodities", {
		bookId: params.bookId ?? 1,
		query,
		limit: params.limit ?? 12,
		activeOnly: params.activeOnly ?? false
	});
}
