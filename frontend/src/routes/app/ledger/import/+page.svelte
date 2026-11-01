<script lang="ts">
  import { maskFinancialText } from '#lib/money/format.ts';
  import { onDestroy } from 'svelte';
  import { parseISO } from 'date-fns';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import Upload from '@lucide/svelte/icons/upload';
  import CheckCircle from '@lucide/svelte/icons/circle-check';
  import AlertCircle from '@lucide/svelte/icons/circle-alert';
  import Info from '@lucide/svelte/icons/info';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Plus from '@lucide/svelte/icons/plus';
  import Loader from '@lucide/svelte/icons/loader-circle';
  import Panel from '#lib/components/panel.svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { APIClientError } from '#lib/api/client.ts';
  import { cryptoCSVSummary } from '#lib/imports/crypto-csv.ts';
  import PayeeResolutionPanel from '#lib/imports/payee-resolution-panel.svelte';
  import ImportRulesPanel from '#lib/imports/import-rules-panel.svelte';
  import type { PayeeResponse } from '#lib/api/payees.ts';
  import { linkableShortFill, linkableSplitFill, linkedFillKind, sourceCorrectionKind, unsupportedSourceFill } from '#lib/imports/source-correction.ts';
  import ShortLinkPanel from '#lib/imports/short-link-panel.svelte';
  import SplitLinkPanel from '#lib/imports/split-link-panel.svelte';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import { investmentInstrumentsQueryOptions } from '#lib/api/investments.ts';
  import { accountsQueryOptions } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions } from '#lib/api/currencies.ts';
  import ReconciliationConfirm from '#lib/investments/reconciliation-confirm.svelte';
  import GainImpactList from '#lib/investments/gain-impact-list.svelte';
  import {
    gainAcknowledgement,
    gainImpactCurrency,
    gainImpactRows,
    hasGainChanges,
    type GainImpactRow,
    checkpointOverride
  } from '#lib/investments/gain-impact.ts';
  import { categoriesQueryOptions } from '#lib/api/categories.ts';
  import { tagsQueryOptions } from '#lib/api/tags.ts';
  import {
    analyzeCSVImport,
    startImport,
    startOnlineImport,
    getImportBatch,
    getFullImportBatch,
    patchImportBatch,
    commitImportBatch,
    previewCommitImportBatch,
    correctTrading212Buy,
    correctTrading212Sale,
    previewTrading212SaleCorrectionReconciliation,
    previewTrading212BuyCorrectionReconciliation,
    type ReconciliationImpactResponse,
    type PreviewCommitResponse,
    discardImportBatch,
    parseNormalized,
    parseResolution,
    parseBatchSourceMeta,
    listImportProfiles,
    createImportProfile,
    updateImportProfile,
    deleteImportProfile,
    importProfilesQueryKey,
    type AnalyzeCSVImportResponse,
    type StartImportResponse,
    type ImportBatch,
    type ImportStagedRow,
    type CommitImportBatchResponse,
    type ImportResolution,
    type ParseWarning
  } from '#lib/api/imports.ts';
  import {
    parseCSVProfileConfig,
    validCSVInvestmentMapping,
    type CSVInvestmentMapping,
    rankCSVProfiles,
    uniqueCSVProfileSuggestion,
    type CSVDelimiter,
    type CSVProfileConfig
  } from '#lib/imports/csv-profile.ts';
  import CSVInvestmentMappingForm from '#lib/imports/csv-investment-mapping.svelte';
  import CSVInvestmentReview from '#lib/imports/csv-investment-review.svelte';
  import { textEncodingOptions } from '#lib/imports/text-encodings.ts';
  import {
    listImportConnections,
    createImportConnection,
    updateImportConnection,
    deleteImportConnection,
    refreshImportConnection,
    importConnectionsQueryKey,
    type ImportConnection
  } from '#lib/api/connections.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { invalidateInvestmentReads } from '#lib/investments/invalidate.ts';

  // ── Page state ─────────────────────────────────────────────────────
  type Step = 'upload' | 'fetching' | 'preview' | 'result';

  let step = $state<Step>('upload');

  // Upload step
  let cryptoCSV = $state(false);
  let cryptoHoldingId = $state(0);
  let cryptoCashId = $state(0);
  let selectedFile = $state<File | null>(null);
  let textEncoding = $state('auto');
  let uploading = $state(false);
  let uploadError = $state<unknown>(undefined);
  let csvHeaders = $state.raw<string[]>([]);
  let csvAnalysis = $state.raw<AnalyzeCSVImportResponse | null>(null);
  let csvAnalysisError = $state<unknown>(undefined);
  let analyzingCSV = $state(false);
  let csvAnalysisRequest = 0;
  let selectedProfileId = $state('');
  let profileName = $state('');
  let csvDelimiter = $state<CSVDelimiter>('comma');
  let csvDateColumn = $state('');
  let csvPayeeColumn = $state('');
  let csvMemoColumn = $state('');
  let csvCategoryColumn = $state('');
  let csvExternalRefColumn = $state('');
  let csvAmountMode = $state<'single' | 'debit_credit'>('single');
  let csvAmountColumn = $state('');
  let csvDebitColumn = $state('');
  let csvCreditColumn = $state('');
  let csvDateLayout = $state<'DMY' | 'MDY' | 'YMD'>('DMY');
  let csvDecimalSeparator = $state<'.' | ','>(',');
  let csvInvertAmount = $state(false);
  let csvInvestment = $state<CSVInvestmentMapping | undefined>();
  let profileSelectionTouched = $state(false);
  let editingProfileId = $state<number | null>(null);
  let savingProfile = $state(false);
  let deletingProfileId = $state<number | null>(null);
  let confirmDeleteProfileId = $state<number | null>(null);
  let profileMutationError = $state<unknown>(undefined);

  // Online import (fetching) step
  let startingOnlineConnectionId = $state<number | null>(null);
  let onlineImportError = $state<unknown>(undefined);
  let fetchFailed = $state(false);
  let pollTimer: ReturnType<typeof setTimeout> | null = null;

  // Preview step
  let previewData = $state.raw<StartImportResponse | null>(null);
  let batchId = $state<number | null>(null);
  // Per-row resolution: accountId, commodityId, categoryId / transferAccountId
  let rowResolutions = $state<Map<number, ImportResolution>>(new Map());
  let globalAccountId = $state<number | undefined>(undefined);
  let globalCommodityId = $state<number | undefined>(undefined);
  let globalCategoryId = $state<number | undefined>(undefined);
  let globalTransferAccountId = $state<number | undefined>(undefined);

  // Commit step
  let committing = $state(false);
  let commitError = $state<unknown>(undefined);
  let commitResult = $state.raw<CommitImportBatchResponse | null>(null);
  let reconciliationOverride = $state(false);
  // The batch override names the checkpoints shown beside it: the preview's
  // union over the pending rows. A row reaching any other checkpoint is held
  // at commit, not posted (#305).
  type BatchCheckpoint = PreviewCommitResponse['reconciliation_checkpoints'][number];
  let reviewedCheckpoints = $state.raw<BatchCheckpoint[] | null>(null);
  let reviewedCheckpointsChanged = $state(false);
  let reviewedCheckpointsPending = $state(false);
  let reviewedCheckpointsError = $state<unknown>(undefined);
  let sourceCorrectionRowID = $state<number | null>(null);
  let splitLinkRowID = $state<number | null>(null);
  let shortLinkRowID = $state<number | null>(null);
  let sourceCorrectionReason = $state('');
  let sourceCorrectionOverride = $state(false);
  let sourceCorrectionImpact = $state.raw<ReconciliationImpactResponse | null>(null);
  let sourceCorrectionPending = $state(false);
  let sourceCorrectionError = $state<unknown>(undefined);
  let sourceCorrectionSuccessID = $state<number | null>(null);
  let sourceCorrectionAcceptGains = $state(false);
  // Imported acquisitions that would change committed sale gains are shown
  // before commit and committed only with per-row acknowledgements (T-126).
  let gainReview = $state<{
    rows: GainImpactRow[];
    acknowledgements: { row_id: number; acknowledgement: string }[];
    refreshed: boolean;
  } | null>(null);
  let reviewChangedPending = $state(false);
  let reviewChangedError = $state<unknown>(undefined);
  let reviewRemainingError = $state<unknown>(undefined);

  // Discard
  let discarding = $state(false);
  let discardError = $state<unknown>(undefined);
  let showDiscardConfirm = $state(false);

  const sourceCorrectionDateFormatter = $derived(new Intl.DateTimeFormat(getLocale(), {
    year: 'numeric', month: 'short', day: 'numeric'
  }));

  function formatDate(value: string): string {
    return sourceCorrectionDateFormatter.format(parseISO(value));
  }

  // ── Queries ────────────────────────────────────────────────────────
  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const csrfToken = $derived(sessionQuery.data?.csrf_token ?? '');

  const accountsQuery = createQuery(() => accountsQueryOptions());
  const cryptoInstrumentsQuery = createQuery(() => ({ ...investmentInstrumentsQueryOptions(), enabled: cryptoCSV }));
  const cryptoCommodityIDs = $derived(new Set((cryptoInstrumentsQuery.data?.instruments ?? []).filter(i => i.instrument_type === 'crypto').map(i => i.commodity_id)));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());
  const categoriesQuery = createQuery(() => categoriesQueryOptions());
  const tagsQuery = createQuery(() => tagsQueryOptions());

  const accounts = $derived(accountsQuery.data?.accounts ?? []);
  const currencies = $derived(currenciesQuery.data?.currencies ?? []);
  const gainCurrency = $derived(gainImpactCurrency(new Map(currencies.map((currency) => [currency.id, currency]))));
  const sourceCorrectionGainRows = $derived(sourceCorrectionImpact && hasGainChanges(sourceCorrectionImpact.gain_impact)
    ? gainImpactRows(sourceCorrectionImpact.gain_impact.changes, gainCurrency, getLocale())
    : []);
  const categories = $derived(categoriesQuery.data?.categories ?? []);
  const tags = $derived(tagsQuery.data?.tags ?? []);

  // ── Upload ─────────────────────────────────────────────────────────
  const profilesQuery = createQuery(() => ({ queryKey: importProfilesQueryKey, queryFn: listImportProfiles, retry: false }));
  const csvProfiles = $derived(profilesQuery.data?.profiles.filter((profile) => profile.adapter_kind === 'csv') ?? []);
  const selectedIsCSV = $derived(!cryptoCSV && (selectedFile?.name.toLowerCase().endsWith('.csv') ?? false));
  const selectedIsQIF = $derived(selectedFile?.name.toLowerCase().endsWith('.qif') ?? false);
  const selectedIsTextImport = $derived(selectedIsCSV || selectedIsQIF);
  const rankedCSVProfiles = $derived(selectedFile ? rankCSVProfiles(csvProfiles, selectedFile.name, csvDelimiter, csvHeaders) : []);
  const suggestedCSVProfile = $derived(uniqueCSVProfileSuggestion(rankedCSVProfiles));
  const orderedCSVProfiles = $derived([
    ...rankedCSVProfiles.map((entry) => entry.profile),
    ...csvProfiles.filter((profile) => !rankedCSVProfiles.some((entry) => entry.profile.id === profile.id))
  ]);
  let autoSuggestionKey = $state('');
  $effect(() => {
    const key = `${selectedFile?.name ?? ''}|${csvDelimiter}|${csvHeaders.join('\u001f')}|${csvProfiles.map((profile) => `${profile.id}:${profile.updated_at}`).join(',')}`;
    if (key !== autoSuggestionKey) {
      autoSuggestionKey = key;
      if (!profileSelectionTouched && suggestedCSVProfile) selectedProfileId = String(suggestedCSVProfile.id);
    }
  });
  const csvMappingValid = $derived(
    !!profileName.trim() && !!csvDateColumn &&
    (csvAmountMode === 'single' ? !!csvAmountColumn : !!csvDebitColumn && !!csvCreditColumn) && validCSVInvestmentMapping(csvInvestment)
  );
  const newCSVProfileValid = $derived(
    !selectedProfileId && csvMappingValid
  );

  async function handleFileChange(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    selectedFile = input.files?.[0] ?? null;
    textEncoding = 'auto';
    uploadError = undefined;
    csvAnalysis = null;
    csvAnalysisError = undefined;
    csvAnalysisRequest += 1;
    selectedProfileId = '';
    editingProfileId = null;
    profileSelectionTouched = false;
    confirmDeleteProfileId = null;
    profileMutationError = undefined;
    if (!selectedFile?.name.toLowerCase().endsWith('.csv')) {
      csvHeaders = [];
      return;
    }
    await analyzeSelectedCSV();
  }

  async function handleCSVDelimiterChange(e: Event) {
    csvDelimiter = (e.currentTarget as HTMLSelectElement).value as CSVDelimiter;
    selectedProfileId = '';
    editingProfileId = null;
    profileSelectionTouched = false;
    await analyzeSelectedCSV(csvDelimiter);
  }

  async function handleTextEncodingChange(e: Event) {
    textEncoding = (e.currentTarget as HTMLSelectElement).value;
    uploadError = undefined;
    if (!selectedIsCSV) return;
    selectedProfileId = '';
    editingProfileId = null;
    profileSelectionTouched = false;
    await analyzeSelectedCSV();
  }

  async function analyzeSelectedCSV(delimiter?: CSVDelimiter) {
    const file = selectedFile;
    if (!file || !file.name.toLowerCase().endsWith('.csv')) return;
    const request = ++csvAnalysisRequest;
    analyzingCSV = true;
    csvAnalysisError = undefined;
    csvAnalysis = null;
    csvHeaders = [];
    try {
      const result = await analyzeCSVImport(file, csrfToken, textEncoding, delimiter);
      if (request !== csvAnalysisRequest || selectedFile !== file) return;
      csvAnalysis = result;
      csvHeaders = result.headers;
      csvDelimiter = result.delimiter;
      csvDateColumn = result.headers.includes(csvDateColumn) ? csvDateColumn : (result.headers[0] ?? '');
      csvPayeeColumn = result.headers.includes(csvPayeeColumn) ? csvPayeeColumn : (result.headers[1] ?? '');
      csvAmountColumn = result.headers.includes(csvAmountColumn) ? csvAmountColumn : (result.headers.at(-1) ?? '');
    } catch (err) {
      if (request !== csvAnalysisRequest || selectedFile !== file) return;
      csvAnalysisError = err;
    } finally {
      if (request === csvAnalysisRequest) analyzingCSV = false;
    }
  }

  function csvProfileConfig(): CSVProfileConfig {
    return {
      investment: csvInvestment,
      delimiter: csvDelimiter,
      date_column: csvDateColumn,
      payee_column: csvPayeeColumn || undefined,
      memo_column: csvMemoColumn || undefined,
      category_column: csvCategoryColumn || undefined,
      external_ref_column: csvExternalRefColumn || undefined,
      amount_column: csvAmountMode === 'single' ? csvAmountColumn : undefined,
      debit_column: csvAmountMode === 'debit_credit' ? csvDebitColumn : undefined,
      credit_column: csvAmountMode === 'debit_credit' ? csvCreditColumn : undefined,
      date_layout: csvDateLayout,
      decimal_separator: csvDecimalSeparator,
      invert_amount: csvInvertAmount,
      source_filename: selectedFile?.name,
      headers: csvHeaders
    };
  }

  function handleProfileSelection(e: Event) {
    selectedProfileId = (e.currentTarget as HTMLSelectElement).value;
    if (!selectedProfileId) profileName = '';
    profileSelectionTouched = true;
    editingProfileId = null;
    confirmDeleteProfileId = null;
    profileMutationError = undefined;
  }

  function beginProfileEdit() {
    const profile = csvProfiles.find((candidate) => candidate.id === Number(selectedProfileId));
    if (!profile) return;
    const config = parseCSVProfileConfig(profile.config);
    if (!config) return;
    profileName = profile.name;
    csvDelimiter = config.delimiter;
    csvDateColumn = config.date_column;
    csvPayeeColumn = config.payee_column ?? '';
    csvMemoColumn = config.memo_column ?? '';
    csvCategoryColumn = config.category_column ?? '';
    csvExternalRefColumn = config.external_ref_column ?? '';
    csvAmountMode = config.amount_column ? 'single' : 'debit_credit';
    csvAmountColumn = config.amount_column ?? '';
    csvDebitColumn = config.debit_column ?? '';
    csvCreditColumn = config.credit_column ?? '';
    csvDateLayout = config.date_layout;
    csvDecimalSeparator = config.decimal_separator;
    csvInvertAmount = config.invert_amount;
    csvInvestment = config.investment;
    editingProfileId = profile.id;
    profileSelectionTouched = true;
    profileMutationError = undefined;
  }

  async function saveProfileEdit() {
    if (!editingProfileId || !csvMappingValid) return;
    savingProfile = true;
    profileMutationError = undefined;
    try {
      await updateImportProfile(editingProfileId, { name: profileName.trim(), config: JSON.stringify(csvProfileConfig()) }, csrfToken);
      await queryClient.invalidateQueries({ queryKey: importProfilesQueryKey });
      editingProfileId = null;
    } catch (err) {
      profileMutationError = err;
    } finally {
      savingProfile = false;
    }
  }

  async function handleDeleteProfile() {
    const profileId = Number(selectedProfileId);
    if (!profileId) return;
    deletingProfileId = profileId;
    profileMutationError = undefined;
    try {
      await deleteImportProfile(profileId, csrfToken);
      selectedProfileId = '';
      profileName = '';
      editingProfileId = null;
      confirmDeleteProfileId = null;
      profileSelectionTouched = true;
      await queryClient.invalidateQueries({ queryKey: importProfilesQueryKey });
    } catch (err) {
      profileMutationError = err;
    } finally {
      deletingProfileId = null;
    }
  }

  async function handleUpload() {
    if (!selectedFile) return;
    uploading = true;
    uploadError = undefined;

    try {
      let profileId = selectedProfileId ? Number(selectedProfileId) : undefined;
      if (selectedIsCSV && !profileId) {
        const profile = await createImportProfile({ name: profileName.trim(), adapter_kind: 'csv', config: JSON.stringify(csvProfileConfig()) }, csrfToken);
        profileId = profile.id;
        selectedProfileId = String(profile.id);
        await queryClient.invalidateQueries({ queryKey: importProfilesQueryKey });
      }
      const result = await startImport(selectedFile, csrfToken, cryptoCSV ? undefined : profileId, selectedIsTextImport ? textEncoding : 'auto', cryptoCSV ? { holdingAccountId: cryptoHoldingId, cashAccountId: cryptoCashId } : undefined);
      previewData = result;
      batchId = result.batch.id;
      rowResolutions = new Map(result.rows.map((row) => [row.id, parseResolution(row)]));
      step = 'preview';
    } catch (err) {
      uploadError = err;
    } finally {
      uploading = false;
    }
  }

  // ── Online import (Trading 212) ───────────────────────────────────────
  async function handleStartOnlineImport(connectionId: number) {
    startingOnlineConnectionId = connectionId;
    onlineImportError = undefined;
    fetchFailed = false;
    try {
      const result = await startOnlineImport(connectionId, csrfToken);
      batchId = result.batch.id;
      step = 'fetching';
      pollFetchStatus();
    } catch (err) {
      onlineImportError = err;
    } finally {
      startingOnlineConnectionId = null;
    }
  }

  async function handleRefreshConnection(connectionId: number) {
    startingOnlineConnectionId = connectionId;
    onlineImportError = undefined;
    fetchFailed = false;
    try {
      const result = await refreshImportConnection(connectionId, csrfToken);
      batchId = result.batch.id;
      step = 'fetching';
      pollFetchStatus();
    } catch (err) {
      onlineImportError = err;
    } finally {
      startingOnlineConnectionId = null;
    }
  }

  async function pollFetchStatus() {
    if (!batchId) return;
    try {
      const firstPage = await getImportBatch(batchId);
      const meta = parseBatchSourceMeta(firstPage.batch);
      if (meta.fetch_status === 'ready') {
        const result = firstPage.next_cursor ? await getFullImportBatch(batchId, firstPage) : firstPage;
        previewData = {
          batch: result.batch,
          rows: result.rows,
          warnings: result.warnings,
          meta: {
            account_hints: meta.account_hints ?? [],
            currency_hints: meta.currency_hints ?? [],
            date_from: meta.date_from,
            date_to: meta.date_to
          }
        };
        rowResolutions = new Map(result.rows.map((row) => [row.id, parseResolution(row)]));
        step = 'preview';
        return;
      }
      if (meta.fetch_status === 'failed') {
        fetchFailed = true;
        return;
      }
      pollTimer = setTimeout(pollFetchStatus, 2000);
    } catch (err) {
      onlineImportError = err;
      fetchFailed = true;
    }
  }

  async function handleCancelFetch() {
    if (pollTimer) {
      clearTimeout(pollTimer);
      pollTimer = null;
    }
    const idToDiscard = batchId;
    step = 'upload';
    batchId = null;
    fetchFailed = false;
    onlineImportError = undefined;
    if (idToDiscard) {
      // Best-effort: the batch may already be "failed" (discard would 409),
      // and there is no UI consequence either way once we've left the step.
      try {
        await discardImportBatch(idToDiscard, csrfToken);
      } catch {
        // ignore
      }
    }
  }

  onDestroy(() => {
    if (pollTimer) clearTimeout(pollTimer);
  });

  // ── Preview helpers ────────────────────────────────────────────────
  function getResolution(rowId: number): ImportResolution {
    return rowResolutions.get(rowId) ?? {};
  }

  function updateResolution(rowId: number, patch: Partial<ImportResolution>) {
    const existing = rowResolutions.get(rowId) ?? {};
    rowResolutions.set(rowId, { ...existing, ...patch });
    rowResolutions = new Map(rowResolutions); // trigger reactivity
  }

  function isTransferRow(row: ImportStagedRow): boolean {
    return !!parseNormalized(row).transfer_hint;
  }

  function applyGlobalAccount() {
    if (!previewData) return;
    for (const row of previewData.rows) {
      if (row.dedupe_status === 'excluded' || parseNormalized(row).investment) continue;
      if (isTransferRow(row)) {
        updateResolution(row.id, {
          account_id: globalAccountId,
          commodity_id: globalCommodityId,
          transfer_account_id: globalTransferAccountId
        });
      } else {
        updateResolution(row.id, {
          account_id: globalAccountId,
          commodity_id: globalCommodityId,
          category_id: globalCategoryId
        });
      }
    }
  }

  function toggleExclude(row: ImportStagedRow) {
    const res = getResolution(row.id);
    updateResolution(row.id, { exclude: !res.exclude });
  }

  function resolveImportPayee(rowIDs: number[], payee: PayeeResponse) {
    for (const rowID of rowIDs) {
      updateResolution(rowID, { payee_id: payee.id, payee_name: payee.name });
    }
  }

  // Parse warnings carry a stable code (#300); an unknown code falls back to
  // the backend's English message.
  function parseWarningText(w: ParseWarning): string {
    const value = w.value ?? '';
    switch (w.code) {
      case 'unrecognized_date': return m.import_warning_unrecognized_date({ value });
      case 'unrecognized_amount': return m.import_warning_unrecognized_amount({ value: maskFinancialText(value) });
      case 'unrecognized_split_amount': return m.import_warning_unrecognized_split_amount({ value: maskFinancialText(value) });
      case 'decimal_separator_conflict':
        return value ? m.import_warning_decimal_separator_conflict({ value }) : m.import_warning_decimal_separator_undecided();
      case 'date_layout_conflict': return m.import_warning_date_layout_conflict({ value });
      case 'date_layout_ambiguous': return m.import_warning_date_layout_ambiguous({ value });
    }
    return w.message === 'unsupported fill type; review required' ? m.import_preview_unsupported_fill() : w.message;
  }

  function dedupeStatusLabel(status: ImportStagedRow['dedupe_status']): string {
    switch (status) {
      case 'duplicate': return m.import_preview_dedupe_duplicate();
      case 'needs_attention': return m.import_preview_dedupe_needs_attention();
      case 'excluded': return m.import_preview_dedupe_excluded();
      default: return m.import_preview_dedupe_new();
    }
  }

  function appliedRuleSummary(resolution: ImportResolution): string {
    const tagNames = (resolution.tag_ids ?? []).map((id) => tags.find((tag) => tag.id === id)?.name ?? String(id));
    return tagNames.length > 0
      ? m.import_preview_rule_applied_with_tags({ name: resolution.applied_rule_name ?? '', tags: tagNames.join(', ') })
      : m.import_preview_rule_applied({ name: resolution.applied_rule_name ?? '' });
  }

  async function handleSourceCorrection(row: ImportStagedRow) {
    if (!batchId || !csrfToken || sourceCorrectionPending || !sourceCorrectionReason.trim()) return;
    const kind = sourceCorrectionKind(row);
    if (!kind) return;
    const rowId = row.id;
    sourceCorrectionPending = true;
    sourceCorrectionError = undefined;
    try {
      if (!sourceCorrectionImpact) {
        sourceCorrectionImpact = await (kind === 'sale' ? previewTrading212SaleCorrectionReconciliation : previewTrading212BuyCorrectionReconciliation)(batchId, rowId, {
          reason: sourceCorrectionReason.trim()
        });
        return;
      }
      if (sourceCorrectionImpact.affected_checkpoints.length > 0 && !sourceCorrectionOverride) return;
      if (sourceCorrectionGainRows.length > 0 && !sourceCorrectionAcceptGains) return;
      const acknowledgement = gainAcknowledgement(sourceCorrectionImpact.gain_impact);
      const result = await (kind === 'sale' ? correctTrading212Sale : correctTrading212Buy)(batchId, rowId, {
        reason: sourceCorrectionReason.trim(),
        // The override names the checkpoints the preview showed (#304).
        ...(sourceCorrectionOverride ? checkpointOverride(sourceCorrectionImpact.affected_checkpoints) : {}),
        ...(acknowledgement ? { gain_impact_acknowledgement: acknowledgement } : {})
      }, csrfToken);
      sourceCorrectionSuccessID = result.replacement.transaction.id;
      sourceCorrectionRowID = null;
      sourceCorrectionImpact = null;
      sourceCorrectionReason = '';
      if (previewData) {
        previewData = {
          ...previewData,
          rows: previewData.rows.map((row) => row.id === rowId
            ? { ...row, source_changed: false, commit_status: 'committed' }
            : row)
        };
      }
      await invalidateInvestmentReads(queryClient);
      try {
        const refreshed = await getFullImportBatch(batchId);
        if (previewData) previewData = { ...previewData, batch: refreshed.batch, rows: refreshed.rows, warnings: refreshed.warnings };
      } catch {
        // The correction is committed; keep its success link and local row state.
      }
    } catch (err) {
      // A stale gain acknowledgement or checkpoint set clears the preview; the
      // next click previews the current impact again.
      sourceCorrectionImpact = null;
      sourceCorrectionOverride = false;
      sourceCorrectionAcceptGains = false;
      sourceCorrectionError = err;
    } finally {
      sourceCorrectionPending = false;
    }
  }

  // A linked split or short row is committed evidence of a recorded operation;
  // refresh the batch so its status and duplicate protection show immediately
  // (T-122, #245).
  async function handleOperationLinked(rowId: number) {
    splitLinkRowID = null;
    shortLinkRowID = null;
    if (previewData) {
      previewData = {
        ...previewData,
        rows: previewData.rows.map((row) => row.id === rowId ? { ...row, commit_status: 'committed' } : row)
      };
    }
    if (!batchId) return;
    try {
      const refreshed = await getFullImportBatch(batchId);
      if (previewData) previewData = { ...previewData, batch: refreshed.batch, rows: refreshed.rows, warnings: refreshed.warnings };
    } catch {
      // The link is committed; keep the local row state.
    }
  }

  async function handleReviewChangedFills() {
    if (!batchId || !previewData) return;
    reviewChangedPending = true;
    reviewChangedError = undefined;
    try {
      const refreshed = await getFullImportBatch(batchId);
      previewData = { ...previewData, batch: refreshed.batch, rows: refreshed.rows, warnings: refreshed.warnings };
      step = 'preview';
    } catch (err) {
      reviewChangedError = err;
    } finally {
      reviewChangedPending = false;
    }
  }

  // ── Commit ─────────────────────────────────────────────────────────
  // A partially committed or failed batch resumes where it stopped: its
  // pending and failed rows can be re-resolved and committed again (#301).
  function batchReviewable(status: ImportBatch['status']): boolean {
    return status === 'previewing' || status === 'partially_committed' || status === 'failed';
  }

  function rowReviewable(row: ImportStagedRow): boolean {
    return row.commit_status === 'pending' || row.commit_status === 'failed';
  }

  function rowPatchChanged(row: ImportStagedRow): boolean {
    return JSON.stringify(getResolution(row.id)) !== JSON.stringify(parseResolution(row));
  }

  async function handleCommit() {
    if (!batchId || !previewData) return;
    committing = true;
    commitError = undefined;

    try {
      await savePendingResolutions();
      const preview = await previewCommitImportBatch(batchId);
      if (reconciliationOverride && !sameCheckpoints(reviewedCheckpoints, preview.reconciliation_checkpoints)) {
        // The owner has not seen this set yet: show it and wait for another
        // click rather than invalidate checkpoints nobody reviewed.
        reviewedCheckpointsChanged = reviewedCheckpoints !== null;
        reviewedCheckpoints = preview.reconciliation_checkpoints;
        return;
      }
      if (await reviewImportGains(false, preview)) return;
      await finishCommit([]);
    } catch (err) {
      commitError = err;
    } finally {
      committing = false;
    }
  }

  // Patch resolutions to the server before anything is previewed or
  // committed. A resumed batch patches only the rows the owner changed, so
  // rows reviewed elsewhere (dividends) keep their stored review.
  async function savePendingResolutions() {
    if (!batchId || !previewData) return;
    const resumed = previewData.batch.status !== 'previewing';
    const patches = previewData.rows
      .filter((row) => rowReviewable(row) && (!resumed || rowPatchChanged(row)))
      .map((row) => ({
        row_id: row.id,
        dedupe_status: getResolution(row.id).exclude ? 'excluded' : row.dedupe_status,
        resolution: getResolution(row.id)
      }));
    if (patches.length > 0) await patchImportBatch(batchId, patches, csrfToken);
  }

  function sameCheckpoints(reviewed: readonly BatchCheckpoint[] | null, current: readonly BatchCheckpoint[]): boolean {
    if (!reviewed || reviewed.length !== current.length) return false;
    const ids = new Set(reviewed.map((checkpoint) => checkpoint.checkpoint_id));
    return current.every((checkpoint) => ids.has(checkpoint.checkpoint_id));
  }

  // Checking the override saves the resolutions and lists the checkpoints they
  // reach. Commit saves and compares again, so an edit made since is shown
  // before anything is invalidated.
  async function handleReconciliationOverrideChange() {
    reviewedCheckpoints = null;
    reviewedCheckpointsChanged = false;
    reviewedCheckpointsError = undefined;
    if (!reconciliationOverride || !batchId) return;
    reviewedCheckpointsPending = true;
    try {
      await savePendingResolutions();
      reviewedCheckpoints = (await previewCommitImportBatch(batchId)).reconciliation_checkpoints;
    } catch (err) {
      reviewedCheckpointsError = err;
    } finally {
      reviewedCheckpointsPending = false;
    }
  }

  // Preview gain changes from imported acquisitions against the current
  // ledger; any change goes to the user before the batch commits.
  async function reviewImportGains(refreshed: boolean, current?: PreviewCommitResponse): Promise<boolean> {
    if (!batchId) return false;
    const preview = current ?? await previewCommitImportBatch(batchId);
    if (preview.gain_impacts.length === 0) return false;
    gainReview = {
      rows: preview.gain_impacts.flatMap((row) => gainImpactRows(row.gain_impact.changes, gainCurrency, getLocale())
        .map((gain) => ({ ...gain, key: `${row.row_id}:${gain.key}` }))),
      acknowledgements: preview.gain_impacts.map((row) => ({
        row_id: row.row_id, acknowledgement: row.gain_impact.acknowledgement
      })),
      refreshed
    };
    return true;
  }

  async function confirmImportGains() {
    if (!gainReview) return;
    const { acknowledgements } = gainReview;
    gainReview = null;
    committing = true;
    commitError = undefined;
    try {
      await finishCommit(acknowledgements);
    } catch (err) {
      commitError = err;
    } finally {
      committing = false;
    }
  }

  async function finishCommit(acknowledgements: { row_id: number; acknowledgement: string }[]) {
    if (!batchId) return;
    const result = await commitImportBatch(
      batchId,
      {
        ...(reconciliationOverride && reviewedCheckpoints
          ? { reconciliation_override: true, reconciliation_override_checkpoint_ids: reviewedCheckpoints.map((checkpoint) => checkpoint.checkpoint_id) }
          : {}),
        gain_impact_acknowledgements: acknowledgements
      },
      csrfToken
    );
    commitResult = result;
    // An override covers one reviewed commit; held rows are reviewed afresh.
    reconciliationOverride = false;
    reviewedCheckpoints = null;
    reviewedCheckpointsChanged = false;
    await invalidateInvestmentReads(queryClient);
    step = 'result';
  }

  // Held rows stay pending in a partially committed batch. An earlier row in
  // the same run can change their gain set, so they are reviewed again here.
  async function handleReviewHeldGains() {
    if (!batchId) return;
    committing = true;
    commitError = undefined;
    try {
      if (!(await reviewImportGains(true))) await finishCommit([]);
    } catch (err) {
      commitError = err;
    } finally {
      committing = false;
    }
  }

  // Back to the preview with the batch's current rows and commit controls,
  // so held and failed rows can be fixed and committed in the same batch.
  async function handleReviewRemainingRows() {
    if (!batchId || !previewData) return;
    reviewChangedPending = true;
    reviewRemainingError = undefined;
    try {
      const refreshed = await getFullImportBatch(batchId);
      previewData = { ...previewData, batch: refreshed.batch, rows: refreshed.rows, warnings: refreshed.warnings };
      rowResolutions = new Map(refreshed.rows.map((row) => [row.id, parseResolution(row)]));
      commitError = undefined;
      step = 'preview';
    } catch (err) {
      reviewRemainingError = err;
    } finally {
      reviewChangedPending = false;
    }
  }

  // ── Discard ────────────────────────────────────────────────────────
  async function handleDiscard() {
    if (!batchId) return;
    discarding = true;
    discardError = undefined;

    try {
      await discardImportBatch(batchId, csrfToken);
      // Reset to upload step.
      step = 'upload';
      previewData = null;
      batchId = null;
      selectedFile = null;
      showDiscardConfirm = false;
      sourceCorrectionRowID = null;
      sourceCorrectionImpact = null;
      sourceCorrectionSuccessID = null;
      sourceCorrectionError = undefined;
    } catch (err) {
      discardError = err;
    } finally {
      discarding = false;
    }
  }

  function handleImportAnother() {
    step = 'upload';
    previewData = null;
    batchId = null;
    selectedFile = null;
    commitResult = null;
    reviewRemainingError = undefined;
    sourceCorrectionRowID = null;
    sourceCorrectionImpact = null;
    sourceCorrectionReason = '';
    sourceCorrectionOverride = false;
    sourceCorrectionSuccessID = null;
    sourceCorrectionError = undefined;
  }

  // ── Connections ────────────────────────────────────────────────────
  const queryClient = useQueryClient();

  const connectionsQuery = createQuery(() => ({
    queryKey: importConnectionsQueryKey,
    queryFn: () => listImportConnections(),
    retry: false
  }));

  const connections = $derived(connectionsQuery.data?.connections ?? []);
  const connectionsConfigError = $derived(
    connectionsQuery.isError && (connectionsQuery.error as { code?: string })?.code === 'CONFIG_REQUIRED'
  );

  // Add-connection form
  let showAddConnection = $state(false);
  let newConnName = $state('');
  let newConnKey = $state('');
  let newConnCashAccountId = $state('');
  let addingConnection = $state(false);
  let addConnectionError = $state<unknown>(undefined);

  // Delete connection
  let deletingConnectionId = $state<number | null>(null);
  let confirmDeleteConnectionId = $state<number | null>(null);
  let deleteConnectionError = $state<unknown>(undefined);

  // Auto-refresh toggle
  let togglingAutoRefreshId = $state<number | null>(null);
  let autoRefreshError = $state<unknown>(undefined);

  // Cash account picker
  let updatingCashAccountId = $state<number | null>(null);
  let cashAccountError = $state<unknown>(undefined);

  const postableAccounts = $derived(
    accounts.filter((a) => a.allows_postings && a.status === 'active' && a.account_class === 'asset' && !a.is_system)
  );

  async function handleAddConnection() {
    if (!newConnName.trim() || !newConnKey.trim()) return;
    addingConnection = true;
    addConnectionError = undefined;
    try {
      await createImportConnection(
        {
          source: 'trading212',
          display_name: newConnName.trim(),
          api_key: newConnKey.trim(),
          ...(newConnCashAccountId ? { cash_account_id: Number(newConnCashAccountId) } : {})
        },
        csrfToken
      );
      await queryClient.invalidateQueries({ queryKey: importConnectionsQueryKey });
      showAddConnection = false;
      newConnName = '';
      newConnKey = '';
      newConnCashAccountId = '';
    } catch (err) {
      addConnectionError = err;
    } finally {
      addingConnection = false;
    }
  }

  async function handleSetCashAccount(conn: ImportConnection, accountId: string) {
    if (!accountId) return;
    updatingCashAccountId = conn.id;
    cashAccountError = undefined;
    try {
      await updateImportConnection(
        conn.id,
        {
          display_name: conn.display_name,
          config: conn.config,
          cash_account_id: Number(accountId)
        },
        csrfToken
      );
      await queryClient.invalidateQueries({ queryKey: importConnectionsQueryKey });
    } catch (err) {
      cashAccountError = err;
    } finally {
      updatingCashAccountId = null;
    }
  }

  async function handleDeleteConnection(id: number) {
    deletingConnectionId = id;
    deleteConnectionError = undefined;
    try {
      await deleteImportConnection(id, csrfToken);
      await queryClient.invalidateQueries({ queryKey: importConnectionsQueryKey });
      confirmDeleteConnectionId = null;
    } catch (err) {
      deleteConnectionError = err;
    } finally {
      deletingConnectionId = null;
    }
  }

  async function handleToggleAutoRefresh(conn: ImportConnection) {
    togglingAutoRefreshId = conn.id;
    autoRefreshError = undefined;
    try {
      await updateImportConnection(
        conn.id,
        {
          display_name: conn.display_name,
          config: conn.config,
          auto_refresh_enabled: !conn.auto_refresh_enabled
        },
        csrfToken
      );
      await queryClient.invalidateQueries({ queryKey: importConnectionsQueryKey });
    } catch (err) {
      autoRefreshError = err;
    } finally {
      togglingAutoRefreshId = null;
    }
  }

  function fetchStatusLabel(conn: ImportConnection): string {
    if (!conn.last_fetch_status) return m.import_connections_status_never();
    if (conn.last_fetch_status === 'fetching') return m.import_connections_status_fetching();
    if (conn.last_fetch_status === 'ready') return m.import_connections_status_ready();
    if (conn.last_fetch_status === 'failed') return m.import_connections_status_failed();
    return m.import_connections_status_never();
  }
