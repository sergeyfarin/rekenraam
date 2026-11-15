/** URL state belongs to the view, including history navigation (#447). */
export type ImportView = 'files' | 'connections' | 'rules' | 'dividends';
export function importView(params: Pick<URLSearchParams, 'get'>): ImportView {
  const value = params.get('view') ?? params.get('tab');
  return value === 'connections' || value === 'rules' || value === 'dividends' ? value : 'files';
}
export function importViewURL(url: Pick<URL, 'href'>, view: string): string {
  const next = new URL(url.href);
  next.searchParams.set('view', view);
  next.searchParams.delete('tab');
  return next.pathname + next.search + next.hash;
}