</script>

<!-- Upload step -->
{#if step === 'upload'}
  <div class="max-w-2xl space-y-6">
    <a href="/app/ledger/import/dividends" class="inline-flex min-h-11 items-center text-sm font-semibold text-foreground underline underline-offset-2">{m.import_dividend_link()}</a>
    <Panel>
      <p class="text-sm font-semibold text-foreground">{m.import_upload_title()}</p>
      <p class="mt-2 text-sm leading-6 text-muted">{m.import_upload_copy()}</p>

      <div class="mt-5 space-y-4">
        <div>
          <label class="mb-4 flex min-h-11 items-center gap-2 text-sm"><input type="checkbox" bind:checked={cryptoCSV} onchange={() => { selectedFile = null; csvAnalysis = null; analyzingCSV = false; csvAnalysisRequest++; }} />{m.quotes_csv()}</label>
          {#if cryptoCSV}
            <p class="mb-3 text-sm text-muted">{m.quotes_csv_help()}</p>
            {#if cryptoInstrumentsQuery.isPending || accountsQuery.isPending}<p role="status">{m.investments_loading()}</p>{/if}
            {#if cryptoInstrumentsQuery.isError}<APIFormError error={cryptoInstrumentsQuery.error} />{/if}
            {#if accountsQuery.isError}<APIFormError error={accountsQuery.error} />{/if}
            <div class="mb-4 grid gap-3 sm:grid-cols-2">
              <label class="grid gap-1 text-sm">{m.investments_form_holding_account()}<select bind:value={cryptoHoldingId} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3"><option value={0}>{m.pricing_choose()}</option>{#each accounts.filter(a => a.account_kind === 'security_holding' && a.status === 'active' && cryptoCommodityIDs.has(a.default_commodity_id ?? 0)) as account}<option value={account.id}>{account.name}</option>{/each}</select></label>
              <label class="grid gap-1 text-sm">{m.investments_form_cash_account()}<select bind:value={cryptoCashId} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3"><option value={0}>{m.pricing_choose()}</option>{#each accounts.filter(a => a.account_class === 'asset' && a.status === 'active' && a.account_kind !== 'security_holding' && a.account_kind !== 'fund_holding') as account}<option value={account.id}>{account.name}</option>{/each}</select></label>
            </div>
            <pre class="mb-4 overflow-x-auto rounded-(--radius-control) border border-border p-3 text-xs">date,side,quantity,amount,external_ref,memo</pre>
          {/if}
          <label
            class="inline-flex cursor-pointer items-center gap-2 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
          >
            <Upload size={16} aria-hidden="true" />
            {m.import_upload_choose_file()}
            <input type="file" accept=".qif,.csv,text/csv" class="sr-only" onchange={handleFileChange} />
          </label>
          <span class="ml-3 text-sm text-muted">
            {selectedFile ? selectedFile.name : m.import_upload_no_file()}
          </span>
        </div>

        {#if selectedIsTextImport}
          <label class="flex max-w-md flex-col gap-1.5 text-xs font-medium text-muted">
            {m.import_upload_encoding_label()}
            <select class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" value={textEncoding} onchange={handleTextEncodingChange}>
              {#each textEncodingOptions as option}
                <option value={option.value}>{option.value === 'auto' ? m.import_upload_encoding_auto() : option.label}</option>
              {/each}
            </select>
            <span class="font-normal leading-5">{m.import_upload_encoding_help()}</span>
            {#if selectedIsCSV && csvAnalysis?.text_encoding}
              <span class="font-normal leading-5">
                {csvAnalysis.encoding_source === 'detected'
                  ? m.import_preview_encoding_detected({ encoding: csvAnalysis.text_encoding, confidence: csvAnalysis.encoding_confidence })
                  : m.import_preview_encoding_used({ encoding: csvAnalysis.text_encoding })}
              </span>
            {/if}
          </label>
        {/if}

        {#if selectedIsCSV}
          <fieldset class="space-y-4 rounded-(--radius-control) border border-border p-4">
            <legend class="px-1 text-sm font-semibold text-foreground">{m.import_csv_mapping_title()}</legend>
            <p class="text-sm text-muted">{m.import_csv_mapping_copy()}</p>
            {#if analyzingCSV}
              <p class="flex items-center gap-2 text-sm text-muted"><Loader size={14} class="animate-spin" aria-hidden="true" />{m.import_csv_analysis_loading()}</p>
            {:else if csvAnalysisError}
              {#if csvAnalysisError instanceof APIClientError && csvAnalysisError.code === 'IMPORT_UPLOAD_TOO_LARGE'}
                <APIFormError error={csvAnalysisError} id="csv-analysis-error" />
              {:else}
                <p class="text-sm text-warning" role="alert">{m.import_csv_analysis_error()}</p>
              {/if}
            {:else if profilesQuery.isLoading}
              <p class="text-sm text-muted">{m.import_csv_profiles_loading()}</p>
            {:else if profilesQuery.isError}
              <p class="text-sm text-warning">{m.import_csv_profiles_error()}</p>
            {:else}
              {#key orderedCSVProfiles.map((profile) => `${profile.id}:${profile.updated_at}`).join(',')}
                <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">
                  {m.import_csv_saved_profile()}
                  <select class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={selectedProfileId} onchange={handleProfileSelection}>
                    <option value="" selected={!selectedProfileId}>{m.import_csv_new_profile()}</option>
                    {#each orderedCSVProfiles as profile (profile.id)}
                      <option value={String(profile.id)} selected={selectedProfileId === String(profile.id)}>
                        {suggestedCSVProfile?.id === profile.id ? m.import_csv_profile_suggested({ name: profile.name }) : profile.name}
                      </option>
                    {/each}
                  </select>
                </label>
              {/key}

              {#if selectedProfileId && !editingProfileId}
                <div class="flex flex-wrap items-center gap-2">
                  <button type="button" class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-medium text-foreground hover:bg-control-hover" onclick={beginProfileEdit}>
                    {m.import_csv_profile_edit()}
                  </button>
                  {#if confirmDeleteProfileId === Number(selectedProfileId)}
                    <span class="text-sm text-muted">{m.import_csv_profile_delete_confirm()}</span>
                    <button type="button" class="text-sm text-muted hover:text-foreground" onclick={() => { confirmDeleteProfileId = null; profileMutationError = undefined; }}>
                      {m.import_csv_profile_delete_cancel()}
                    </button>
                    <button type="button" class="rounded-(--radius-control) bg-foreground px-3 py-2 text-sm font-semibold text-background disabled:opacity-60" onclick={handleDeleteProfile} disabled={deletingProfileId === Number(selectedProfileId)}>
                      {m.import_csv_profile_delete_confirm_button()}
                    </button>
                  {:else}
                    <button type="button" class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-medium text-muted hover:bg-control-hover hover:text-foreground" onclick={() => { confirmDeleteProfileId = Number(selectedProfileId); profileMutationError = undefined; }}>
                      {m.import_csv_profile_delete()}
                    </button>
                  {/if}
                </div>
              {/if}

              {#if !selectedProfileId || editingProfileId}
                <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">
                  {m.import_csv_profile_name()}
                  <input class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={profileName} />
                </label>
                <div class="grid gap-4 sm:grid-cols-2">
                  <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">
                    {m.import_csv_delimiter()}
                    <select class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" value={csvDelimiter} onchange={handleCSVDelimiterChange}>
                      <option value="comma">{m.import_csv_delimiter_comma()}</option>
                      <option value="semicolon">{m.import_csv_delimiter_semicolon()}</option>
                      <option value="tab">{m.import_csv_delimiter_tab()}</option>
                    </select>
                  </label>
                  <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">
                    {m.import_csv_date_column()}
                    <select class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={csvDateColumn}>
                      {#each csvHeaders as header}<option value={header}>{header}</option>{/each}
                    </select>
                  </label>
                  <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">
                    {m.import_csv_payee_column()}
                    <select class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={csvPayeeColumn}>
                      <option value="">{m.import_csv_column_none()}</option>
                      {#each csvHeaders as header}<option value={header}>{header}</option>{/each}
                    </select>
                  </label>
                  <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">
                    {m.import_csv_memo_column()}
                    <select class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={csvMemoColumn}>
                      <option value="">{m.import_csv_column_none()}</option>
                      {#each csvHeaders as header}<option value={header}>{header}</option>{/each}
                    </select>
                  </label>
                  <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">
                    {m.import_csv_category_column()}
                    <select class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={csvCategoryColumn}>
                      <option value="">{m.import_csv_column_none()}</option>
                      {#each csvHeaders as header}<option value={header}>{header}</option>{/each}
                    </select>
                  </label>
                  <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">
                    {m.import_csv_external_ref_column()}
                    <select class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={csvExternalRefColumn}>
                      <option value="">{m.import_csv_column_none()}</option>
                      {#each csvHeaders as header}<option value={header}>{header}</option>{/each}
                    </select>
                  </label>
                  <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">
                    {m.import_csv_amount_layout()}
                    <select class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={csvAmountMode}>
                      <option value="single">{m.import_csv_amount_single()}</option>
                      <option value="debit_credit">{m.import_csv_amount_debit_credit()}</option>
                    </select>
                  </label>
                  {#if csvAmountMode === 'single'}
                    <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">
                      {m.import_csv_amount_column()}
                      <select class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={csvAmountColumn}>
                        {#each csvHeaders as header}<option value={header}>{header}</option>{/each}
                      </select>
                    </label>
                  {:else}
                    <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">{m.import_csv_debit_column()}<select class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={csvDebitColumn}>{#each csvHeaders as header}<option value={header}>{header}</option>{/each}</select></label>
                    <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">{m.import_csv_credit_column()}<select class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={csvCreditColumn}>{#each csvHeaders as header}<option value={header}>{header}</option>{/each}</select></label>
                  {/if}
                  <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">{m.import_csv_date_layout()}<select class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={csvDateLayout}><option value="DMY">DD/MM/YYYY</option><option value="MDY">MM/DD/YYYY</option><option value="YMD">YYYY-MM-DD</option></select></label>
                  <label class="flex flex-col gap-1.5 text-xs font-medium text-muted">{m.import_csv_decimal_separator()}<select class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" bind:value={csvDecimalSeparator}><option value=",">1.234,56</option><option value=".">1,234.56</option></select></label>
                </div>
                <label class="flex items-center gap-2 text-sm text-foreground"><input type="checkbox" bind:checked={csvInvertAmount} />{m.import_csv_invert_amount()}</label>
                <CSVInvestmentMappingForm bind:value={csvInvestment} headers={csvHeaders} {accounts} />
                {#if editingProfileId}
                  <div class="flex items-center gap-3">
                    <button type="button" class="rounded-(--radius-control) bg-foreground px-3 py-2 text-sm font-semibold text-background disabled:opacity-60" onclick={saveProfileEdit} disabled={savingProfile || !csvMappingValid}>
                      {savingProfile ? m.import_csv_profile_saving() : m.import_csv_profile_save()}
                    </button>
                    <button type="button" class="text-sm text-muted hover:text-foreground" onclick={() => { editingProfileId = null; profileMutationError = undefined; }}>
                      {m.import_csv_profile_edit_cancel()}
                    </button>
                  </div>
                {/if}
              {/if}
              <APIFormError error={profileMutationError} id="profile-error" />
            {/if}
          </fieldset>
        {/if}

        <APIFormError error={uploadError} id="upload-error" />

        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
          onclick={handleUpload}
          disabled={!selectedFile || (cryptoCSV && (!cryptoHoldingId || !cryptoCashId)) || uploading || analyzingCSV || editingProfileId !== null || (selectedIsCSV && (!!csvAnalysisError || csvHeaders.length === 0 || (!selectedProfileId && !newCSVProfileValid)))}
        >
          {uploading ? m.import_upload_submitting() : m.import_upload_submit()}
        </button>
      </div>
    </Panel>

    <ImportRulesPanel {csrfToken} />

    <!-- MS Money help panel -->
    <Panel variant="subtle">
      <p class="text-sm font-semibold text-foreground">{m.import_upload_ms_money_help()}</p>
      <p class="mt-2 text-sm leading-6 text-muted">{m.import_upload_ms_money_steps()}</p>
    </Panel>

    <!-- Online connections panel -->
    <Panel>
      <div class="flex flex-wrap items-center justify-between gap-4">
        <div class="min-w-0">
          <p class="text-sm font-semibold text-foreground">{m.import_connections_title()}</p>
          <p class="mt-1 text-sm text-muted">{m.import_connections_copy()}</p>
        </div>
        <button
          type="button"
          class="inline-flex shrink-0 items-center gap-1.5 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-medium text-foreground transition hover:bg-control-hover"
          onclick={() => { showAddConnection = !showAddConnection; addConnectionError = undefined; }}
        >
          <Plus size={14} aria-hidden="true" />
          {m.import_connections_add()}
        </button>
      </div>

      {#if connectionsConfigError}
        <p role="alert" class="mt-4 text-sm text-warning">{m.import_connections_error_config()}</p>
      {:else if connectionsQuery.isError}
        <p role="alert" class="mt-4 text-sm text-warning">{m.import_connections_load_error()}</p>
      {:else if connections.length === 0 && !showAddConnection}
        <p class="mt-4 text-sm text-muted">{m.import_connections_empty()}</p>
      {:else if connections.length > 0}
        <p class="mt-3 text-sm leading-6 text-muted">{m.import_connections_refetch_help()}</p>
        <div class="mt-4 overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-border text-left">
                <th class="pb-2 pr-4 font-semibold text-muted">{m.import_connections_col_name()}</th>
                <th class="pb-2 pr-4 font-semibold text-muted">{m.import_connections_col_source()}</th>
                <th class="pb-2 pr-4 font-semibold text-muted">{m.import_connections_col_key()}</th>
                <th class="pb-2 pr-4 font-semibold text-muted">{m.import_connections_col_status()}</th>
                <th class="pb-2 pr-4 font-semibold text-muted">{m.import_connections_col_auto_refresh()}</th>
                <th class="pb-2 pr-4 font-semibold text-muted">{m.import_connections_col_cash_account()}</th>
                <th class="pb-2 font-semibold text-muted">{m.import_connections_col_actions()}</th>
              </tr>
            </thead>
            <tbody>
              {#each connections as conn (conn.id)}
                <tr class="border-b border-border last:border-b-0">
                  <td class="py-2.5 pr-4 font-medium text-foreground">{conn.display_name}</td>
                  <td class="py-2.5 pr-4 text-muted">{conn.source}</td>
                  <td class="py-2.5 pr-4 font-mono text-xs text-muted">{conn.key_hint}</td>
                  <td class="py-2.5 pr-4 text-muted">{fetchStatusLabel(conn)}</td>
                  <td class="py-2.5 pr-4">
                    <button
                      type="button"
                      role="switch"
                      aria-checked={conn.auto_refresh_enabled}
                      title={conn.auto_refresh_enabled
                        ? m.import_connections_auto_refresh_on()
                        : m.import_connections_auto_refresh_off()}
                      class="relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition disabled:cursor-not-allowed disabled:opacity-60 {conn.auto_refresh_enabled
                        ? 'bg-foreground'
                        : 'bg-control'} border border-border"
                      onclick={() => handleToggleAutoRefresh(conn)}
                      disabled={togglingAutoRefreshId === conn.id}
                    >
                      <span
                        class="inline-block h-3.5 w-3.5 transform rounded-full bg-background transition {conn.auto_refresh_enabled
                          ? 'translate-x-4'
                          : 'translate-x-1'}"
                      ></span>
                    </button>
                  </td>
                  <td class="py-2.5 pr-4">
                    <select
                      aria-label={m.import_connections_col_cash_account()}
                      class="rounded-(--radius-control) border border-border bg-control px-2 py-1.5 text-xs focus:outline-none focus:ring-2 focus:ring-foreground disabled:cursor-not-allowed disabled:opacity-60"
                      value={conn.cash_account_id ?? ''}
                      disabled={updatingCashAccountId === conn.id}
                      onchange={(e) => handleSetCashAccount(conn, e.currentTarget.value)}
                    >
                      <option value="" disabled>{m.import_connections_cash_account_placeholder()}</option>
                      {#each postableAccounts as account (account.id)}
                        <option value={account.id}>{account.name}</option>
                      {/each}
                    </select>
                  </td>
                  <td class="py-2.5">
                    <div class="flex flex-wrap items-center gap-2">
                    {#if confirmDeleteConnectionId !== conn.id}
                      <button
                        type="button"
                        class="inline-flex items-center gap-1 rounded-(--radius-control) border border-border px-2.5 py-1 text-xs font-medium text-foreground transition hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-60"
                        onclick={() =>
                          conn.last_fetch_status
                            ? handleRefreshConnection(conn.id)
                            : handleStartOnlineImport(conn.id)}
                        disabled={startingOnlineConnectionId !== null || conn.last_fetch_status === 'fetching'}
                      >
                        {#if startingOnlineConnectionId === conn.id}
                          {m.import_connections_starting()}
                        {:else if conn.last_fetch_status}
                          {m.import_connections_refresh()}
                        {:else}
                          {m.import_connections_import()}
                        {/if}
                      </button>
                      {#if conn.last_fetch_status}
                        <button
                          type="button"
                          class="inline-flex items-center gap-1 rounded-(--radius-control) border border-border px-2.5 py-1 text-xs font-medium text-foreground transition hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-60"
                          onclick={() => handleStartOnlineImport(conn.id)}
                          disabled={startingOnlineConnectionId !== null || conn.last_fetch_status === 'fetching'}
                        >
                          {m.import_connections_refetch()}
                        </button>
                      {/if}
                    {/if}
                    {#if confirmDeleteConnectionId === conn.id}
                      <div class="flex items-center gap-2">
                        <span class="text-xs text-muted">{m.import_connections_delete_confirm()}</span>
                        <button
                          type="button"
                          class="text-xs text-muted hover:text-foreground"
                          onclick={() => { confirmDeleteConnectionId = null; deleteConnectionError = undefined; }}
                        >
                          {m.import_connections_delete_cancel()}
                        </button>
                        <button
                          type="button"
                          class="inline-flex items-center gap-1 rounded-(--radius-control) bg-foreground px-2.5 py-1 text-xs font-semibold text-background transition hover:opacity-90 disabled:opacity-60"
                          onclick={() => handleDeleteConnection(conn.id)}
                          disabled={deletingConnectionId === conn.id}
                        >
                          <Trash2 size={12} aria-hidden="true" />
                          {m.import_connections_delete_confirm_button()}
                        </button>
                      </div>
                    {:else}
                      <button
                        type="button"
                        class="inline-flex items-center gap-1 rounded-(--radius-control) border border-border px-2.5 py-1 text-xs font-medium text-muted transition hover:bg-control-hover hover:text-foreground"
                        onclick={() => { confirmDeleteConnectionId = conn.id; deleteConnectionError = undefined; }}
                      >
                        <Trash2 size={12} aria-hidden="true" />
                        {m.import_connections_delete()}
                      </button>
                    {/if}
                    </div>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}

      {#if onlineImportError}
        <p class="mt-3 text-sm text-warning">{m.import_connections_fetch_error()}</p>
      {/if}

      {#if deleteConnectionError}
        <p class="mt-3 text-sm text-warning">{m.import_connections_delete_error()}</p>
      {/if}

      {#if autoRefreshError}
        <p class="mt-3 text-sm text-warning">{m.import_connections_auto_refresh_error()}</p>
      {/if}

      {#if cashAccountError}
        <p class="mt-3 text-sm text-warning">{m.import_connections_cash_account_error()}</p>
      {/if}

      <!-- Add connection form -->
      {#if showAddConnection}
        <div class="mt-5 border-t border-border pt-5">
          <p class="text-sm font-semibold text-foreground">{m.import_connections_add_title()}</p>
          <div class="mt-4 space-y-4">
            <div class="flex flex-col gap-1.5">
              <label class="text-xs font-medium text-muted" for="conn-name">
                {m.import_connections_add_name_label()}
              </label>
              <input
                id="conn-name"
                type="text"
                class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-foreground"
                placeholder={m.import_connections_add_name_placeholder()}
                bind:value={newConnName}
              />
            </div>
            <div class="flex flex-col gap-1.5">
              <label class="text-xs font-medium text-muted" for="conn-key">
                {m.import_connections_add_key_label()}
              </label>
              <input
                id="conn-key"
                type="password"
                autocomplete="new-password"
                class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-foreground"
                placeholder={m.import_connections_add_key_placeholder()}
                bind:value={newConnKey}
              />
              <p class="text-xs text-muted">{m.import_connections_add_key_help()}</p>
            </div>
            <div class="flex flex-col gap-1.5">
              <label class="text-xs font-medium text-muted" for="conn-cash-account">
                {m.import_connections_add_cash_account_label()}
              </label>
              <select
                id="conn-cash-account"
                class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-foreground"
                bind:value={newConnCashAccountId}
              >
                <option value="">{m.import_connections_add_cash_account_none()}</option>
                {#each postableAccounts as account (account.id)}
                  <option value={account.id}>{account.name}</option>
                {/each}
              </select>
              <p class="text-xs text-muted">{m.import_connections_add_cash_account_help()}</p>
            </div>

            {#if addConnectionError}
              {@const errCode = (addConnectionError as { code?: string })?.code}
              <p class="text-sm text-warning">
                {#if errCode === 'PROVIDER_ERROR'}
                  {m.import_connections_error_provider()}
                {:else if errCode === 'CONFIG_REQUIRED'}
                  {m.import_connections_error_config()}
                {:else if errCode === 'CONFLICT'}
                  {m.import_connections_error_duplicate()}
                {:else}
                  {m.import_connections_error_generic()}
                {/if}
              </p>
            {/if}

            <div class="flex items-center gap-3">
              <button
                type="button"
                class="inline-flex items-center gap-2 rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
                onclick={handleAddConnection}
                disabled={addingConnection || !newConnName.trim() || !newConnKey.trim()}
              >
                {addingConnection ? m.import_connections_add_submitting() : m.import_connections_add_submit()}
              </button>
              <button
                type="button"
                class="text-sm text-muted hover:text-foreground"
                onclick={() => { showAddConnection = false; addConnectionError = undefined; newConnName = ''; newConnKey = ''; newConnCashAccountId = ''; }}
              >
                {m.import_discard_cancel()}
              </button>
            </div>
          </div>
        </div>
      {/if}
    </Panel>
  </div>

<!-- Fetching step (online import in progress) -->
{:else if step === 'fetching'}
  <div class="max-w-2xl space-y-6">
    <Panel>
      {#if fetchFailed}
        <div class="flex items-center gap-2">
          <AlertCircle size={20} class="text-warning shrink-0" aria-hidden="true" />
          <p class="text-sm font-semibold text-foreground">{m.import_fetching_failed_title()}</p>
        </div>
        <p class="mt-2 text-sm text-muted">{m.import_fetching_failed_copy()}</p>
      {:else}
        <div class="flex items-center gap-2">
          <Loader size={20} class="shrink-0 animate-spin text-muted" aria-hidden="true" />
          <p class="text-sm font-semibold text-foreground">{m.import_fetching_title()}</p>
        </div>
        <p class="mt-2 text-sm leading-6 text-muted">{m.import_fetching_copy()}</p>
      {/if}

      <div class="mt-5">
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
          onclick={handleCancelFetch}
        >
          {m.import_fetching_cancel()}
        </button>
      </div>
    </Panel>
  </div>

<!-- Preview step -->
{:else if step === 'preview' && previewData}
  <div class="space-y-6">
    {#if previewData.batch.source_kind === 'trading212' && previewData.rows.some(row => {
      try { return JSON.parse(row.raw)?.kind === 'trading212_dividend'; } catch { return false; }
    })}
      <Panel><a href={`/app/ledger/import/dividends?batch_id=${batchId}`} class="inline-flex min-h-11 items-center text-sm font-semibold text-foreground underline underline-offset-2">{m.import_dividend_link()}</a><p class="text-sm text-muted">{m.import_dividend_help()}</p></Panel>
    {/if}
    <!-- Warnings -->
    {#if previewData.warnings.length > 0}
      <Panel>
        <div class="flex items-center gap-2">
          <AlertCircle size={16} class="text-warning shrink-0" aria-hidden="true" />
          <p class="text-sm font-semibold text-foreground">{m.import_preview_warnings_title()}</p>
        </div>
        <ul class="mt-3 space-y-1">
          {#each previewData.warnings as w}
            <li class="text-sm text-muted">{w.row_index < 0
              ? parseWarningText(w)
              : m.import_preview_warning_row({ row: w.row_index + 1, message: parseWarningText(w) })}</li>
          {/each}
        </ul>
      </Panel>
    {/if}

    <!-- Date range summary -->
    {#if previewData.meta.date_from || previewData.meta.date_to}
      <p class="text-sm text-muted">
        {m.import_preview_date_range({ from: previewData.meta.date_from ?? '?', to: previewData.meta.date_to ?? '?' })}
      </p>
    {/if}

    {#if previewData.meta.text_encoding}
      <p class="text-sm text-muted">
        {previewData.meta.encoding_source === 'detected'
          ? m.import_preview_encoding_detected({ encoding: previewData.meta.text_encoding, confidence: previewData.meta.encoding_confidence ?? 0 })
          : m.import_preview_encoding_used({ encoding: previewData.meta.text_encoding })}
      </p>
    {/if}

    {#if previewData.batch.source_kind !== 'crypto_csv'}
    <PayeeResolutionPanel
      rows={previewData.rows}
      resolutions={rowResolutions}
      {csrfToken}
      onresolve={resolveImportPayee}
    />

    <!-- Global account/currency/category assignment -->
    <Panel>
      <p class="text-sm font-semibold text-foreground">{m.import_preview_apply_all()}</p>
      <div class="mt-3 flex flex-wrap gap-3">
        <div class="flex min-w-0 max-w-full flex-col gap-1">
          <label class="text-xs font-medium text-muted" for="global-account">
            {m.import_preview_account_label()}
          </label>
          <select
            id="global-account"
            class="min-w-0 max-w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-foreground"
            bind:value={globalAccountId}
          >
            <option value={undefined}>{m.import_preview_account_placeholder()}</option>
            {#each accounts as account}
              <option value={account.id}>{account.name ?? account.code}</option>
            {/each}
          </select>
        </div>

        <div class="flex min-w-0 max-w-full flex-col gap-1">
          <label class="text-xs font-medium text-muted" for="global-currency">
            {m.import_preview_currency_label()}
          </label>
          <select
            id="global-currency"
            class="min-w-0 max-w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-foreground"
            bind:value={globalCommodityId}
          >
            <option value={undefined}>{m.import_preview_currency_placeholder()}</option>
            {#each currencies as currency}
              <option value={currency.id}>{currency.code}</option>
            {/each}
          </select>
        </div>

        <div class="flex min-w-0 max-w-full flex-col gap-1">
          <label class="text-xs font-medium text-muted" for="global-category">
            {m.import_preview_category_label()}
          </label>
          <select
            id="global-category"
            class="min-w-0 max-w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-foreground"
            bind:value={globalCategoryId}
          >
            <option value={undefined}>{m.import_preview_category_placeholder()}</option>
            {#each categories as category}
              <option value={category.id}>{category.name}</option>
            {/each}
          </select>
        </div>

        <div class="flex min-w-0 max-w-full flex-col gap-1">
          <label class="text-xs font-medium text-muted" for="global-transfer-account">
            {m.import_preview_transfer_account_label()}
          </label>
          <select
            id="global-transfer-account"
            class="min-w-0 max-w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-foreground"
            bind:value={globalTransferAccountId}
          >
            <option value={undefined}>{m.import_preview_transfer_account_placeholder()}</option>
            {#each accounts as account}
              <option value={account.id}>{account.name ?? account.code}</option>
            {/each}
          </select>
        </div>

        <div class="flex items-end">
          <button
            type="button"
            class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-medium text-foreground transition hover:bg-control-hover"
            onclick={applyGlobalAccount}
          >
            {m.import_preview_apply_all()}
          </button>
        </div>
      </div>
    </Panel>

    {/if}

    {#if previewData.rows.some(row => parseNormalized(row).investment)}
      <CSVInvestmentReview rows={previewData.rows} resolutions={rowResolutions} {accounts} {currencies} onchange={updateResolution} />
    {/if}
    <!-- Rows table -->
    {#if sourceCorrectionSuccessID}
      <Panel>
        <p class="text-sm font-medium text-positive">{m.import_preview_source_corrected()}</p>
        <a href={`/app/ledger/transactions?transaction_id=${sourceCorrectionSuccessID}`} class="text-sm font-semibold text-foreground underline underline-offset-2">
          {m.import_preview_open_correction()}
        </a>
      </Panel>
    {/if}
    <Panel padding="none">
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="border-b border-border text-left">
              <th class="px-4 py-3 font-semibold text-muted">{m.import_preview_col_date()}</th>
              <th class="px-4 py-3 font-semibold text-muted">{m.import_preview_col_payee()}</th>
              <th class="px-4 py-3 font-semibold text-muted text-right">{m.import_preview_col_amount()}</th>
              <th class="px-4 py-3 font-semibold text-muted">{m.import_preview_col_memo()}</th>
              <th class="px-4 py-3 font-semibold text-muted">{m.import_preview_col_dedupe()}</th>
              <th class="px-4 py-3 font-semibold text-muted">{m.import_preview_account_label()}</th>
              <th class="px-4 py-3 font-semibold text-muted">{m.import_preview_currency_label()}</th>
              <th class="px-4 py-3 font-semibold text-muted">{m.import_preview_col_category()}</th>
              <th class="px-4 py-3 font-semibold text-muted">{m.import_preview_col_exclude()}</th>
            </tr>
          </thead>
          <tbody>
            {#each previewData.rows as row, rowIndex (row.id)}
              {@const norm = parseNormalized(row)}
              {@const cryptoSummary = cryptoCSVSummary(row.raw, getLocale())}
              {@const res = getResolution(row.id)}
              {@const isDuplicate = row.dedupe_status === 'duplicate'}
              {@const unsupportedFill = !norm.investment && unsupportedSourceFill(row)}
              {@const isBlocked = unsupportedFill || isDuplicate || row.source_changed || !rowReviewable(row) || !batchReviewable(previewData.batch.status)}
              {@const isExcluded = res.exclude || row.dedupe_status === 'excluded'}
              {@const isTransfer = !!norm.transfer_hint}
              <tr
                class:opacity-40={isDuplicate || isExcluded}
                class="border-b border-border last:border-b-0"
              >
                <td class="px-4 py-2.5 tabular-nums text-muted">{norm.date}</td>
                <td class="px-4 py-2.5 font-medium text-foreground">{cryptoSummary || res.payee_name || norm.payee_hint || '—'}</td>
                <td class="px-4 py-2.5 tabular-nums text-right text-foreground">{maskFinancialText(norm.amount)}</td>
                <td class="max-w-xs px-4 py-2.5 text-muted">
                  <span class="block truncate">{norm.memo || '—'}</span>
                  {#if res.applied_rule_name}
                    <span class="mt-1 block text-xs font-medium text-foreground">{appliedRuleSummary(res)}</span>
                  {/if}
                </td>
                <td class="px-4 py-2.5">
                  <span
                    class:text-warning={unsupportedFill || row.source_changed || row.dedupe_status === 'needs_attention' || !!norm.parse_errors?.length}
                    class:text-muted={row.dedupe_status === 'duplicate' || row.dedupe_status === 'excluded'}
                    class="text-xs font-medium"
                  >
                    {row.commit_status === 'committed' && unsupportedFill
                      ? (linkedFillKind(row) === 'split' ? m.import_preview_split_linked() : m.import_preview_short_linked())
                      : row.commit_status === 'committed' && row.dedupe_status === 'needs_attention'
                      ? m.import_preview_source_revision_accepted()
                      : unsupportedFill ? m.import_preview_unsupported_fill() : row.source_changed ? m.import_preview_source_changed()
                      : norm.parse_errors?.length ? m.import_preview_parse_error() : dedupeStatusLabel(row.dedupe_status)}
                  </span>
                  {#if row.commit_status === 'pending' && row.commit_error === 'reconciliation override required'}
                    <span class="mt-1 block text-xs font-medium text-warning">{m.import_preview_row_reconciliation_held()}</span>
                  {:else if row.commit_status === 'failed'}
                    <span class="mt-1 block text-xs font-medium text-warning">{m.import_preview_row_failed()}</span>
                    {#if row.commit_error}<span class="block text-xs text-muted">{row.commit_error}</span>{/if}
                  {/if}
                  {#if row.source_changed && row.source_transaction_id}
                    <a
                      href={`/app/ledger/transactions?transaction_id=${row.source_transaction_id}`}
                      class="mt-1 block text-xs font-semibold text-foreground underline underline-offset-2 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-foreground"
                    >
                      {m.import_preview_open_original()}
                    </a>
                  {/if}
                  {#if sourceCorrectionKind(row)}
                    <button
                      type="button"
                      disabled={sourceCorrectionPending}
                      class="mt-2 block rounded-(--radius-control) border border-border bg-control px-2 py-1 text-xs font-semibold text-foreground hover:bg-control-hover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-foreground"
                      onclick={() => {
                        sourceCorrectionRowID = sourceCorrectionRowID === row.id ? null : row.id;
                        sourceCorrectionError = undefined;
                        sourceCorrectionReason = '';
                        sourceCorrectionImpact = null;
                        sourceCorrectionOverride = false;
                      }}
                    >{sourceCorrectionKind(row) === 'sale' ? m.import_preview_correct_sale() : m.import_preview_correct_buy()}</button>
                  {/if}
                  {#if linkableSplitFill(row) && batchId}
                    <button
                      type="button"
                      aria-expanded={splitLinkRowID === row.id}
                      class="mt-2 block rounded-(--radius-control) border border-border bg-control px-2 py-1 text-xs font-semibold text-foreground hover:bg-control-hover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-foreground"
                      onclick={() => { splitLinkRowID = splitLinkRowID === row.id ? null : row.id; }}
                    >{m.import_preview_split_link()}</button>
                  {/if}
                  {#if linkableShortFill(row) && batchId}
                    <button
                      type="button"
                      aria-expanded={shortLinkRowID === row.id}
                      class="mt-2 block rounded-(--radius-control) border border-border bg-control px-2 py-1 text-xs font-semibold text-foreground hover:bg-control-hover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-foreground"
                      onclick={() => { shortLinkRowID = shortLinkRowID === row.id ? null : row.id; }}
                    >{m.import_preview_short_link()}</button>
                  {/if}
                </td>
                <td class="px-4 py-2.5">
                  {#if !isBlocked && !cryptoSummary && !norm.investment}
                    <select
                      aria-label={m.import_preview_row_account({ row: rowIndex + 1 })}
                      class="rounded-(--radius-control) border border-border bg-control px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-foreground"
                      value={res.account_id}
                      onchange={(e) =>
                        updateResolution(row.id, {
                          account_id: Number((e.currentTarget as HTMLSelectElement).value) || undefined
                        })}
                    >
                      <option value="">—</option>
                      {#each accounts as account}
                        <option value={account.id}>{account.name ?? account.code}</option>
                      {/each}
                    </select>
                  {:else}
                    <span class="text-xs text-muted">{cryptoSummary || norm.investment ? accounts.find(a => a.id === res.account_id)?.name : '—'}</span>
                  {/if}
                </td>
                <td class="px-4 py-2.5">
                  {#if !isBlocked && !cryptoSummary && !norm.investment}
                    <select
                      aria-label={m.import_preview_row_currency({ row: rowIndex + 1 })}
                      class="rounded-(--radius-control) border border-border bg-control px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-foreground"
                      value={res.commodity_id}
                      onchange={(e) =>
                        updateResolution(row.id, {
                          commodity_id: Number((e.currentTarget as HTMLSelectElement).value) || undefined
                        })}
                    >
                      <option value="">—</option>
                      {#each currencies as currency}
                        <option value={currency.id}>{currency.code}</option>
                      {/each}
                    </select>
                  {:else}
                    <span class="text-xs text-muted">{cryptoSummary || norm.investment ? currencies.find(c => c.id === res.commodity_id)?.code : '—'}</span>
                  {/if}
                </td>
                <td class="px-4 py-2.5">
                  {#if !isBlocked && !cryptoSummary && !norm.investment}
                    {#if isTransfer}
                      <select
                        aria-label={m.import_preview_row_transfer_account({ row: rowIndex + 1 })}
                        class="rounded-(--radius-control) border border-border bg-control px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-foreground"
                        value={res.transfer_account_id}
                        onchange={(e) =>
                          updateResolution(row.id, {
                            transfer_account_id: Number((e.currentTarget as HTMLSelectElement).value) || undefined
                          })}
                      >
                        <option value="">{norm.transfer_hint} →?</option>
                        {#each accounts as account}
                          <option value={account.id}>{account.name ?? account.code}</option>
                        {/each}
                      </select>
                    {:else}
                      <select
                        aria-label={m.import_preview_row_category({ row: rowIndex + 1 })}
                        class="rounded-(--radius-control) border border-border bg-control px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-foreground"
                        value={res.category_id}
                        onchange={(e) =>
                          updateResolution(row.id, {
                            category_id: Number((e.currentTarget as HTMLSelectElement).value) || undefined
                          })}
                      >
                        <option value="">—</option>
                        {#each categories as category}
                          <option value={category.id}>{category.name}</option>
                        {/each}
                      </select>
                    {/if}
                  {:else}
                    <span class="text-xs text-muted">—</span>
                  {/if}
                </td>
                <td class="px-4 py-2.5">
                  {#if !isBlocked}
                    <input
                      type="checkbox"
                      aria-label={m.import_preview_col_exclude()}
                      checked={!!res.exclude}
                      onchange={() => toggleExclude(row)}
                      class="h-4 w-4 rounded border-border text-foreground focus:ring-foreground"
                    />
                  {/if}
                </td>
              </tr>
              {#if splitLinkRowID === row.id && batchId}
                <tr class="border-b border-border bg-control">
                  <td colspan="9" class="px-4 py-4">
                    <SplitLinkPanel batchId={batchId} rowId={row.id} {csrfToken} {accounts} onLinked={() => handleOperationLinked(row.id)} />
                  </td>
                </tr>
              {/if}
              {#if shortLinkRowID === row.id && batchId}
                <tr class="border-b border-border bg-control">
                  <td colspan="9" class="px-4 py-4">
                    <!-- Pinned to the visible width so the review stays readable inside the scrolling table on a phone. -->
                    <div class="sticky left-4 max-w-[calc(100vw-4rem)]">
                      <ShortLinkPanel batchId={batchId} rowId={row.id} {csrfToken} {accounts} onLinked={() => handleOperationLinked(row.id)} />
                    </div>
                  </td>
                </tr>
              {/if}
              {#if sourceCorrectionRowID === row.id}
                <tr class="border-b border-border bg-control">
                  <td colspan="9" class="px-4 py-4">
                    <form class="max-w-xl space-y-3" onsubmit={(event) => { event.preventDefault(); void handleSourceCorrection(row); }}>
                      <p class="text-sm text-muted">{sourceCorrectionKind(row) === 'sale' ? m.import_preview_correct_sale_scope() : m.import_preview_correct_buy_scope()}</p>
                      <label class="block text-sm font-medium text-foreground" for={`source-correction-reason-${row.id}`}>
                        {m.import_preview_correct_buy_reason()}
                      </label>
                      <input
                        id={`source-correction-reason-${row.id}`}
                        type="text"
                        required
                        bind:value={sourceCorrectionReason}
                        disabled={sourceCorrectionPending}
                        oninput={() => { sourceCorrectionImpact = null; sourceCorrectionOverride = false; sourceCorrectionAcceptGains = false; }}
                        class="w-full rounded-(--radius-control) border border-border bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-foreground"
                      />
                      {#if sourceCorrectionImpact}
                        {#if sourceCorrectionImpact.affected_checkpoints.length > 0}
                          <p class="text-sm font-medium text-foreground">{m.import_preview_correct_buy_impact()}</p>
                          <ul class="space-y-2 text-sm text-foreground">
                            {#each sourceCorrectionImpact.affected_checkpoints as checkpoint (checkpoint.checkpoint_id)}
                              <li>{m.transactions_reconciliation_checkpoint_label({
                                account: checkpoint.account_label,
                                commodity: checkpoint.commodity_code,
                                date: formatDate(checkpoint.statement_date)
                              })}</li>
                            {/each}
                          </ul>
                          <label class="flex items-center gap-2 text-sm text-foreground">
                            <input type="checkbox" bind:checked={sourceCorrectionOverride} disabled={sourceCorrectionPending} class="h-4 w-4 rounded border-border" />
                            {m.import_commit_reconciliation_override()}
                          </label>
                          <p class="text-sm text-muted">{m.import_commit_reconciliation_override_hint()}</p>
                        {:else if sourceCorrectionGainRows.length === 0}
                          <p class="text-sm text-muted">{m.import_preview_correct_buy_no_impact()}</p>
                        {/if}
                        {#if sourceCorrectionGainRows.length > 0}
                          <div class="rounded-(--radius-control) border border-border bg-background">
                            <GainImpactList rows={sourceCorrectionGainRows} showHeading />
                          </div>
                          <label class="flex items-center gap-2 text-sm text-foreground">
                            <input type="checkbox" bind:checked={sourceCorrectionAcceptGains} disabled={sourceCorrectionPending} class="h-4 w-4 rounded border-border" />
                            {m.investments_gain_impact_confirm()}
                          </label>
                        {/if}
                      {/if}
                      <APIFormError error={sourceCorrectionError} id="source-correction-error" />
                      <button
                        type="submit"
                        disabled={sourceCorrectionPending || !csrfToken || !sourceCorrectionReason.trim() || (sourceCorrectionImpact !== null && sourceCorrectionImpact.affected_checkpoints.length > 0 && !sourceCorrectionOverride) || (sourceCorrectionGainRows.length > 0 && !sourceCorrectionAcceptGains)}
                        class="rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
                      >{sourceCorrectionPending ? (sourceCorrectionImpact ? m.import_preview_correct_buy_pending() : m.import_preview_correct_buy_preview_pending()) : sourceCorrectionImpact ? m.import_preview_correct_buy_submit() : m.import_preview_correct_buy_preview()}</button>
                    </form>
                  </td>
                </tr>
              {/if}
            {/each}
          </tbody>
        </table>

        {#if previewData.rows.length === 0}
          <p class="px-4 py-8 text-center text-sm text-muted">{m.import_preview_no_rows()}</p>
        {/if}
      </div>
    </Panel>

    <!-- Commit controls -->
    {#if batchReviewable(previewData.batch.status)}
    <Panel>
      <div class="space-y-4">
        <label class="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            bind:checked={reconciliationOverride}
            onchange={() => void handleReconciliationOverrideChange()}
            aria-describedby="commit-reconciliation-checkpoints"
            class="h-4 w-4 rounded border-border focus:ring-foreground"
          />
          <span class="font-medium text-foreground">{m.import_commit_reconciliation_override()}</span>
        </label>
        <p class="text-sm text-muted">{m.import_commit_reconciliation_override_hint()}</p>
        <div id="commit-reconciliation-checkpoints" aria-live="polite" class="space-y-2 text-sm">
          {#if reconciliationOverride}
            {#if reviewedCheckpointsPending}
              <p class="text-muted">{m.import_commit_checkpoints_loading()}</p>
            {:else if reviewedCheckpoints}
              {#if reviewedCheckpointsChanged}
                <p class="text-warning">{m.import_commit_checkpoints_changed()}</p>
              {/if}
              {#if reviewedCheckpoints.length === 0}
                <p class="text-muted">{m.import_commit_checkpoints_none()}</p>
              {:else}
                <p class="font-medium text-foreground">{m.import_commit_checkpoints_title()}</p>
                <ul class="divide-y divide-border rounded-(--radius-control) border border-border">
                  {#each reviewedCheckpoints as checkpoint (checkpoint.checkpoint_id)}
                    <li class="px-3 py-2 text-foreground">
                      {m.transactions_reconciliation_checkpoint_label({
                        account: checkpoint.account_label,
                        commodity: checkpoint.commodity_code,
                        date: checkpoint.statement_date
                      })}
                    </li>
                  {/each}
                </ul>
              {/if}
            {/if}
            <APIFormError error={reviewedCheckpointsError} id="commit-checkpoints-error" />
          {/if}
        </div>

        <APIFormError error={commitError} id="commit-error" />

        <div class="flex items-center gap-3">
          <button
            type="button"
            class="inline-flex items-center gap-2 rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
            onclick={handleCommit}
            disabled={committing || sourceCorrectionPending || reviewedCheckpointsPending}
          >
            {committing ? m.import_commit_pending() : m.import_commit_button()}
          </button>

          {#if previewData.batch.status === 'previewing'}
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
              onclick={() => { showDiscardConfirm = true; }}
            >
              {m.import_discard_button()}
            </button>
          {/if}
        </div>
      </div>
    </Panel>
    {/if}

    <!-- Discard confirm -->
    {#if showDiscardConfirm}
      <Panel>
        <p class="text-sm font-semibold text-foreground">{m.import_discard_confirm()}</p>
        <APIFormError error={discardError} id="discard-error" />
        <div class="mt-4 flex gap-3">
          <button
            type="button"
            class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
            onclick={() => { showDiscardConfirm = false; discardError = undefined; }}
          >
            {m.import_discard_cancel()}
          </button>
          <button
            type="button"
            class="inline-flex items-center gap-2 rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
            onclick={handleDiscard}
            disabled={discarding}
          >
            {m.import_discard_confirm_button()}
          </button>
        </div>
      </Panel>
    {/if}
  </div>

<!-- Result step -->
{:else if step === 'result' && commitResult}
  <div class="max-w-2xl space-y-6">
    <Panel>
      <div class="flex items-center gap-2">
        <CheckCircle size={20} class="text-positive shrink-0" aria-hidden="true" />
        <p class="text-sm font-semibold text-foreground">{m.import_result_title()}</p>
      </div>

      <div class="mt-4 space-y-2 text-sm">
        <p class="text-foreground">{m.import_result_committed({ count: commitResult.committed_count })}</p>
        {#if commitResult.skipped_count > 0}
          <p class="text-muted">{m.import_result_skipped({ count: commitResult.skipped_count })}</p>
          {#if previewData?.rows.some((row) => row.source_changed)}
            <button type="button" disabled={reviewChangedPending} class="text-sm font-semibold text-foreground underline underline-offset-2 disabled:opacity-60" onclick={() => void handleReviewChangedFills()}>
              {m.import_result_review_changed_fills()}
            </button>
            <APIFormError error={reviewChangedError} id="review-changed-error" />
          {/if}
        {/if}
        {#if commitResult.failed_count > 0}
          <p class="text-warning">{m.import_result_failed({ count: commitResult.failed_count })}</p>
        {/if}
        {#if commitResult.reconciliation_hold_row_ids.length > 0}
          <p class="text-warning">{m.import_result_reconciliation_held({ count: commitResult.reconciliation_hold_row_ids.length })}</p>
        {/if}
        {#if commitResult.reconciliation_stale_row_ids.length > 0}
          <p class="text-warning">{m.import_result_reconciliation_stale({ count: commitResult.reconciliation_stale_row_ids.length })}</p>
        {/if}
        {#if commitResult.failed_count > 0 || commitResult.reconciliation_hold_row_ids.length > 0 || commitResult.reconciliation_stale_row_ids.length > 0}
          <button type="button" disabled={reviewChangedPending} class="inline-flex min-h-11 items-center text-sm font-semibold text-foreground underline underline-offset-2 disabled:opacity-60" onclick={() => void handleReviewRemainingRows()}>
            {m.import_result_review_remaining()}
          </button>
          <APIFormError error={reviewRemainingError} id="review-remaining-error" />
        {/if}
        {#if commitResult.dividend_review_row_ids.length > 0}
          <p class="text-warning">{m.import_dividend_held({ count: commitResult.dividend_review_row_ids.length })}</p>
          <a href={`/app/ledger/import/dividends?batch_id=${batchId}`} class="inline-flex min-h-11 items-center text-sm font-semibold text-foreground underline underline-offset-2">{m.import_dividend_link()}</a>
        {/if}
        {#if commitResult.gain_review_row_ids.length > 0}
          <p class="text-warning">{m.import_result_gain_review({ count: commitResult.gain_review_row_ids.length })}</p>
          <button type="button" disabled={committing} class="text-sm font-semibold text-foreground underline underline-offset-2 disabled:opacity-60" onclick={() => void handleReviewHeldGains()}>
            {m.import_result_gain_review_action()}
          </button>
          <APIFormError error={commitError} id="gain-review-error" />
        {/if}
      </div>

      <div class="mt-6 flex gap-3">
        <a
          href="/app/ledger/transactions"
          class="inline-flex items-center gap-2 rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90"
        >
          {m.import_result_view_transactions()}
        </a>
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
          onclick={handleImportAnother}
        >
          {m.import_result_import_another()}
        </button>
      </div>
    </Panel>
  </div>
{/if}

{#if gainReview}
  <ReconciliationConfirm
    impacts={[]}
    gainRows={gainReview.rows}
    gainRefreshed={gainReview.refreshed}
    pending={committing}
    onCancel={() => (gainReview = null)}
    onConfirm={() => void confirmImportGains()}
  />
{/if}
