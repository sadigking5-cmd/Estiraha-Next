import React, { useEffect, useRef, useState } from 'react';
import { Events } from '@wailsio/runtime';
import * as PoCService from './bindings/github.com/sadigking5-cmd/Estiraha-Next/internal/api/pocservice';
import type { PlatformDiagnostics } from './bindings/github.com/sadigking5-cmd/Estiraha-Next/internal/api/models';
import { LOCALES } from './i18n/locale';
import type { LanguageCode, ThemeMode } from './services/contracts';
import {
  LibraryClient,
  callPoC,
  isNativeWailsRuntime,
  type AIAssistResponse,
  type AuditLogView,
  type BackupRecord,
  type DuplicateGroupView,
  type ErrorCategorySpec,
  type ErrorFolderEntryView,
  type ExecutionManifestSummary,
  type ExecutionProgressView,
  type HealthIssueView,
  type JournalEntryView,
  type MovieItem,
  type PlaybackSession,
  type ProviderStatusView,
  type ReviewBatchView,
  type ReviewItemDetailView,
  type ReviewItemView,
  type ScanAndAnalyzeResult,
  type SeriesItem,
  type SystemOverview,
} from './services/libraryClient';

type WorkspaceKey =
  | 'dashboard'
  | 'movies'
  | 'series'
  | 'disks'
  | 'review'
  | 'organizer'
  | 'errors'
  | 'ai'
  | 'player'
  | 'reports'
  | 'settings';

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let val = bytes;
  while (val >= 1024 && i < units.length - 1) {
    val /= 1024;
    i++;
  }
  return `${val.toFixed(i > 1 ? 2 : 0)} ${units[i]}`;
}

export function App(): React.JSX.Element {
  const [lang, setLang] = useState<LanguageCode>('ar');
  const [theme, setTheme] = useState<ThemeMode>('dark');
  const [tab, setTab] = useState<WorkspaceKey>('dashboard');
  const [busy, setBusy] = useState<boolean>(false);
  const [banner, setBanner] = useState<{ kind: 'ok' | 'err'; text: string } | null>(null);

  // Global System & Phase 1A Diagnostics State
  const [diag, setDiag] = useState<PlatformDiagnostics | null>(null);
  const [overview, setOverview] = useState<SystemOverview | null>(null);
  const [lastScanResult, setLastScanResult] = useState<ScanAndAnalyzeResult | null>(null);
  const [brokenPosterUrls, setBrokenPosterUrls] = useState<Record<string, boolean>>({});

  const markPosterBroken = (url: string) => {
    if (!url) return;
    setBrokenPosterUrls((prev) => (prev[url] ? prev : { ...prev, [url]: true }));
  };

  // Movies State (B3)
  const [movieQuery, setMovieQuery] = useState('');
  const [movieStatusFilter, setMovieStatusFilter] = useState('all');
  const [movieOriginFilter, setMovieOriginFilter] = useState<'all' | 'ar' | 'foreign'>('all');
  const [moviesList, setMoviesList] = useState<MovieItem[]>([]);
  const [selectedMovie, setSelectedMovie] = useState<MovieItem | null>(null);
  const [editMovieTitle, setEditMovieTitle] = useState('');
  const [editMovieYear, setEditMovieYear] = useState(2024);
  const [editMovieAr, setEditMovieAr] = useState('');
  const [editMovieEn, setEditMovieEn] = useState('');
  const [editMovieIsAr, setEditMovieIsAr] = useState(false);
  const [editMovieRating, setEditMovieRating] = useState(8.0);
  const [editMovieNote, setEditMovieNote] = useState('');

  // Series State (B4)
  const [seriesQuery, setSeriesQuery] = useState('');
  const [seriesMissingOnly, setSeriesMissingOnly] = useState(false);
  const [seriesList, setSeriesList] = useState<SeriesItem[]>([]);
  const [selectedSeries, setSelectedSeries] = useState<SeriesItem | null>(null);
  const [editSeriesTitle, setEditSeriesTitle] = useState('');
  const [editSeriesIsAr, setEditSeriesIsAr] = useState(false);

  // Disks & Roots State (B1)
  const [newRootPath, setNewRootPath] = useState('');
  const [newRootCategory, setNewRootCategory] = useState('movies');
  const [newRootName, setNewRootName] = useState('');
  const [newDiskLabel, setNewDiskLabel] = useState('');
  const [scanComputeHash, setScanComputeHash] = useState(true);

  // Review Center, Execution Dashboard & Error Center State (Batch 3)
  const [batches, setBatches] = useState<ReviewBatchView[]>([]);
  const [activeBatchId, setActiveBatchId] = useState('');
  const [batchItems, setBatchItems] = useState<ReviewItemView[]>([]);
  const [manifestSummary, setManifestSummary] = useState<ExecutionManifestSummary | null>(null);
  const [execProgress, setExecProgress] = useState<ExecutionProgressView | null>(null);
  const [selectedReviewIds, setSelectedReviewIds] = useState<Record<string, boolean>>({});
  const [reviewFilterStatus, setReviewFilterStatus] = useState<string>('all');
  const [editTargetModalItem, setEditTargetModalItem] = useState<ReviewItemView | null>(null);
  const [modalEditName, setModalEditName] = useState('');
  const [modalEditDir, setModalEditDir] = useState('');
  const [detailModalData, setDetailModalData] = useState<ReviewItemDetailView | null>(null);

  // Error Center State (6 Official Categories)
  const [errorCategories, setErrorCategories] = useState<ErrorCategorySpec[]>([]);
  const [activeErrorCategory, setActiveErrorCategory] = useState<string>('all');
  const [errorEntries, setErrorEntries] = useState<ErrorFolderEntryView[]>([]);
  const [selectedErrorIds, setSelectedErrorIds] = useState<Record<string, boolean>>({});
  const [inspectedErrorEntry, setInspectedErrorEntry] = useState<ErrorFolderEntryView | null>(null);

  const [dupGroups, setDupGroups] = useState<DuplicateGroupView[]>([]);
  const [journalEntries, setJournalEntries] = useState<JournalEntryView[]>([]);
  const [namingSampleTitle, setNamingSampleTitle] = useState('الناظر');
  const [namingSampleYear, setNamingSampleYear] = useState(2000);
  const [namingPreviewOut, setNamingPreviewOut] = useState<string>('');

  // AI Assistant State
  const [aiPrompt, setAiPrompt] = useState('');
  const [allowDeepSeek, setAllowDeepSeek] = useState(false);
  const [aiReply, setAiReply] = useState<AIAssistResponse | null>(null);

  // Integrated Player State (B10)
  const [session, setSession] = useState<PlaybackSession | null>(null);
  const videoRef = useRef<HTMLVideoElement | null>(null);

  // Reports State (B9)
  const [issueFilter, setIssueFilter] = useState('all');
  const [issues, setIssues] = useState<HealthIssueView[]>([]);

  // Settings, Providers, Backups & Audit State
  const [backups, setBackups] = useState<BackupRecord[]>([]);
  const [providersList, setProvidersList] = useState<ProviderStatusView[]>([]);
  const [providerKeyInputs, setProviderKeyInputs] = useState<Record<string, string>>({});
  const [auditLogs, setAuditLogs] = useState<AuditLogView[]>([]);

  const isAr = lang === 'ar';
  const localeCfg = LOCALES[lang];

  const notify = (kind: 'ok' | 'err', text: string) => {
    setBanner({ kind, text });
  };

  useEffect(() => {
    document.documentElement.lang = localeCfg.code;
    document.documentElement.dir = localeCfg.dir;
    document.documentElement.setAttribute('data-theme', theme);
  }, [localeCfg, theme]);

  const refreshCore = async () => {
    try {
      const [d, ov, errCats, errItems] = await Promise.all([
        callPoC(PoCService.GetPlatformDiagnostics, 'GetPlatformDiagnostics'),
        LibraryClient.getSystemOverview(),
        LibraryClient.listErrorFolderCategories(),
        LibraryClient.listErrorFolderEntries('all'),
      ]);
      setDiag(d);
      setOverview(ov);
      setErrorCategories(errCats);
      setErrorEntries(errItems);
    } catch (err: unknown) {
      notify('err', String(err));
    }
  };

  useEffect(() => {
    void refreshCore();
    if (!isNativeWailsRuntime()) return;
    const offProg = Events.On('estiraha:scan-progress', (ev: unknown) => {
      notify('ok', `${isAr ? 'جاري الفحص اليدوي...' : 'Manual scan in progress...'} ${JSON.stringify(ev)}`);
    });
    const offDone = Events.On('estiraha:scan-completed', () => {
      void refreshCore();
    });
    return () => {
      offProg();
      offDone();
    };
  }, []);

  useEffect(() => {
    if (tab === 'movies') void loadMovies();
    if (tab === 'series') void loadSeries();
    if (tab === 'review' || tab === 'organizer') void loadReviewAndOrganizer();
    if (tab === 'errors') void loadErrorCenter(activeErrorCategory);
    if (tab === 'reports') void loadIssues(issueFilter);
    if (tab === 'settings') void loadSettingsData();
  }, [tab]);

  const loadMovies = async () => {
    try {
      const list = await LibraryClient.listMovies({
        query: movieQuery,
        identificationStatus: movieStatusFilter,
        watchStatus: 'all',
        arabicOnly: movieOriginFilter === 'ar',
        foreignOnly: movieOriginFilter === 'foreign',
        diskId: 'all',
        limit: 250,
      });
      setMoviesList(list);
    } catch (err: unknown) {
      notify('err', String(err));
    }
  };

  const openMovieDetail = async (workId: string) => {
    try {
      const detail = await LibraryClient.getMovieDetail(workId);
      setSelectedMovie(detail);
      setEditMovieTitle(detail.titleOriginal);
      setEditMovieYear(detail.productionYear || 2024);
      setEditMovieAr(detail.titleAr);
      setEditMovieEn(detail.titleEn);
      setEditMovieIsAr(detail.isArabicOrigin);
      setEditMovieRating(detail.ratingOutOf10 || 8.0);
      setEditMovieNote(detail.personalNote || '');
    } catch (err: unknown) {
      notify('err', String(err));
    }
  };

  const handleSaveMovieMetadata = async () => {
    if (!selectedMovie) return;
    setBusy(true);
    try {
      const updated = await LibraryClient.updateMovieMetadata({
        workId: selectedMovie.id,
        titleOriginal: editMovieTitle,
        titleAr: editMovieAr,
        titleEn: editMovieEn,
        productionYear: Number(editMovieYear),
        isArabicOrigin: editMovieIsAr,
        synopsisAr: selectedMovie.synopsisAr || '',
        synopsisEn: selectedMovie.synopsisEn || '',
        ratingOutOf10: Number(editMovieRating),
        personalNote: editMovieNote,
        isFavorite: selectedMovie.isFavorite,
      });
      setSelectedMovie(updated);
      await loadMovies();
      notify('ok', isAr ? 'تم حفظ بيانات الفيلم وقفل التعريف بنجاح (الملفات لم تُمس قبل المراجعة).' : 'Movie metadata saved (physical files untouched until approved in Review Center).');
    } catch (err: unknown) {
      notify('err', String(err));
    } finally {
      setBusy(false);
    }
  };

  const loadSeries = async () => {
    try {
      const list = await LibraryClient.listSeries({
        query: seriesQuery,
        mediaType: 'all',
        missingOnly: seriesMissingOnly,
        arabicOnly: false,
        limit: 250,
      });
      setSeriesList(list);
    } catch (err: unknown) {
      notify('err', String(err));
    }
  };

  const openSeriesDetail = async (workId: string) => {
    try {
      const detail = await LibraryClient.getSeriesDetail(workId);
      setSelectedSeries(detail);
      setEditSeriesTitle(detail.titleOriginal);
      setEditSeriesIsAr(detail.isArabicOrigin);
    } catch (err: unknown) {
      notify('err', String(err));
    }
  };

  const handleSaveSeriesMetadata = async () => {
    if (!selectedSeries) return;
    setBusy(true);
    try {
      const updated = await LibraryClient.updateSeriesMetadata({
        workId: selectedSeries.id,
        titleOriginal: editSeriesTitle,
        titleAr: editSeriesIsAr ? editSeriesTitle : selectedSeries.titleAr,
        titleEn: !editSeriesIsAr ? editSeriesTitle : selectedSeries.titleEn,
        isArabicOrigin: editSeriesIsAr,
        synopsisAr: selectedSeries.synopsisAr || '',
        synopsisEn: selectedSeries.synopsisEn || '',
      });
      setSelectedSeries(updated);
      await loadSeries();
      notify('ok', isAr ? 'تم تحديث بيانات المسلسل (بدون سنة إنتاج في اسم المجلد).' : 'Series metadata saved (no year in canonical series folder).');
    } catch (err: unknown) {
      notify('err', String(err));
    } finally {
      setBusy(false);
    }
  };

  const handleBrowseFolder = async () => {
    try {
      const picked = await LibraryClient.pickFolderDialog();
      if (picked) setNewRootPath(picked);
    } catch (err: unknown) {
      notify('err', String(err));
    }
  };

  const handleAddRoot = async () => {
    if (!newRootPath.trim()) return;
    setBusy(true);
    try {
      await LibraryClient.addLibraryRoot({
        fullPath: newRootPath.trim(),
        contentCategory: newRootCategory,
        displayName: newRootName.trim(),
        diskLabel: newDiskLabel.trim(),
      });
      setNewRootPath('');
      setNewRootName('');
      await refreshCore();
      notify('ok', isAr ? 'تمت إضافة مسار المكتبة بنجاح (لن يبدأ الفحص إلا يدوياً بأمرك).' : 'Library root registered (Manual Scan only — zero auto-scan).');
    } catch (err: unknown) {
      notify('err', String(err));
    } finally {
      setBusy(false);
    }
  };

  const handleManualScan = async (rootIds: string[] = []) => {
    setBusy(true);
    try {
      const res = await LibraryClient.runManualScanAndAnalyze({
        rootIds,
        computeHash: scanComputeHash,
      });
      setLastScanResult(res);
      await refreshCore();
      notify(
        'ok',
        isAr
          ? `اكتمل الفحص اليدوي: ${res.scan.filesDiscovered} ملف (${res.identification.moviesIdentified} فيلم، ${res.identification.seriesIdentified} مسلسل، ${res.scan.shortsDetected} ملف .shorts محمي).`
          : `Manual scan completed: ${res.scan.filesDiscovered} files (${res.identification.moviesIdentified} movies, ${res.identification.seriesIdentified} series, ${res.scan.shortsDetected} protected .shorts).`
      );
    } catch (err: unknown) {
      notify('err', String(err));
    } finally {
      setBusy(false);
    }
  };

  const loadReviewAndOrganizer = async (preferredBatchId?: string) => {
    try {
      const [bList, dGroups, jList, errItems] = await Promise.all([
        LibraryClient.listReviewBatches(),
        LibraryClient.listDuplicateGroups(),
        LibraryClient.listOperationJournal(''),
        LibraryClient.listErrorFolderEntries('all'),
      ]);
      setBatches(bList);
      setDupGroups(dGroups);
      setJournalEntries(jList);
      setErrorEntries(errItems);
      const targetId = preferredBatchId || activeBatchId || (bList[0]?.id ?? '');
      if (targetId) {
        setActiveBatchId(targetId);
        const [items, manifest, prog] = await Promise.all([
          LibraryClient.listReviewBatchItems(targetId),
          LibraryClient.getExecutionManifestSummary(targetId),
          LibraryClient.getBatchExecutionProgress(targetId),
        ]);
        setBatchItems(items);
        setManifestSummary(manifest);
        setExecProgress(prog);
      } else {
        setBatchItems([]);
        setManifestSummary(null);
        setExecProgress(null);
      }
    } catch (err: unknown) {
      notify('err', String(err));
    }
  };

  const loadErrorCenter = async (catCode = 'all') => {
    try {
      const [cats, list] = await Promise.all([
        LibraryClient.listErrorFolderCategories(),
        LibraryClient.listErrorFolderEntries(catCode),
      ]);
      setErrorCategories(cats);
      setErrorEntries(list);
    } catch (err: unknown) {
      notify('err', String(err));
    }
  };

  const handleGeneratePlan = async () => {
    setBusy(true);
    try {
      const batch = await LibraryClient.generateOrganizationPlan('');
      setSelectedReviewIds({});
      await loadReviewAndOrganizer(batch.id);
      setTab('review');
      notify('ok', isAr ? `تم إنشاء خطة معاينة غير مدمرة (${batch.totalItems} عنصر بانتظار موافقتك).` : `Created non-destructive preview plan (${batch.totalItems} items awaiting explicit approval).`);
    } catch (err: unknown) {
      notify('err', String(err));
    } finally {
      setBusy(false);
    }
  };

  const handleDecideItem = async (item: ReviewItemView, decision: string) => {
    try {
      await LibraryClient.decideReviewItem({
        itemId: item.id,
        decision,
        userEditedRelPath: '',
        userEditedName: '',
      });
      await loadReviewAndOrganizer(item.batchId);
    } catch (err: unknown) {
      notify('err', String(err));
    }
  };

  const handleSelectAllReview = (visibleItems: ReviewItemView[]) => {
    const next: Record<string, boolean> = {};
    for (const it of visibleItems) {
      if (it.decisionStatus !== 'executed') {
        next[it.id] = true;
      }
    }
    setSelectedReviewIds(next);
  };

  const handleDeselectAllReview = () => {
    setSelectedReviewIds({});
  };

  const handleBulkReviewDecision = async (decision: 'approved' | 'rejected') => {
    if (!activeBatchId) return;
    const ids = Object.keys(selectedReviewIds).filter((k) => selectedReviewIds[k]);
    if (ids.length === 0) {
      notify('err', isAr ? 'يرجى تحديد عنصر واحد على الأقل أولاً.' : 'Please select at least one item first.');
      return;
    }
    setBusy(true);
    try {
      const res = await LibraryClient.bulkDecideReviewItems({
        batchId: activeBatchId,
        itemIds: ids,
        decision,
      });
      await loadReviewAndOrganizer(activeBatchId);
      notify(
        'ok',
        isAr
          ? `تم تحديث ${res.updatedCount} عنصر محدد إلى (${decision === 'approved' ? 'معتمد' : 'مرفوض'})${res.skippedCount > 0 ? ` — استُبعد ${res.skippedCount} لوجود تعارض` : ''}.`
          : `Updated ${res.updatedCount} selected item(s) to ${decision}${res.skippedCount > 0 ? ` (${res.skippedCount} skipped due to conflict)` : ''}.`
      );
    } catch (err: unknown) {
      notify('err', String(err));
    } finally {
      setBusy(false);
    }
  };

  const openEditTargetModal = (item: ReviewItemView) => {
    setEditTargetModalItem(item);
    setModalEditName(item.effectiveName || item.userEditedName || item.proposedName);
    const dirVal = item.effectiveDirPath || item.proposedDirPath || '';
    setModalEditDir(dirVal === '/' ? '' : dirVal);
  };

  const handleSaveEditedTarget = async () => {
    if (!editTargetModalItem) return;
    setBusy(true);
    try {
      const nextStatus = editTargetModalItem.decisionStatus === 'executed' ? 'pending' : editTargetModalItem.decisionStatus;
      await LibraryClient.decideReviewItem({
        itemId: editTargetModalItem.id,
        decision: nextStatus,
        userEditedRelPath: '',
        userEditedName: modalEditName.trim(),
        userEditedDirPath: modalEditDir.trim(),
      });
      setEditTargetModalItem(null);
      await loadReviewAndOrganizer(editTargetModalItem.batchId);
      notify('ok', isAr ? 'تم حفظ الاسم والمجلد المقترح الجديد والتحقق الفوري من عدم وجود تعارض.' : 'Updated proposed filename and destination folder with live collision check.');
    } catch (err: unknown) {
      notify('err', String(err));
    } finally {
      setBusy(false);
    }
  };

  const handleOpenItemDetails = async (itemId: string) => {
    try {
      const details = await LibraryClient.getReviewItemDetails(itemId);
      setDetailModalData(details);
    } catch (err: unknown) {
      notify('err', String(err));
    }
  };

  const handleApproveAllSafe = async () => {
    if (!activeBatchId) return;
    setBusy(true);
    try {
      await LibraryClient.approveAllSafeReviewItems(activeBatchId);
      await loadReviewAndOrganizer(activeBatchId);
      notify('ok', isAr ? 'تم اعتماد العناصر الآمنة الخالية من التعارض؛ راجع ملخص العمليات ثم اضغط تنفيذ التغييرات المعتمدة.' : 'Approved all safe conflict-free items; review the manifest summary and click Execute Approved Changes.');
    } catch (err: unknown) {
      notify('err', String(err));
    } finally {
      setBusy(false);
    }
  };

  const handleExecuteBatch = async (batchId: string) => {
    setBusy(true);
    try {
      const res = await LibraryClient.executeApprovedBatch(batchId);
      await loadReviewAndOrganizer(batchId);
      await refreshCore();
      notify(
        'ok',
        isAr
          ? `اكتمل تنفيذ البيان المعتمد [${res.manifestSha256.slice(0, 10)}]: نُفذت ${res.executedCount} عملية بتحقق SHA-256 كامل ونسخة احتياطية مسبقة.`
          : `Executed immutable manifest [${res.manifestSha256.slice(0, 10)}]: ${res.executedCount} SHA-256 verified operations after pre-batch backup.`
      );
    } catch (err: unknown) {
      await loadReviewAndOrganizer(batchId);
      notify('err', String(err));
    } finally {
      setBusy(false);
    }
  };

  const handlePauseExecution = async () => {
    try {
      await LibraryClient.pauseBatchExecution();
      if (activeBatchId) {
        setExecProgress(await LibraryClient.getBatchExecutionProgress(activeBatchId));
      }
      notify('ok', isAr ? 'تم إيقاف التنفيذ مؤقتاً بأمان عند حد العملية الحالية.' : 'Execution safely paused at step boundary.');
    } catch (err: unknown) {
      notify('err', String(err));
    }
  };

  const handleResumeExecution = async () => {
    try {
      await LibraryClient.resumeBatchExecution();
      if (activeBatchId) {
        setExecProgress(await LibraryClient.getBatchExecutionProgress(activeBatchId));
      }
      notify('ok', isAr ? 'تم استئناف التنفيذ الآمن.' : 'Execution resumed.');
    } catch (err: unknown) {
      notify('err', String(err));
    }
  };

  const handleCancelExecution = async () => {
    try {
      await LibraryClient.cancelBatchExecution();
      if (activeBatchId) {
        await loadReviewAndOrganizer(activeBatchId);
      }
      notify('ok', isAr ? 'تم إلغاء العمليات المتبقية بأمان مع بقاء التراجع متاحاً لما نُفذ.' : 'Safely cancelled remaining steps; executed steps remain reversible.');
    } catch (err: unknown) {
      notify('err', String(err));
    }
  };

  const handleRecoverBatch = async (batchId: string) => {
    setBusy(true);
    try {
      const rec = await LibraryClient.recoverInterruptedBatch(batchId);
      await loadReviewAndOrganizer(batchId);
      await refreshCore();
      notify(
        'ok',
        isAr
          ? `اكتملت استعادة الدفعة المتوقفة: حُسمت ${rec.finalizedCount} عملية وأُعيدت جدولة ${rec.requeuedCount} عملية بأمان دون فقد أي ملف.`
          : `Recovered interrupted batch: finalized ${rec.finalizedCount}, safely requeued ${rec.requeuedCount} with zero file loss.`
      );
    } catch (err: unknown) {
      notify('err', String(err));
    } finally {
      setBusy(false);
    }
  };

  const handleRollbackBatch = async (batchId: string) => {
    setBusy(true);
    try {
      const res = await LibraryClient.rollbackBatch(batchId);
      await loadReviewAndOrganizer(batchId);
      await refreshCore();
      notify('ok', isAr ? `تم التراجع الكامل الموثق واستعادة ${res.restoredCount} عنصر إلى مساراتها الأصلية مع التحقق من SHA-256.` : `Verified rollback complete: restored ${res.restoredCount} item(s) to exact original paths with SHA-256 check.`);
    } catch (err: unknown) {
      notify('err', String(err));
    } finally {
      setBusy(false);
    }
  };

  const handleRestoreSingleErrorEntry = async (entryId: string) => {
    setBusy(true);
    try {
      const ev = await LibraryClient.restoreErrorFolderEntry(entryId);
      await loadErrorCenter(activeErrorCategory);
      await refreshCore();
      notify('ok', isAr ? `تم استرجاع (${ev.itemName}) إلى موقعه الأصلي بنجاح.` : `Restored (${ev.itemName}) to its exact original location.`);
    } catch (err: unknown) {
      notify('err', String(err));
    } finally {
      setBusy(false);
    }
  };

  const handleRestoreSelectedErrorEntries = async () => {
    const ids = Object.keys(selectedErrorIds).filter((k) => selectedErrorIds[k]);
    if (ids.length === 0) {
      notify('err', isAr ? 'اختر عنصراً واحداً على الأقل للاسترجاع.' : 'Select at least one item to restore.');
      return;
    }
    setBusy(true);
    try {
      const res = await LibraryClient.restoreSelectedErrorFolderEntries(ids);
      setSelectedErrorIds({});
      await loadErrorCenter(activeErrorCategory);
      await refreshCore();
      notify('ok', isAr ? `تم استرجاع ${res.restoredCount} عنصر محدد إلى مساراتها الأصلية.` : `Restored ${res.restoredCount} selected item(s) to original paths.`);
    } catch (err: unknown) {
      notify('err', String(err));
    } finally {
      setBusy(false);
    }
  };

  const handleRestoreErrorBatch = async (batchId: string) => {
    if (!batchId) return;
    setBusy(true);
    try {
      const res = await LibraryClient.restoreErrorFolderBatch(batchId);
      setSelectedErrorIds({});
      await loadErrorCenter(activeErrorCategory);
      await refreshCore();
      notify('ok', isAr ? `تم استرجاع كامل دفعة الأخطاء (${res.restoredCount} عنصر) بأمان.` : `Restored complete reversible error batch (${res.restoredCount} item(s)).`);
    } catch (err: unknown) {
      notify('err', String(err));
    } finally {
      setBusy(false);
    }
  };

  const startPlayback = async (workId: string, episodeId = '', fileId = '') => {
    setBusy(true);
    try {
      const s = await LibraryClient.startPlaybackSession(workId, episodeId, fileId);
      setSession(s);
      setTab('player');
    } catch (err: unknown) {
      notify('err', String(err));
    } finally {
      setBusy(false);
    }
  };

  const handleSaveProgress = async (overrideStatus = '') => {
    if (!session) return;
    const v = videoRef.current;
    const posMs = v ? Math.floor(v.currentTime * 1000) : session.resumePositionMs;
    const durMs = v && Number.isFinite(v.duration) && v.duration > 0 ? Math.floor(v.duration * 1000) : session.durationMs;
    try {
      const newStatus = await LibraryClient.savePlaybackProgress({
        workId: session.workId,
        episodeId: session.episodeId,
        fileId: session.fileId,
        positionMs: posMs,
        durationMs: durMs,
        watchStatus: overrideStatus,
      });
      setSession({ ...session, watchStatus: newStatus, resumePositionMs: posMs, durationMs: durMs });
      await refreshCore();
    } catch (err: unknown) {
      notify('err', String(err));
    }
  };

  const loadIssues = async (filter: string) => {
    setBusy(true);
    try {
      const list = await LibraryClient.refreshAndListHealthIssues(filter);
      setIssues(list);
      await refreshCore();
    } catch (err: unknown) {
      notify('err', String(err));
    } finally {
      setBusy(false);
    }
  };

  const loadSettingsData = async () => {
    try {
      const [bks, provs, logs] = await Promise.all([
        LibraryClient.listBackups(),
        LibraryClient.listProviders(),
        LibraryClient.listAuditLogs(),
      ]);
      setBackups(bks);
      setProvidersList(provs);
      setAuditLogs(logs);
    } catch (err: unknown) {
      notify('err', String(err));
    }
  };

  const handleCreateBackup = async () => {
    setBusy(true);
    try {
      const bk = await LibraryClient.createVerifiedBackup();
      await loadSettingsData();
      await refreshCore();
      notify('ok', isAr ? `تم إنشاء نسخة احتياطية موثقة (SHA-256: ${bk.sha256Checksum.slice(0, 12)}...).` : `Verified SQLite + Manifest backup created (${bk.sha256Checksum.slice(0, 12)}...).`);
    } catch (err: unknown) {
      notify('err', String(err));
    } finally {
      setBusy(false);
    }
  };

  const handleToggleLanguage = async () => {
    const next: LanguageCode = lang === 'ar' ? 'en' : 'ar';
    setLang(next);
    try {
      const updated = await callPoC(PoCService.SetActiveLocale, 'SetActiveLocale', next);
      setDiag(updated);
    } catch {
      // ignore
    }
  };

  const activeErrorCount = errorEntries.filter((e) => e.entryStatus === 'active').length;

  const navItems: Array<{ key: WorkspaceKey; ar: string; en: string; badge?: number }> = [
    { key: 'dashboard', ar: 'لوحة القيادة السينمائية', en: 'Main Dashboard' },
    { key: 'movies', ar: 'مكتبة الأفلام', en: 'Movies Library', badge: overview?.stats.totalMovies },
    { key: 'series', ar: 'مكتبة المسلسلات والحلقات', en: 'Series & Episodes', badge: overview?.stats.totalSeries },
    { key: 'disks', ar: 'الأقراص ومسارات التخزين', en: 'Drives & Paths', badge: overview?.stats.totalRoots },
    { key: 'review', ar: 'مركز المراجعة والاعتماد', en: 'Smart Review Center', badge: overview?.stats.pendingReviewBatches },
    { key: 'organizer', ar: 'لوحة التنفيذ والتراجع', en: 'Execution & Rollback' },
    { key: 'errors', ar: 'مركز الأخطاء والاسترجاع', en: 'Error Center (الأخطاء)', badge: activeErrorCount },
    { key: 'player', ar: 'المشغل المدمج', en: 'Integrated Player', badge: overview?.stats.watchingNowCount },
    { key: 'reports', ar: 'النواقص وصحة المكتبة', en: 'Reports & Missing' },
    { key: 'ai', ar: 'المساعد الذكي', en: 'AI Assistant' },
    { key: 'settings', ar: 'الإعدادات والنسخ الاحتياطي', en: 'Settings & Backups', badge: overview?.stats.verifiedBackupsCount },
  ];

  return (
    <div className="app-shell" dir={localeCfg.dir}>
      <aside className="app-sidebar" id="directional-sidebar">
        <div className="brand-box">
          <h1>Estiraha Next</h1>
          <p>{isAr ? 'استراحة نكست — إدارة المكتبات السينمائية' : 'Hybrid Cinematic & Admin Media System'}</p>
          <div style={{ marginTop: '8px' }}>
            <span className="badge ok">
              {isAr ? 'وضع المراجعة الإلزامي نشط' : 'Review Safety Gate Active'}
            </span>
          </div>
        </div>

        <nav className="nav-list" aria-label="Main Navigation">
          {navItems.map((item) => (
            <button
              key={item.key}
              id={`nav-${item.key}`}
              className={`nav-btn ${tab === item.key ? 'active' : ''}`}
              onClick={() => setTab(item.key)}
            >
              <span>{isAr ? item.ar : item.en}</span>
              {typeof item.badge === 'number' && <span className="nav-pill">{item.badge}</span>}
            </button>
          ))}
        </nav>

        <div style={{ marginTop: 'auto', display: 'flex', flexDirection: 'column', gap: '8px' }}>
          <button onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}>
            {isAr ? `المظهر: ${theme === 'dark' ? 'داكن سينمائي' : 'فاتح إداري'}` : `Theme: ${theme.toUpperCase()}`}
          </button>
          <button onClick={() => void callPoC(PoCService.HideWindowToTray, 'HideWindowToTray')}>
            {isAr ? 'إخفاء إلى System Tray' : 'Minimize to System Tray'}
          </button>
          <button className="danger" onClick={() => void callPoC(PoCService.RequestCleanQuit, 'RequestCleanQuit')}>
            {isAr ? 'خروج آمن نظيف' : 'Quit Cleanly'}
          </button>
        </div>
      </aside>

      <main className="app-main" id="main-workspace">
        <header className="topbar">
          <div>
            <strong>
              {isAr ? 'استراحة نكست — سطح مكتب Windows الأصلي' : 'Estiraha Next — Native Windows Desktop'}
            </strong>
            <span style={{ marginInlineStart: '10px', fontSize: '0.8rem', color: 'var(--text-muted)' }}>
              {diag ? `SQLite v${diag.schemaVersion} WAL | ${diag.goVersion} | Wails ${diag.wailsVersion}` : ''}
            </span>
          </div>
          <div className="btn-row">
            <button className="primary" onClick={() => void handleManualScan([])} disabled={busy}>
              {isAr ? 'فحص يدوي شامل للمكتبة' : 'Run Manual Scan'}
            </button>
            <button onClick={() => void handleGeneratePlan()} disabled={busy}>
              {isAr ? 'إنشاء خطة تنظيم للمراجعة' : 'Generate Organization Plan'}
            </button>
            <button id="toggle-locale-btn" onClick={() => void handleToggleLanguage()}>
              {isAr ? 'English (LTR)' : 'العربية (RTL)'}
            </button>
          </div>
        </header>

        {banner && (
          <section className="panel" style={{ borderColor: banner.kind === 'err' ? 'var(--status-err)' : 'var(--status-ok)', padding: '10px 16px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '12px' }}>
              <span>{banner.text}</span>
              <button onClick={() => setBanner(null)}>✕</button>
            </div>
          </section>
        )}

        {/* 1. MAIN DASHBOARD */}
        {tab === 'dashboard' && overview && (
          <>
            <section className="stat-grid">
              <article className="stat-card">
                <div className="label">{isAr ? 'إجمالي الأفلام' : 'Total Movies'}</div>
                <div className="value">{overview.stats.totalMovies}</div>
              </article>
              <article className="stat-card">
                <div className="label">{isAr ? 'المسلسلات (الحلقات المتوفرة)' : 'Series (Present Episodes)'}</div>
                <div className="value">
                  {overview.stats.totalSeries} ({overview.stats.totalEpisodesPresent})
                </div>
              </article>
              <article className="stat-card">
                <div className="label">{isAr ? 'ملفات الوسائط المفهرسة' : 'Indexed Media Files'}</div>
                <div className="value">{overview.stats.totalMediaFiles}</div>
                <div className="label">{formatBytes(overview.stats.totalSizeBytes)}</div>
              </article>
              <article className="stat-card">
                <div className="label">{isAr ? 'ملفات .shorts المحمية' : 'Protected .shorts Files'}</div>
                <div className="value">{overview.stats.protectedShortsCount}</div>
              </article>
              <article className="stat-card">
                <div className="label">{isAr ? 'بانتظار مراجعة التعريف' : 'Needs Review'}</div>
                <div className="value">{overview.stats.needsReviewCount}</div>
              </article>
              <article className="stat-card">
                <div className="label">{isAr ? 'نسخ احتياطية موثقة' : 'Verified Backups'}</div>
                <div className="value">{overview.stats.verifiedBackupsCount}</div>
              </article>
            </section>

            <div className="split-layout">
              <section className="panel">
                <h2>{isAr ? 'متابعة المشاهدة (Continue Watching)' : 'Continue Watching'}</h2>
                {overview.continueWatching.length === 0 ? (
                  <p style={{ color: 'var(--text-muted)', fontSize: '0.88rem' }}>
                    {isAr ? 'لا توجد عناصر قيد المشاهدة حالياً. ابدأ تشغيل أي فيلم أو حلقة من المكتبة.' : 'No active playback sessions yet. Play any movie or episode to track resume progress.'}
                  </p>
                ) : (
                  <div className="media-grid">
                    {overview.continueWatching.map((cw) => (
                      <article
                        key={`${cw.workId}-${cw.episodeId}`}
                        className="poster-card"
                        onClick={() => void startPlayback(cw.workId, cw.episodeId, cw.fileId)}
                      >
                        <div className="poster-art">
                          {cw.posterUrl && !brokenPosterUrls[cw.posterUrl] ? (
                            <img src={cw.posterUrl} alt={cw.title} onError={() => markPosterBroken(cw.posterUrl)} />
                          ) : (
                            <span className="badge gold">▶ PLAY</span>
                          )}
                        </div>
                        <div className="poster-body">
                          <div className="poster-title">{cw.title}</div>
                          <span className="badge warn">
                            {Math.floor(cw.resumePositionMs / 1000)}s / {Math.floor(cw.durationMs / 1000)}s
                          </span>
                        </div>
                      </article>
                    ))}
                  </div>
                )}
              </section>

              <section className="panel">
                <h2>{isAr ? 'السياسات المعتمدة النشطة وحالة الأقراص' : 'Binding Phase B Policies & Storage Health'}</h2>
                <div className="btn-row">
                  <span className="badge gold">Classification: Hybrid</span>
                  <span className="badge ok">Cross-Disk Copies: Keep</span>
                  <span className="badge ok">Automation: Review Required</span>
                  <span className="badge ok">.shorts: Protected</span>
                  <span className="badge ok">Scan: Manual Only</span>
                  <span className="badge ok">Backup: Both (SQLite + Journal)</span>
                </div>
                {overview.roots.some((r) => r.passiveChangeAlert) && (
                  <div className="badge warn">
                    {isAr ? 'تنبيه مراقبة سلبي: تم رصد تغيّر في أحد المجلدات؛ اضغط فحص يدوي عند الرغبة.' : 'Passive Monitor Alert: Folder change detected; run Manual Scan when ready.'}
                  </div>
                )}
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>{isAr ? 'القرص' : 'Drive'}</th>
                      <th>{isAr ? 'الحالة' : 'State'}</th>
                      <th>{isAr ? 'المساحة الحرة' : 'Free Space'}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {overview.disks.map((d) => (
                      <tr key={d.id}>
                        <td>{d.label} ({d.mountPath})</td>
                        <td>
                          <span className={`badge ${d.connectionState === 'online' ? 'ok' : 'err'}`}>
                            {d.connectionState}
                          </span>
                        </td>
                        <td>{formatBytes(d.freeBytes)} / {formatBytes(d.totalBytes)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </section>
            </div>
          </>
        )}

        {/* 2. MOVIES LIBRARY */}
        {tab === 'movies' && (
          <div className={selectedMovie ? 'split-layout' : ''}>
            <section className="panel">
              <h2>{isAr ? 'مكتبة الأفلام السينمائية (Movie Name YYYY)' : 'Movies Library (Canonical: Movie Name YYYY)'}</h2>
              <div className="btn-row">
                <input
                  className="input"
                  style={{ flex: 1, minWidth: '220px' }}
                  placeholder={isAr ? 'بحث فوري FTS5 بالعربية أو الإنجليزية...' : 'Instant FTS5 search in Arabic or English...'}
                  value={movieQuery}
                  onChange={(e) => setMovieQuery(e.target.value)}
                />
                <select className="input" value={movieStatusFilter} onChange={(e) => setMovieStatusFilter(e.target.value)}>
                  <option value="all">{isAr ? 'كل حالات التعريف' : 'All Statuses'}</option>
                  <option value="identified">{isAr ? 'معرّف تلقائياً' : 'Identified'}</option>
                  <option value="locked_by_user">{isAr ? 'مؤكد يدوياً' : 'Locked by User'}</option>
                  <option value="needs_review">{isAr ? 'يحتاج مراجعة' : 'Needs Review'}</option>
                </select>
                <select
                  className="input"
                  value={movieOriginFilter}
                  onChange={(e) => setMovieOriginFilter(e.target.value as 'all' | 'ar' | 'foreign')}
                >
                  <option value="all">{isAr ? 'عربي + أجنبي (Hybrid)' : 'Arabic + Foreign'}</option>
                  <option value="ar">{isAr ? 'أفلام عربية فقط' : 'Arabic Origin Only'}</option>
                  <option value="foreign">{isAr ? 'أفلام أجنبية فقط' : 'Foreign Origin Only'}</option>
                </select>
                <button className="primary" onClick={() => void loadMovies()}>
                  {isAr ? 'تطبيق البحث' : 'Search'}
                </button>
              </div>

              <div className="media-grid">
                {moviesList.map((m) => (
                  <article key={m.id} className="poster-card" onClick={() => void openMovieDetail(m.id)}>
                    <div className="poster-art">
                      {m.posterUrl && !brokenPosterUrls[m.posterUrl] ? (
                        <img
                          src={m.posterUrl}
                          alt={m.canonicalFolderName}
                          onError={() => markPosterBroken(m.posterUrl)}
                        />
                      ) : (
                        <span className="badge gold">{m.productionYear || '????'}</span>
                      )}
                    </div>
                    <div className="poster-body">
                      <div className="poster-title">{m.canonicalFolderName}</div>
                      <div className="btn-row">
                        {m.bestResolution && <span className="badge ok">{m.bestResolution}</span>}
                        {m.shortsCount > 0 && <span className="badge gold">.shorts ({m.shortsCount})</span>}
                        {m.identificationStatus === 'needs_review' && (
                          <span className="badge warn">{isAr ? 'مراجعة' : 'Review'}</span>
                        )}
                      </div>
                      <div style={{ fontSize: '0.76rem', color: 'var(--text-muted)' }}>
                        {m.diskLabels} • {m.versionCount}v • {m.subtitleCount} sub
                      </div>
                    </div>
                  </article>
                ))}
              </div>
            </section>

            {selectedMovie && (
              <section className="panel">
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <h2>{selectedMovie.canonicalFolderName}</h2>
                  <button onClick={() => setSelectedMovie(null)}>✕</button>
                </div>
                <div className="btn-row">
                  <button className="primary" onClick={() => void startPlayback(selectedMovie.id, '', '')}>
                    ▶ {isAr ? 'تشغيل أفضل نسخة بالمشغل المدمج' : 'Play Best Version in Integrated Player'}
                  </button>
                </div>

                <div className="form-grid">
                  <label>
                    <div style={{ fontSize: '0.78rem', color: 'var(--text-muted)' }}>
                      {isAr ? 'العنوان الأصلي المعتمد للتسمية' : 'Canonical Base Title'}
                    </div>
                    <input className="input" style={{ width: '100%' }} value={editMovieTitle} onChange={(e) => setEditMovieTitle(e.target.value)} />
                  </label>
                  <label>
                    <div style={{ fontSize: '0.78rem', color: 'var(--text-muted)' }}>
                      {isAr ? 'سنة الإنتاج (بدون أقواس)' : 'Production Year (No parentheses)'}
                    </div>
                    <input
                      type="number"
                      className="input"
                      style={{ width: '100%' }}
                      value={editMovieYear}
                      onChange={(e) => setEditMovieYear(Number(e.target.value))}
                    />
                  </label>
                  <label>
                    <div style={{ fontSize: '0.78rem', color: 'var(--text-muted)' }}>{isAr ? 'الاسم بالعربية' : 'Arabic Title'}</div>
                    <input className="input" style={{ width: '100%' }} value={editMovieAr} onChange={(e) => setEditMovieAr(e.target.value)} />
                  </label>
                  <label>
                    <div style={{ fontSize: '0.78rem', color: 'var(--text-muted)' }}>{isAr ? 'الاسم بالإنجليزية' : 'English Title'}</div>
                    <input className="input" style={{ width: '100%' }} value={editMovieEn} onChange={(e) => setEditMovieEn(e.target.value)} />
                  </label>
                  <label>
                    <div style={{ fontSize: '0.78rem', color: 'var(--text-muted)' }}>{isAr ? 'التقييم الشخصي (/10)' : 'Personal Rating (/10)'}</div>
                    <input
                      type="number"
                      step="0.5"
                      min="0"
                      max="10"
                      className="input"
                      style={{ width: '100%' }}
                      value={editMovieRating}
                      onChange={(e) => setEditMovieRating(Number(e.target.value))}
                    />
                  </label>
                  <label style={{ display: 'flex', alignItems: 'center', gap: '8px', paddingTop: '18px' }}>
                    <input type="checkbox" checked={editMovieIsAr} onChange={(e) => setEditMovieIsAr(e.target.checked)} />
                    <span>{isAr ? 'عمل عربي أصلي (احتفظ بالاسم العربي دون ترجمة مخترعة)' : 'Arabic Origin (Keep authentic Arabic title)'}</span>
                  </label>
                </div>
                <textarea
                  className="input"
                  rows={2}
                  placeholder={isAr ? 'ملاحظات شخصية...' : 'Personal notes...'}
                  value={editMovieNote}
                  onChange={(e) => setEditMovieNote(e.target.value)}
                />
                <button className="primary" onClick={() => void handleSaveMovieMetadata()} disabled={busy}>
                  {isAr ? 'حفظ التعديلات وتثبيت الاسم الرسمي' : 'Save & Lock Canonical Metadata'}
                </button>

                <h3 style={{ fontSize: '0.9rem', marginTop: '8px' }}>
                  {isAr ? 'الملفات المرتبطة (فيديو، ترجمة، بوستر، .shorts محمية)' : 'Linked Files (Video, Subtitles, Poster, Protected .shorts)'}
                </h3>
                <table className="admin-table">
                  <thead>
                    <tr>
                      <th>{isAr ? 'النوع' : 'Kind'}</th>
                      <th>{isAr ? 'الملف والمسار' : 'File & Path'}</th>
                      <th>{isAr ? 'الجودة/الحجم' : 'Quality/Size'}</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {(selectedMovie.files || []).map((f) => (
                      <tr key={f.fileId}>
                        <td>
                          <span className={`badge ${f.protectedShorts ? 'gold' : f.integrityStatus === 'healthy' ? 'ok' : 'err'}`}>
                            {f.protectedShorts ? '.shorts PROTECTED' : f.fileKind}
                          </span>
                        </td>
                        <td className="mono">{f.fullPath}</td>
                        <td>
                          {f.resolutionLabel ? `${f.resolutionLabel} ${f.videoCodec} • ` : ''}
                          {formatBytes(f.sizeBytes)}
                        </td>
                        <td>
                          {f.fileKind === 'video' && (
                            <button onClick={() => void startPlayback(selectedMovie.id, '', f.fileId)}>▶</button>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </section>
            )}
          </div>
        )}

        {/* 3. SERIES LIBRARY */}
        {tab === 'series' && (
          <div className={selectedSeries ? 'split-layout' : ''}>
            <section className="panel">
              <h2>{isAr ? 'مكتبة المسلسلات والمواسم والحلقات (Series Name / S01 / 01.mp4)' : 'Series Library (Series Name / S01 / 01.mp4 — No Year)'}</h2>
              <div className="btn-row">
                <input
                  className="input"
                  style={{ flex: 1, minWidth: '220px' }}
                  placeholder={isAr ? 'ابحث في المسلسلات...' : 'Search series...'}
                  value={seriesQuery}
                  onChange={(e) => setSeriesQuery(e.target.value)}
                />
                <label style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                  <input
                    type="checkbox"
                    checked={seriesMissingOnly}
                    onChange={(e) => setSeriesMissingOnly(e.target.checked)}
                  />
                  <span>{isAr ? 'يحتوي حلقات/مواسم مفقودة فقط' : 'With Missing Episodes/Seasons Only'}</span>
                </label>
                <button className="primary" onClick={() => void loadSeries()}>
                  {isAr ? 'تحديث القائمة' : 'Filter'}
                </button>
              </div>

              <div className="media-grid">
                {seriesList.map((s) => (
                  <article key={s.id} className="poster-card" onClick={() => void openSeriesDetail(s.id)}>
                    <div className="poster-art">
                      {s.posterUrl && !brokenPosterUrls[s.posterUrl] ? (
                        <img
                          src={s.posterUrl}
                          alt={s.canonicalFolderName}
                          onError={() => markPosterBroken(s.posterUrl)}
                        />
                      ) : (
                        <span className="badge gold">{s.seasonCount} Seasons</span>
                      )}
                    </div>
                    <div className="poster-body">
                      <div className="poster-title">{s.canonicalFolderName}</div>
                      <div className="btn-row">
                        <span className="badge ok">{s.presentEpisodeCount} EP</span>
                        {s.missingEpisodeCount > 0 && (
                          <span className="badge err">
                            {isAr ? `${s.missingEpisodeCount} حلقة مفقودة` : `${s.missingEpisodeCount} Missing EP`}
                          </span>
                        )}
                        {s.missingSeasonNumbers && s.missingSeasonNumbers.length > 0 && (
                          <span className="badge warn">
                            {isAr ? `موسم مفقود S0${s.missingSeasonNumbers[0]}` : `Missing S0${s.missingSeasonNumbers[0]}`}
                          </span>
                        )}
                      </div>
                    </div>
                  </article>
                ))}
              </div>
            </section>

            {selectedSeries && (
              <section className="panel">
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <h2>{selectedSeries.canonicalFolderName}</h2>
                  <button onClick={() => setSelectedSeries(null)}>✕</button>
                </div>
                <div className="btn-row">
                  <input
                    className="input"
                    style={{ flex: 1 }}
                    value={editSeriesTitle}
                    onChange={(e) => setEditSeriesTitle(e.target.value)}
                  />
                  <button className="primary" onClick={() => void handleSaveSeriesMetadata()} disabled={busy}>
                    {isAr ? 'حفظ اسم المسلسل' : 'Save Series Name'}
                  </button>
                </div>

                {(selectedSeries.seasons || []).map((sn) => (
                  <div key={sn.id} className="panel" style={{ background: 'var(--bg-secondary)' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '8px' }}>
                      <strong>
                        {sn.canonicalFolderCode} — {isAr ? `المتوفر: ${sn.presentEpisodeCount}` : `Present: ${sn.presentEpisodeCount}`}
                        {sn.missingEpisodeNumbers.length > 0 && (
                          <span className="badge err" style={{ marginInlineStart: '8px' }}>
                            {isAr ? `مفقود: ${sn.missingEpisodeNumbers.join(', ')}` : `Missing: ${sn.missingEpisodeNumbers.join(', ')}`}
                          </span>
                        )}
                      </strong>
                      <div className="btn-row">
                        <span style={{ fontSize: '0.78rem', color: 'var(--text-muted)' }}>
                          {isAr ? 'إجمالي الحلقات المتوقع:' : 'Expected EPs:'}
                        </span>
                        <input
                          type="number"
                          className="input"
                          style={{ width: '74px' }}
                          defaultValue={sn.expectedEpisodeCount}
                          onBlur={(e) => {
                            void LibraryClient.setSeasonExpectedEpisodes(sn.id, Number(e.target.value)).then(() =>
                              openSeriesDetail(selectedSeries.id)
                            );
                          }}
                        />
                      </div>
                    </div>
                    <table className="admin-table">
                      <thead>
                        <tr>
                          <th>#</th>
                          <th>{isAr ? 'الحالة' : 'Status'}</th>
                          <th>{isAr ? 'أفضل نسخة (Best Version)' : 'Best Version'}</th>
                          <th></th>
                        </tr>
                      </thead>
                      <tbody>
                        {sn.episodes.map((ep) => (
                          <tr key={ep.id}>
                            <td className="mono">{ep.canonicalFileStem}</td>
                            <td>
                              {ep.isMissing ? (
                                <span className="badge err">{isAr ? 'حلقة مفقودة' : 'MISSING'}</span>
                              ) : (
                                <span className="badge ok">{ep.watchStatus}</span>
                              )}
                              {ep.hasShorts && <span className="badge gold" style={{ marginInlineStart: '6px' }}>.shorts</span>}
                            </td>
                            <td>
                              {ep.isMissing
                                ? '—'
                                : `${ep.bestResolution} (${ep.versions.length}v, ${ep.subtitleCount} sub)`}
                            </td>
                            <td>
                              {!ep.isMissing && (
                                <button
                                  className="primary"
                                  onClick={() => void startPlayback(selectedSeries.id, ep.id, ep.bestFileId)}
                                >
                                  ▶
                                </button>
                              )}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ))}
              </section>
            )}
          </div>
        )}

        {/* 4. DISKS & STORAGE PATHS */}
        {tab === 'disks' && overview && (
          <section className="panel">
            <h2>{isAr ? 'الأقراص ومسارات المكتبة (تنظيم متعدد الأقراص حسب نوع المحتوى)' : 'Drives & Library Roots (Multi-Disk By Content Type)'}</h2>
            <div className="form-grid">
              <div style={{ display: 'flex', gap: '6px' }}>
                <input
                  className="input"
                  style={{ flex: 1 }}
                  placeholder={isAr ? 'مسار المجلد (مثال: D:\\Movies)' : 'Folder path (e.g. D:\\Movies)'}
                  value={newRootPath}
                  onChange={(e) => setNewRootPath(e.target.value)}
                />
                <button onClick={() => void handleBrowseFolder()}>{isAr ? 'استعراض...' : 'Browse...'}</button>
              </div>
              <select className="input" value={newRootCategory} onChange={(e) => setNewRootCategory(e.target.value)}>
                <option value="movies">{isAr ? 'أفلام (Movies)' : 'Movies'}</option>
                <option value="series">{isAr ? 'مسلسلات (Series)' : 'Series'}</option>
                <option value="anime">{isAr ? 'أنمي (Anime)' : 'Anime'}</option>
                <option value="cartoons">{isAr ? 'كرتون (Cartoons)' : 'Cartoons'}</option>
                <option value="documentaries">{isAr ? 'وثائقيات (Documentaries)' : 'Documentaries'}</option>
                <option value="programs">{isAr ? 'برامج تلفزيونية (TV Programs)' : 'TV Programs'}</option>
                <option value="mixed">{isAr ? 'مختلط (Mixed)' : 'Mixed'}</option>
              </select>
              <input
                className="input"
                placeholder={isAr ? 'اسم وصفي للمسار (اختياري)' : 'Display Name (optional)'}
                value={newRootName}
                onChange={(e) => setNewRootName(e.target.value)}
              />
              <input
                className="input"
                placeholder={isAr ? 'تسمية القرص (مثال: Cinema Archive 1)' : 'Drive Label (optional)'}
                value={newDiskLabel}
                onChange={(e) => setNewDiskLabel(e.target.value)}
              />
            </div>
            <div className="btn-row">
              <button className="primary" onClick={() => void handleAddRoot()} disabled={busy}>
                + {isAr ? 'تسجيل المسار في قاعدة البيانات' : 'Register Library Root'}
              </button>
              <label style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                <input
                  type="checkbox"
                  checked={scanComputeHash}
                  onChange={(e) => setScanComputeHash(e.target.checked)}
                />
                <span>{isAr ? 'حساب البصمة الكاملة SHA-256 أثناء الفحص اليدوي' : 'Compute full SHA-256 during Manual Scan'}</span>
              </label>
              <button onClick={() => void refreshCore()}>
                {isAr ? 'تحديث حالة اتصال الأقراص' : 'Refresh Drive Connectivity'}
              </button>
              {overview.scanRunning && (
                <button className="danger" onClick={() => void LibraryClient.cancelActiveScan()}>
                  {isAr ? 'إلغاء الفحص الجاري بأمان' : 'Cancel Active Scan Safely'}
                </button>
              )}
            </div>

            {lastScanResult && (
              <div className="badge ok">
                {isAr
                  ? `آخر فحص يدوي: ${lastScanResult.scan.filesDiscovered} ملف | ${lastScanResult.scan.newFiles} جديد | ${lastScanResult.scan.corruptedDetected} تالف | المدة ${lastScanResult.scan.durationMs}ms`
                  : `Last Scan: ${lastScanResult.scan.filesDiscovered} files | ${lastScanResult.scan.newFiles} new | ${lastScanResult.scan.corruptedDetected} corrupted | ${lastScanResult.scan.durationMs}ms`}
              </div>
            )}

            <table className="admin-table">
              <thead>
                <tr>
                  <th>{isAr ? 'الاسم' : 'Display Name'}</th>
                  <th>{isAr ? 'التصنيف' : 'Category'}</th>
                  <th>{isAr ? 'المسار الكامل' : 'Full Path'}</th>
                  <th>{isAr ? 'الحالة' : 'Status'}</th>
                  <th>{isAr ? 'الملفات' : 'Files'}</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {overview.roots.map((r) => (
                  <tr key={r.id}>
                    <td><strong>{r.displayName}</strong></td>
                    <td><span className="badge gold">{r.contentCategory}</span></td>
                    <td className="mono">{r.fullPath}</td>
                    <td>
                      <span className={`badge ${r.isAccessible ? 'ok' : 'err'}`}>
                        {r.isAccessible ? (isAr ? 'متصل' : 'Online') : (isAr ? 'منفصل (محفوظ)' : 'Offline (Preserved)')}
                      </span>
                      {r.passiveChangeAlert && (
                        <span className="badge warn" style={{ marginInlineStart: '6px' }}>
                          {isAr ? 'تغيّر مرصود' : 'Changed'}
                        </span>
                      )}
                    </td>
                    <td>{r.indexedFileCount}</td>
                    <td className="btn-row">
                      <button onClick={() => void handleManualScan([r.id])} disabled={busy || !r.isAccessible}>
                        {isAr ? 'فحص هذا المسار' : 'Scan Root'}
                      </button>
                      <button
                        className="danger"
                        onClick={() => void LibraryClient.removeLibraryRoot(r.id).then(() => refreshCore())}
                      >
                        {isAr ? 'إزالة الربط' : 'Unlink'}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
        )}

        {/* 5. SMART REVIEW CENTER (Batch 3 Professional Review Center) */}
        {tab === 'review' && (() => {
          const visibleItems = batchItems.filter((it) =>
            reviewFilterStatus === 'all' ? true : it.decisionStatus === reviewFilterStatus
          );
          const selectedCount = Object.values(selectedReviewIds).filter(Boolean).length;
          const activeBatchObj = batches.find((b) => b.id === activeBatchId) ?? null;

          return (
            <section className="panel" id="review-center-panel">
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '10px' }}>
                <div>
                  <h2>
                    {isAr
                      ? 'مركز المراجعة الاحترافي والاعتماد الإلزامي (لا يُعدّل أي ملف بدون موافقتك)'
                      : 'Professional Review Center (Zero File Changes Without Explicit Approval)'}
                  </h2>
                  <p style={{ fontSize: '0.8rem', color: 'var(--text-muted)', marginTop: '3px' }}>
                    {isAr
                      ? 'راجع الاسم الأصلي والمقترح والمجلدات ونوع الملف والعمل المرتبط وفحوص السلامة والتعارضات قبل الاعتماد.'
                      : 'Inspect original/proposed filenames, directories, file types, linked works, safety checks, and conflicts before approval.'}
                  </p>
                </div>
                <div className="btn-row">
                  <button className="primary" onClick={() => void handleGeneratePlan()} disabled={busy}>
                    {isAr ? 'توليد خطة معاينة جديدة' : 'Generate Fresh Plan'}
                  </button>
                  <button onClick={() => void handleApproveAllSafe()} disabled={busy || !activeBatchId}>
                    ✓ {isAr ? 'اعتماد كل العمليات الآمنة' : 'Approve All Safe Items'}
                  </button>
                  <button
                    className="cta-execute-btn"
                    style={{ padding: '8px 16px', fontSize: '0.88rem' }}
                    onClick={() => setTab('organizer')}
                  >
                    {isAr ? 'الانتقال للوحة التنفيذ (تنفيذ التغييرات المعتمدة) ←' : 'Go to Execution Dashboard →'}
                  </button>
                </div>
              </div>

              <div className="btn-row" style={{ justifyContent: 'space-between', background: 'var(--bg-secondary)', padding: '10px 14px', borderRadius: '10px', border: '1px solid var(--border-color)' }}>
                <div className="btn-row">
                  <span style={{ fontSize: '0.84rem', color: 'var(--text-muted)' }}>{isAr ? 'الدفعة النشطة:' : 'Active Batch:'}</span>
                  <select
                    id="review-batch-select"
                    className="input"
                    value={activeBatchId}
                    onChange={(e) => {
                      setSelectedReviewIds({});
                      void loadReviewAndOrganizer(e.target.value);
                    }}
                  >
                    {batches.map((b) => (
                      <option key={b.id} value={b.id}>
                        {b.title} [{b.batchStatus}] ({b.approvedItems}/{b.totalItems})
                      </option>
                    ))}
                  </select>

                  <select
                    id="review-status-filter"
                    className="input"
                    value={reviewFilterStatus}
                    onChange={(e) => setReviewFilterStatus(e.target.value)}
                  >
                    <option value="all">{isAr ? 'كل الحالات' : 'All Statuses'} ({batchItems.length})</option>
                    <option value="pending">{isAr ? 'بانتظار القرار (Pending)' : 'Pending'}</option>
                    <option value="approved">{isAr ? 'معتمد للتنفيذ (Approved)' : 'Approved'}</option>
                    <option value="rejected">{isAr ? 'مرفوض / مستبعد (Rejected)' : 'Rejected'}</option>
                    <option value="deferred">{isAr ? 'مؤجل للمراجعة اليدوية (Deferred)' : 'Deferred'}</option>
                    <option value="executed">{isAr ? 'تم تنفيذه (Executed)' : 'Executed'}</option>
                  </select>
                </div>

                <div className="btn-row">
                  <span className="badge gold">
                    {isAr ? `المحدد: ${selectedCount}` : `Selected: ${selectedCount}`}
                  </span>
                  <button id="btn-select-all-review" onClick={() => handleSelectAllReview(visibleItems)} disabled={visibleItems.length === 0}>
                    {isAr ? 'تحديد الكل' : 'Select All'}
                  </button>
                  <button id="btn-deselect-all-review" onClick={() => handleDeselectAllReview()} disabled={selectedCount === 0}>
                    {isAr ? 'إلغاء تحديد الكل' : 'Deselect All'}
                  </button>
                  <button
                    id="btn-approve-selected-review"
                    className="primary"
                    disabled={busy || selectedCount === 0}
                    onClick={() => void handleBulkReviewDecision('approved')}
                  >
                    ✓ {isAr ? 'اعتماد المحدد' : 'Approve Selected'}
                  </button>
                  <button
                    id="btn-reject-selected-review"
                    className="danger"
                    disabled={busy || selectedCount === 0}
                    onClick={() => void handleBulkReviewDecision('rejected')}
                  >
                    ✕ {isAr ? 'رفض المحدد' : 'Reject Selected'}
                  </button>
                </div>
              </div>

              {activeBatchObj && (
                <div className="btn-row" style={{ fontSize: '0.8rem' }}>
                  <span className="badge gold">{isAr ? `إجمالي العناصر: ${activeBatchObj.totalItems}` : `Total: ${activeBatchObj.totalItems}`}</span>
                  <span className="badge ok">{isAr ? `معتمد: ${activeBatchObj.approvedItems}` : `Approved: ${activeBatchObj.approvedItems}`}</span>
                  <span className="badge warn">{isAr ? `معلق: ${activeBatchObj.pendingItems}` : `Pending: ${activeBatchObj.pendingItems}`}</span>
                  <span className="badge err">{isAr ? `مرفوض: ${activeBatchObj.rejectedItems ?? 0}` : `Rejected: ${activeBatchObj.rejectedItems ?? 0}`}</span>
                  <span className="badge ok">{isAr ? `منفذ: ${activeBatchObj.executedItems}` : `Executed: ${activeBatchObj.executedItems}`}</span>
                </div>
              )}

              <div className="table-scroll">
                <table className="admin-table" id="review-items-table">
                  <thead>
                    <tr>
                      <th className="checkbox-cell">☑</th>
                      <th>{isAr ? 'الاسم الأصلي والمجلد الأصلي' : 'Original Filename & Directory'}</th>
                      <th>{isAr ? 'الاسم المقترح والوجهة المقترحة' : 'Proposed Filename & Destination'}</th>
                      <th>{isAr ? 'نوع العنصر والعمل المرتبط' : 'File Type & Associated Work'}</th>
                      <th>{isAr ? 'السبب ومستوى الثقة' : 'Reason & Confidence'}</th>
                      <th>{isAr ? 'فحص السلامة والتعارضات' : 'Safety Validation & Conflict'}</th>
                      <th>{isAr ? 'الإجراءات والقرار' : 'Actions & Decision'}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {visibleItems.map((it) => {
                      const checked = Boolean(selectedReviewIds[it.id]);
                      const kindBadge = it.protectedShortsFile
                        ? '.shorts (Protected)'
                        : it.mediaFileKind || it.itemFileKind || 'file';
                      return (
                        <tr key={it.id}>
                          <td className="checkbox-cell">
                            <input
                              type="checkbox"
                              checked={checked}
                              disabled={it.decisionStatus === 'executed'}
                              onChange={(e) =>
                                setSelectedReviewIds((prev) => ({ ...prev, [it.id]: e.target.checked }))
                              }
                            />
                          </td>
                          <td>
                            <div className="mono" style={{ fontWeight: 700 }}>{it.oldName}</div>
                            <div className="mono" style={{ fontSize: '0.73rem', color: 'var(--text-muted)', marginTop: '2px' }}>
                              {isAr ? 'المجلد الأصلي: ' : 'Orig Dir: '}[{it.diskLabel}] {it.oldDirPath || '/'}
                            </div>
                          </td>
                          <td>
                            <div className="mono" style={{ fontWeight: 700, color: 'var(--accent-gold)' }}>
                              {it.effectiveName || it.proposedName}
                            </div>
                            <div className="mono" style={{ fontSize: '0.73rem', color: 'var(--text-muted)', marginTop: '2px' }}>
                              {isAr ? 'الوجهة المقترحة: ' : 'Dest Dir: '}{it.effectiveDirPath || it.proposedDirPath || '/'}
                            </div>
                            {it.userEditedRelPath && (
                              <span className="badge gold" style={{ marginTop: '4px' }}>
                                {isAr ? 'مُعدّل يدوياً' : 'User Edited'}
                              </span>
                            )}
                          </td>
                          <td>
                            <div className="btn-row">
                              <span className={`badge ${it.protectedShortsFile ? 'gold' : 'ok'}`}>{kindBadge}</span>
                              <span className="badge">{it.actionKind}</span>
                            </div>
                            <div style={{ fontSize: '0.79rem', marginTop: '4px' }}>
                              {it.workTitle
                                ? `${isAr ? 'مرتبط بـ: ' : 'Work: '}${it.workTitle}`
                                : it.errorCategoryCode
                                  ? `${isAr ? 'تصنيف الأخطاء: ' : 'Error Cat: '}${it.errorCategoryCode}`
                                  : isAr ? 'غير مرتبط بعمل' : 'Unlinked'}
                            </div>
                          </td>
                          <td>
                            <div style={{ fontSize: '0.81rem', lineHeight: 1.4 }}>{it.reasonDescription}</div>
                            <div style={{ marginTop: '4px' }}>
                              <span className={`badge ${it.confidenceScore >= 0.85 ? 'ok' : 'warn'}`}>
                                {isAr ? `الثقة: ${Math.round(it.confidenceScore * 100)}%` : `Confidence: ${Math.round(it.confidenceScore * 100)}%`}
                              </span>
                            </div>
                          </td>
                          <td>
                            {it.conflictDetected ? (
                              <span className="badge err">
                                ⚠ {it.conflictDetails || (isAr ? 'تعارض يتطلب مراجعة' : 'Conflict Detected')}
                              </span>
                            ) : (
                              <span className="badge ok">
                                ✓ {isAr ? 'آمن — بلا تعارض' : 'Safe — No Collision'}
                              </span>
                            )}
                            <div style={{ display: 'flex', flexWrap: 'wrap', gap: '4px', marginTop: '5px' }}>
                              {(it.safetyChecks ?? []).slice(0, 3).map((chk) => (
                                <span key={chk} className="mono" style={{ fontSize: '0.69rem', color: 'var(--text-muted)' }}>
                                  [{chk}]
                                </span>
                              ))}
                            </div>
                          </td>
                          <td>
                            <div className="btn-row">
                              <span
                                className={`badge ${
                                  it.decisionStatus === 'approved' || it.decisionStatus === 'executed'
                                    ? 'ok'
                                    : it.decisionStatus === 'rejected'
                                      ? 'err'
                                      : 'warn'
                                }`}
                              >
                                {it.decisionStatus}
                              </span>
                              {it.decisionStatus !== 'executed' && (
                                <>
                                  <button
                                    className="primary"
                                    title={isAr ? 'اعتماد' : 'Approve'}
                                    disabled={it.conflictDetected}
                                    onClick={() => void handleDecideItem(it, 'approved')}
                                  >
                                    ✓
                                  </button>
                                  <button
                                    className="danger"
                                    title={isAr ? 'رفض' : 'Reject'}
                                    onClick={() => void handleDecideItem(it, 'rejected')}
                                  >
                                    ✕
                                  </button>
                                  <button
                                    title={isAr ? 'تعديل الاسم المقترح أو الوجهة' : 'Edit Proposed Name / Destination'}
                                    onClick={() => openEditTargetModal(it)}
                                  >
                                    ✎ {isAr ? 'تعديل الاسم/الوجهة' : 'Edit'}
                                  </button>
                                </>
                              )}
                              <button
                                title={isAr ? 'عرض تفاصيل الملف الكاملة' : 'View File Details'}
                                onClick={() => void handleOpenItemDetails(it.id)}
                              >
                                ℹ {isAr ? 'التفاصيل' : 'Details'}
                              </button>
                            </div>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </section>
          );
        })()}

        {/* 6. ORGANIZATION EXECUTION DASHBOARD, MANIFEST SUMMARY & ROLLBACK UI */}
        {tab === 'organizer' && (() => {
          const activeBatchObj = batches.find((b) => b.id === activeBatchId) ?? batches[0] ?? null;
          const canExecuteActive =
            Boolean(activeBatchObj) &&
            !busy &&
            (manifestSummary?.totalApprovedOperations ?? activeBatchObj?.approvedItems ?? 0) > 0 &&
            activeBatchObj?.batchStatus !== 'completed';
          const hasInterrupted = (execProgress?.interruptedEntries ?? 0) > 0 || activeBatchObj?.batchStatus === 'halted_on_interrupt';

          return (
            <>
              <section className="panel" id="execution-dashboard-panel">
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '14px' }}>
                  <div>
                    <h2>
                      {isAr
                        ? 'لوحة التنفيذ الآمن الموثق ببيان العمليات (Immutable SHA-256 Manifest) والتراجع الكامل'
                        : 'Safe Organization Execution Dashboard & 1-Click Verified Rollback'}
                    </h2>
                    <p style={{ fontSize: '0.8rem', color: 'var(--text-muted)', marginTop: '3px' }}>
                      {isAr
                        ? 'يُنفذ النظام فقط العمليات المعتمدة صراحةً داخل بيان غير قابل للتغيير بعد أخذ نسخة احتياطية مسبقة والتحقق من SHA-256 ومنع أي استبدال.'
                        : 'Executes strictly user-approved operations locked in an immutable SHA-256 manifest after a verified pre-batch backup with zero overwrites.'}
                    </p>
                  </div>

                  <div className="btn-row">
                    <button
                      id="btn-execute-approved-changes"
                      className="cta-execute-btn"
                      disabled={!canExecuteActive}
                      onClick={() => activeBatchObj && void handleExecuteBatch(activeBatchObj.id)}
                    >
                      {isAr ? 'تنفيذ التغييرات المعتمدة' : 'Execute Approved Changes (تنفيذ التغييرات المعتمدة)'}
                    </button>

                    {activeBatchObj && activeBatchObj.executedItems > 0 && activeBatchObj.batchStatus !== 'rolled_back' && (
                      <button
                        id="btn-rollback-active-batch"
                        className="danger"
                        disabled={busy}
                        onClick={() => void handleRollbackBatch(activeBatchObj.id)}
                      >
                        ↺ {isAr ? 'تراجع كامل موثق (Rollback)' : 'Verified Batch Rollback'}
                      </button>
                    )}
                  </div>
                </div>

                <div className="btn-row" style={{ justifyContent: 'space-between', background: 'var(--bg-secondary)', padding: '10px 14px', borderRadius: '10px', border: '1px solid var(--border-color)' }}>
                  <div className="btn-row">
                    <span style={{ fontSize: '0.84rem', color: 'var(--text-muted)' }}>
                      {isAr ? 'دفعة التنفيذ المحددة:' : 'Target Execution Batch:'}
                    </span>
                    <select
                      className="input"
                      value={activeBatchId}
                      onChange={(e) => void loadReviewAndOrganizer(e.target.value)}
                    >
                      {batches.map((b) => (
                        <option key={b.id} value={b.id}>
                          {b.title} [{b.batchStatus}] ({b.approvedItems}/{b.totalItems})
                        </option>
                      ))}
                    </select>
                    {manifestSummary?.manifestSha256 && (
                      <span className="badge gold mono">
                        Manifest SHA-256: {manifestSummary.manifestSha256.slice(0, 14)}…
                      </span>
                    )}
                  </div>

                  <div className="btn-row">
                    <button
                      id="btn-pause-execution"
                      disabled={!execProgress?.running || execProgress?.paused}
                      onClick={() => void handlePauseExecution()}
                    >
                      ⏸ {isAr ? 'إيقاف مؤقت آمن' : 'Pause'}
                    </button>
                    <button
                      id="btn-resume-execution"
                      disabled={!execProgress?.paused}
                      onClick={() => void handleResumeExecution()}
                    >
                      ▶ {isAr ? 'استئناف' : 'Resume'}
                    </button>
                    <button
                      id="btn-cancel-execution"
                      className="danger"
                      disabled={!execProgress?.running}
                      onClick={() => void handleCancelExecution()}
                    >
                      ⏹ {isAr ? 'إلغاء آمن للعمليات المتبقية' : 'Safe Cancel'}
                    </button>
                    {activeBatchObj && hasInterrupted && (
                      <button
                        id="btn-recover-interrupted"
                        className="primary"
                        disabled={busy}
                        onClick={() => void handleRecoverBatch(activeBatchObj.id)}
                      >
                        ⚡ {isAr ? 'استعادة التنفيذ المتوقف بأمان' : 'Recover Interrupted Execution'}
                      </button>
                    )}
                  </div>
                </div>

                {/* 9-Metric Pre-Execution Summary Generated Directly From Approved Operation Manifest */}
                {manifestSummary && (
                  <div className="manifest-grid" id="manifest-pre-execution-summary">
                    <div className="manifest-metric">
                      <span className="m-label">{isAr ? 'ملفات ستُعاد تسميتها' : 'Files to Rename'}</span>
                      <span className="m-val">{manifestSummary.filesToRename}</span>
                    </div>
                    <div className="manifest-metric">
                      <span className="m-label">{isAr ? 'ملفات ستُنقل لمجلداتها' : 'Files to Move'}</span>
                      <span className="m-val">{manifestSummary.filesToMove}</span>
                    </div>
                    <div className="manifest-metric">
                      <span className="m-label">{isAr ? 'مجلدات رسمية ستُنشأ' : 'Folders to Create'}</span>
                      <span className="m-val">{manifestSummary.foldersToCreate}</span>
                    </div>
                    <div className="manifest-metric">
                      <span className="m-label">{isAr ? 'مجلدات فارغة إلى الأخطاء' : 'Empty Folders -> Errors'}</span>
                      <span className="m-val">{manifestSummary.emptyFoldersToMoveErrors}</span>
                    </div>
                    <div className="manifest-metric">
                      <span className="m-label">{isAr ? 'ملفات ترجمة ستُربط وتُنظم' : 'Subtitles to Associate'}</span>
                      <span className="m-val">{manifestSummary.subtitlesToAssociate}</span>
                    </div>
                    <div className="manifest-metric">
                      <span className="m-label">{isAr ? 'بوسترات وصور فنية ستُعتمد' : 'Posters & Artwork'}</span>
                      <span className="m-val">{manifestSummary.postersArtworkToAssociate}</span>
                    </div>
                    <div className="manifest-metric">
                      <span className="m-label">{isAr ? 'مرفقات .shorts محمية دائماً' : '.shorts Companions Preserved'}</span>
                      <span className="m-val">{manifestSummary.shortsToPreserve}</span>
                    </div>
                    <div className="manifest-metric">
                      <span className="m-label">{isAr ? 'تعارضات غير محلولة (تُمنع)' : 'Unresolved Conflicts'}</span>
                      <span className="m-val" style={{ color: manifestSummary.unresolvedConflicts > 0 ? 'var(--status-warn)' : 'var(--status-ok)' }}>
                        {manifestSummary.unresolvedConflicts}
                      </span>
                    </div>
                    <div className="manifest-metric">
                      <span className="m-label">{isAr ? 'ملفات ستبقى بدون أي تغيير' : 'Files Remaining Unchanged'}</span>
                      <span className="m-val">{manifestSummary.unchangedFilesCount}</span>
                    </div>
                  </div>
                )}

                {/* Per-Operation Status Table */}
                <h3 style={{ fontSize: '0.92rem', color: 'var(--accent-gold)', marginTop: '4px' }}>
                  {isAr ? 'حالة كل عملية داخل الدفعة (Per-Operation Execution Status)' : 'Per-Operation Execution Status'}
                </h3>
                <div className="table-scroll">
                  <table className="admin-table">
                    <thead>
                      <tr>
                        <th>#</th>
                        <th>{isAr ? 'العنصر الأصلي' : 'Source Item'}</th>
                        <th>{isAr ? 'الوجهة المعتمدة' : 'Target Path'}</th>
                        <th>{isAr ? 'العملية' : 'Action'}</th>
                        <th>{isAr ? 'حالة القرار / التنفيذ' : 'Operation Status'}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {batchItems.slice(0, 30).map((it, idx) => (
                        <tr key={it.id}>
                          <td>{idx + 1}</td>
                          <td className="mono">{it.oldRelPath}</td>
                          <td className="mono">{it.effectiveRelPath}</td>
                          <td><span className="badge">{it.actionKind}</span></td>
                          <td>
                            <span
                              className={`badge ${
                                it.decisionStatus === 'executed' || it.decisionStatus === 'approved'
                                  ? 'ok'
                                  : it.decisionStatus === 'rejected' || it.decisionStatus === 'failed'
                                    ? 'err'
                                    : 'warn'
                              }`}
                            >
                              {it.operationState ? `${it.decisionStatus} (${it.operationState})` : it.decisionStatus}
                            </span>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </section>

              <div className="split-layout">
                <section className="panel">
                  <h2>{isAr ? 'تحليل التكرارات واختيار أفضل نسخة (Cross-Disk = Keep)' : 'Duplicates & Best-Version Selection (Cross-Disk = Keep)'}</h2>
                  <div className="btn-row">
                    <input
                      className="input"
                      value={namingSampleTitle}
                      onChange={(e) => setNamingSampleTitle(e.target.value)}
                      placeholder="Movie / Series Title"
                    />
                    <input
                      type="number"
                      className="input"
                      style={{ width: '95px' }}
                      value={namingSampleYear}
                      onChange={(e) => setNamingSampleYear(Number(e.target.value))}
                    />
                    <button
                      onClick={() =>
                        void LibraryClient.previewNamingRules(namingSampleTitle, namingSampleYear, false, 1, 1).then((r) =>
                          setNamingPreviewOut(JSON.stringify(r, null, 2))
                        )
                      }
                    >
                      {isAr ? 'معاينة فيلم + .shorts' : 'Preview Movie + .shorts'}
                    </button>
                    <button
                      onClick={() =>
                        void LibraryClient.previewNamingRules(namingSampleTitle, namingSampleYear, true, 1, 1).then((r) =>
                          setNamingPreviewOut(JSON.stringify(r, null, 2))
                        )
                      }
                    >
                      {isAr ? 'معاينة مسلسل S01/01' : 'Preview Series S01/01'}
                    </button>
                  </div>
                  {namingPreviewOut && (
                    <pre className="mono" style={{ background: 'var(--bg-primary)', padding: '10px', borderRadius: '8px' }}>
                      {namingPreviewOut}
                    </pre>
                  )}

                  {dupGroups.length === 0 ? (
                    <p style={{ color: 'var(--text-muted)', fontSize: '0.86rem' }}>
                      {isAr ? 'لا توجد مجموعات تكرار مرصودة حالياً.' : 'No duplicate groups detected.'}
                    </p>
                  ) : (
                    dupGroups.map((g) => (
                      <div key={g.groupId} className="panel" style={{ background: 'var(--bg-secondary)' }}>
                        <div className="btn-row">
                          <strong>{g.workTitle}</strong>
                          <span className={`badge ${g.crossDiskKeep ? 'ok' : 'warn'}`}>
                            {g.crossDiskKeep
                              ? isAr ? 'نسخ عبر أقراص مختلفة — محفوظة تلقائياً (Keep)' : 'Cross-Disk Copies — Preserved (Keep)'
                              : isAr ? 'تكرار على نفس القرص — مراجعة بدون حذف تلقائي' : 'Same-Disk Multi-Version'}
                          </span>
                        </div>
                        {g.members.map((m) => (
                          <div key={m.fileId} className="mono" style={{ fontSize: '0.78rem' }}>
                            {m.isSelectedBest ? '★ BEST ' : '• '} [{m.diskLabel}] {m.filename} — {m.resolutionLabel} {m.videoCodec} (Score: {Math.round(m.qualityScore)}) → {m.proposedAction}
                          </div>
                        ))}
                      </div>
                    ))
                  )}
                </section>

                <section className="panel">
                  <h2>{isAr ? 'سجل العمليات الموثق بـ SHA-256 (Operation Journal)' : 'Verified SHA-256 Operation Journal'}</h2>
                  <table className="admin-table">
                    <thead>
                      <tr>
                        <th>#</th>
                        <th>{isAr ? 'الحالة' : 'State'}</th>
                        <th>{isAr ? 'من ← إلى' : 'From -> To'}</th>
                        <th>SHA-256</th>
                      </tr>
                    </thead>
                    <tbody>
                      {journalEntries.slice(0, 25).map((j) => (
                        <tr key={j.id}>
                          <td>{j.stepSequence}</td>
                          <td><span className={`badge ${j.operationState === 'completed' ? 'ok' : 'warn'}`}>{j.operationState}</span></td>
                          <td className="mono">{j.sourcePath} → {j.destinationPath}</td>
                          <td className="mono">{j.postSha256 ? `${j.postSha256.slice(0, 12)}…` : j.errorMessage}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </section>
              </div>
            </>
          );
        })()}

        {/* 6B. MANAGED ERROR CENTER (الأخطاء — 6 OFFICIAL CATEGORIES & REVERSIBLE RESTORE) */}
        {tab === 'errors' && (() => {
          const selectedErrCount = Object.values(selectedErrorIds).filter(Boolean).length;
          const activeEntries = errorEntries.filter((e) => e.entryStatus === 'active');
          const distinctBatches = Array.from(
            new Set(activeEntries.map((e) => e.batchId).filter(Boolean))
          );

          return (
            <section className="panel" id="error-center-panel">
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '12px' }}>
                <div>
                  <h2>
                    {isAr
                      ? 'مركز الأخطاء المُدار (الأخطاء/) — الفئات الست الرسمية والاسترجاع الكامل بدون أي حذف تلقائي'
                      : 'Managed Error Center (الأخطاء/) — 6 Official Categories & Full Reversible Restore'}
                  </h2>
                  <p style={{ fontSize: '0.8rem', color: 'var(--text-muted)', marginTop: '3px' }}>
                    {isAr
                      ? 'جميع الملفات المنقولة إلى الأخطاء محفوظة ببصمة SHA-256 ومسارها الأصلي الكامل، مع حماية دائمة لملفات .shorts من النقل إلى العينات أو الأخطاء.'
                      : 'All quarantined items preserve SHA-256 hashes and original paths with 1-click individual, multi-select, or batch restoration. .shorts files are permanently protected.'}
                  </p>
                </div>

                <div className="btn-row">
                  <button
                    onClick={() => {
                      const next: Record<string, boolean> = {};
                      for (const ev of activeEntries) next[ev.id] = true;
                      setSelectedErrorIds(next);
                    }}
                    disabled={activeEntries.length === 0}
                  >
                    {isAr ? 'تحديد كل النشط' : 'Select All Active'}
                  </button>
                  <button onClick={() => setSelectedErrorIds({})} disabled={selectedErrCount === 0}>
                    {isAr ? 'إلغاء التحديد' : 'Deselect All'}
                  </button>
                  <button
                    id="btn-restore-selected-errors"
                    className="primary"
                    disabled={busy || selectedErrCount === 0}
                    onClick={() => void handleRestoreSelectedErrorEntries()}
                  >
                    ↺ {isAr ? `استرجاع المحدد (${selectedErrCount})` : `Restore Selected (${selectedErrCount})`}
                  </button>
                  {distinctBatches.length > 0 && (
                    <button
                      id="btn-restore-complete-error-batch"
                      disabled={busy}
                      onClick={() => void handleRestoreErrorBatch(distinctBatches[0])}
                    >
                      ↺ {isAr ? 'استرجاع دفعة الأخطاء كاملة' : 'Restore Complete Reversible Batch'}
                    </button>
                  )}
                </div>
              </div>

              <div className="category-tabs" id="error-category-tabs">
                <button
                  className={`cat-tab-btn ${activeErrorCategory === 'all' ? 'active' : ''}`}
                  onClick={() => {
                    setActiveErrorCategory('all');
                    void loadErrorCenter('all');
                  }}
                >
                  <span>{isAr ? 'كل الفئات (6)' : 'All 6 Categories'}</span>
                  <span className="nav-pill">{activeEntries.length}</span>
                </button>
                {errorCategories.map((cat) => (
                  <button
                    key={cat.code}
                    id={`err-cat-${cat.code}`}
                    className={`cat-tab-btn ${activeErrorCategory === cat.code ? 'active' : ''}`}
                    onClick={() => {
                      setActiveErrorCategory(cat.code);
                      void loadErrorCenter(cat.code);
                    }}
                  >
                    <span>{isAr ? cat.folderAr : `${cat.titleEn} (${cat.folderAr})`}</span>
                  </button>
                ))}
              </div>

              <div className="table-scroll">
                <table className="admin-table" id="error-entries-table">
                  <thead>
                    <tr>
                      <th className="checkbox-cell">☑</th>
                      <th>{isAr ? 'العنصر والتصنيف الرسمي' : 'Item & Official Category'}</th>
                      <th>{isAr ? 'الموقع الأصلي الكامل (قبل النقل)' : 'Original Location (Before)'}</th>
                      <th>{isAr ? 'موقع الحفظ في الأخطاء' : 'Managed Error Path'}</th>
                      <th>{isAr ? 'سبب التصنيف والتحقق' : 'Classification Reason & SHA-256'}</th>
                      <th>{isAr ? 'الحالة والاسترجاع' : 'Status & Restore'}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {errorEntries.length === 0 ? (
                      <tr>
                        <td colSpan={6} style={{ textAlign: 'center', color: 'var(--text-muted)', padding: '18px' }}>
                          {isAr
                            ? 'لا توجد عناصر حالياً في هذا التصنيف داخل مجلد الأخطاء.'
                            : 'No items currently in this Error Center category.'}
                        </td>
                      </tr>
                    ) : (
                      errorEntries.map((ev) => (
                        <tr key={ev.id}>
                          <td className="checkbox-cell">
                            <input
                              type="checkbox"
                              disabled={ev.entryStatus !== 'active'}
                              checked={Boolean(selectedErrorIds[ev.id])}
                              onChange={(e) =>
                                setSelectedErrorIds((prev) => ({ ...prev, [ev.id]: e.target.checked }))
                              }
                            />
                          </td>
                          <td>
                            <strong>{ev.itemName}</strong>
                            <div className="btn-row" style={{ marginTop: '4px' }}>
                              <span className="badge gold">{ev.categoryFolderAr}</span>
                              <span className="badge">{ev.entryKind}</span>
                              {ev.entryKind === 'file' && <span className="mono">{formatBytes(ev.sizeBytes)}</span>}
                            </div>
                          </td>
                          <td className="mono">
                            [{ev.diskLabel || ev.diskId.slice(0, 6)}] {ev.originalRelPath}
                          </td>
                          <td className="mono">{ev.errorRelPath}</td>
                          <td>
                            <div style={{ fontSize: '0.8rem' }}>{isAr ? ev.reasonAr : ev.reasonEn || ev.reasonAr}</div>
                            {ev.sha256Hash && (
                              <div className="mono" style={{ fontSize: '0.72rem', color: 'var(--text-muted)', marginTop: '3px' }}>
                                SHA-256: {ev.sha256Hash.slice(0, 16)}…
                              </div>
                            )}
                          </td>
                          <td>
                            <div className="btn-row">
                              <span className={`badge ${ev.entryStatus === 'restored' ? 'ok' : 'warn'}`}>
                                {ev.entryStatus === 'restored'
                                  ? isAr ? 'تم الاسترجاع' : 'Restored'
                                  : isAr ? 'محفوظ في الأخطاء' : 'Quarantined'}
                              </span>
                              <button onClick={() => setInspectedErrorEntry(ev)}>
                                ℹ {isAr ? 'فحص' : 'Inspect'}
                              </button>
                              {ev.entryStatus === 'active' && (
                                <button
                                  className="primary"
                                  disabled={busy}
                                  onClick={() => void handleRestoreSingleErrorEntry(ev.id)}
                                >
                                  ↺ {isAr ? 'استرجاع للموقع الأصلي' : 'Restore Item'}
                                </button>
                              )}
                            </div>
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </section>
          );
        })()}

        {/* 7. INTEGRATED MEDIA PLAYER */}
        {tab === 'player' && (
          <section className="panel">
            <h2>{isAr ? 'المشغل السينمائي المدمج (HTTP Byte-Range + WebVTT Subtitles)' : 'Integrated Media Player (Byte-Range Streaming + WebVTT Subtitles)'}</h2>
            {!session ? (
              <p style={{ color: 'var(--text-muted)' }}>
                {isAr ? 'اختر أي فيلم أو حلقة من مكتبة الأفلام أو المسلسلات لبدء التشغيل الفوري مع حفظ موضع الاستئناف.' : 'Select any movie or episode from Movies or Series to play immediately with resume tracking.'}
              </p>
            ) : (
              <>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '10px' }}>
                  <div>
                    <strong>{session.title}</strong>
                    <div className="mono" style={{ color: 'var(--text-muted)' }}>{session.fullPath}</div>
                  </div>
                  <div className="btn-row">
                    {session.prevEpisodeId && (
                      <button onClick={() => void startPlayback(session.workId, session.prevEpisodeId, '')}>
                        ⏮ {isAr ? 'الحلقة السابقة' : 'Prev Episode'}
                      </button>
                    )}
                    {session.nextEpisodeId && (
                      <button className="primary" onClick={() => void startPlayback(session.workId, session.nextEpisodeId, '')}>
                        {isAr ? 'الحلقة التالية' : 'Next Episode'} ⏭
                      </button>
                    )}
                    <button onClick={() => void handleSaveProgress('watched')}>
                      ✓ {isAr ? 'تعليم كـ تمت المشاهدة' : 'Mark Watched'}
                    </button>
                    <button onClick={() => void handleSaveProgress('watch_later')}>
                      ★ {isAr ? 'مشاهدة لاحقاً' : 'Watch Later'}
                    </button>
                    <button onClick={() => void LibraryClient.launchExternalFallbackPlayer(session.fileId)}>
                      ↗ {isAr ? 'فتح بمشغل خارجي احتياطي (VLC/MPV)' : 'External Player Fallback'}
                    </button>
                  </div>
                </div>

                <video
                  ref={videoRef}
                  key={session.streamUrl}
                  className="video-stage"
                  controls
                  autoPlay
                  crossOrigin="anonymous"
                  onLoadedMetadata={(e) => {
                    if (session.resumePositionMs > 2000) {
                      e.currentTarget.currentTime = session.resumePositionMs / 1000;
                    }
                  }}
                  onPause={() => void handleSaveProgress('')}
                  onEnded={() => void handleSaveProgress('watched')}
                >
                  <source src={session.streamUrl} type={session.mimeType} />
                  {session.subtitles.map((sub, idx) => (
                    <track
                      key={sub.trackId}
                      kind="subtitles"
                      src={sub.webVttUrl}
                      srcLang={sub.languageCode}
                      label={sub.label}
                      default={idx === 0}
                    />
                  ))}
                </video>
              </>
            )}
          </section>
        )}

        {/* 8. REPORTS & MISSING DETECTION */}
        {tab === 'reports' && (
          <section className="panel">
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '10px' }}>
              <h2>{isAr ? 'مركز التقارير وكشف النواقص والأخطاء (Phase B9)' : 'Missing Items & Library Health Reports (Phase B9)'}</h2>
              <div className="btn-row">
                <select
                  className="input"
                  value={issueFilter}
                  onChange={(e) => {
                    setIssueFilter(e.target.value);
                    void loadIssues(e.target.value);
                  }}
                >
                  <option value="all">{isAr ? 'كل الفئات (8 فئات)' : 'All Categories'}</option>
                  <option value="missing_episode">{isAr ? 'حلقات مفقودة' : 'Missing Episodes'}</option>
                  <option value="missing_season">{isAr ? 'مواسم مفقودة' : 'Missing Seasons'}</option>
                  <option value="missing_subtitle">{isAr ? 'ترجمات مفقودة' : 'Missing Subtitles'}</option>
                  <option value="missing_poster">{isAr ? 'بوسترات مفقودة' : 'Missing Posters'}</option>
                  <option value="unidentified_file">{isAr ? 'ملفات غير معرّفة' : 'Unidentified Files'}</option>
                  <option value="corrupted_file">{isAr ? 'ملفات تالفة / 0 بايت' : 'Corrupted / Unreadable'}</option>
                  <option value="duplicate_conflict">{isAr ? 'تكرارات نفس القرص' : 'Same-Disk Duplicates'}</option>
                  <option value="offline_disk">{isAr ? 'أقراص منفصلة' : 'Offline Drives'}</option>
                </select>
                <button className="primary" onClick={() => void loadIssues(issueFilter)} disabled={busy}>
                  {isAr ? 'إعادة فحص صحة المكتبة' : 'Refresh Health Report'}
                </button>
              </div>
            </div>

            <table className="admin-table">
              <thead>
                <tr>
                  <th>{isAr ? 'النوع' : 'Kind'}</th>
                  <th>{isAr ? 'التفاصيل' : 'Issue'}</th>
                  <th>{isAr ? 'الإجراء المقترح' : 'Suggested Action'}</th>
                </tr>
              </thead>
              <tbody>
                {issues.map((it) => (
                  <tr key={it.id}>
                    <td>
                      <span className={`badge ${it.severity === 'error' ? 'err' : it.severity === 'warning' ? 'warn' : 'gold'}`}>
                        {it.issueKind}
                      </span>
                    </td>
                    <td>
                      <strong>{isAr ? it.titleAr : it.titleEn}</strong>
                      <div style={{ fontSize: '0.8rem', color: 'var(--text-muted)' }}>
                        {isAr ? it.detailsAr : it.detailsEn}
                      </div>
                    </td>
                    <td style={{ fontSize: '0.82rem' }}>{isAr ? it.suggestedFixAr : it.suggestedFixEn}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
        )}

        {/* 9. AI ASSISTANT */}
        {tab === 'ai' && (
          <section className="panel">
            <h2>{isAr ? 'المساعد السينمائي الذكي (ذكاء محلي أوفلاين + DeepSeek بموافقة صريحة)' : 'Smart Cinema Assistant (Offline Local Engine + Consented DeepSeek)'}</h2>
            <div className="btn-row">
              <input
                className="input"
                style={{ flex: 1 }}
                placeholder={isAr ? 'اسأل عن مكتبتك، اقتراح سهرة، أو حالة التنظيم...' : 'Ask about your library, evening watch picks, or organization health...'}
                value={aiPrompt}
                onChange={(e) => setAiPrompt(e.target.value)}
              />
              <label style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '0.82rem' }}>
                <input
                  type="checkbox"
                  checked={allowDeepSeek}
                  onChange={(e) => setAllowDeepSeek(e.target.checked)}
                />
                <span>{isAr ? 'موافقة صريحة لهذا الطلب فقط عبر DeepSeek (دون إرسال مساراتك)' : 'Per-request DeepSeek consent (paths stripped)'}</span>
              </label>
              <button
                className="primary"
                disabled={busy}
                onClick={() =>
                  void LibraryClient.askSmartAssistant({
                    prompt: aiPrompt,
                    workId: '',
                    allowCloudDeepSeek: allowDeepSeek,
                  }).then(setAiReply)
                }
              >
                {isAr ? 'تحليل وإجابة' : 'Ask Assistant'}
              </button>
            </div>
            {aiReply && (
              <div className="panel" style={{ background: 'var(--bg-secondary)' }}>
                <div className="btn-row">
                  <span className="badge ok">Mode: {aiReply.mode}</span>
                  <span className="badge gold">Zero Local Path Leak: {String(aiReply.privacyGuaranteed)}</span>
                </div>
                <p style={{ lineHeight: 1.6 }}>{isAr ? aiReply.answerAr : aiReply.answerEn}</p>
                {aiReply.recommendations.length > 0 && (
                  <div>
                    <strong>{isAr ? 'ترشيحات غير مشاهدة من مكتبتك:' : 'Unwatched Picks From Your Library:'}</strong>
                    <div className="btn-row" style={{ marginTop: '6px' }}>
                      {aiReply.recommendations.map((r, i) => (
                        <span key={i} className="badge gold">{r}</span>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )}
          </section>
        )}

        {/* 10. SETTINGS, CREDENTIAL MANAGER & VERIFIED BACKUPS */}
        {tab === 'settings' && (
          <div className="split-layout">
            <section className="panel">
              <h2>{isAr ? 'النسخ الاحتياطي المزدوج الموثق (SQLite VACUUM + SHA-256 + سجل التراجع)' : 'Verified Backups (SQLite VACUUM INTO + SHA-256 + Reversible Manifest)'}</h2>
              <p style={{ fontSize: '0.82rem', color: 'var(--text-muted)' }}>
                {isAr
                  ? 'تنبيه نطاق صريح: النسخة الاحتياطية تحمي الفهرس وحالات المشاهدة وسجل التراجع، ولا تنسخ ملفات الفيديو الأصلية الضخمة بدون إذنك.'
                  : 'Scope Boundary: Backups protect SQLite catalog, watch progress, and rollback journals; they never mass-copy user video files.'}
              </p>
              <div className="btn-row">
                <button className="primary" onClick={() => void handleCreateBackup()} disabled={busy}>
                  + {isAr ? 'إنشاء نسخة احتياطية موثقة الآن' : 'Create Verified Backup Now'}
                </button>
              </div>
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>{isAr ? 'النوع' : 'Kind'}</th>
                    <th>{isAr ? 'الملف' : 'Archive'}</th>
                    <th>{isAr ? 'التحقق' : 'Integrity'}</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {backups.map((b) => (
                    <tr key={b.id}>
                      <td>{b.backupKind}</td>
                      <td className="mono">{b.archivePath} ({formatBytes(b.sizeBytes)})</td>
                      <td>
                        <span className={`badge ${b.verificationStatus === 'verified' ? 'ok' : 'err'}`}>
                          {b.verificationStatus} ({b.sha256Checksum.slice(0, 8)})
                        </span>
                      </td>
                      <td>
                        <button onClick={() => void LibraryClient.verifyBackup(b.id).then(() => loadSettingsData())}>
                          {isAr ? 'إعادة فحص' : 'Re-Verify'}
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </section>

            <section className="panel">
              <h2>{isAr ? 'مزودو البيانات وWindows Credential Manager' : 'API Hub & Windows Credential Manager'}</h2>
              <div className="btn-row">
                <button
                  onClick={() =>
                    void callPoC(PoCService.TestCredentialManager, 'TestCredentialManager').then((r) =>
                      notify('ok', `Credential Manager (${r.backend}): write=${r.writeSucceeded}, purged=${r.confirmedPurged}`)
                    )
                  }
                >
                  {isAr ? 'اختبار خزنة Windows Credential Manager' : 'Run Credential Manager Probe'}
                </button>
              </div>
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>Provider</th>
                    <th>Status</th>
                    <th>API Key (Vault Only)</th>
                  </tr>
                </thead>
                <tbody>
                  {providersList.map((p) => (
                    <tr key={p.id}>
                      <td>
                        <strong>{p.displayName}</strong>
                        <div style={{ fontSize: '0.74rem', color: 'var(--text-muted)' }}>{p.attributionRequirement}</div>
                      </td>
                      <td>
                        <span className={`badge ${p.hasKeyConfigured ? 'ok' : 'warn'}`}>
                          {p.integrationStatus} {p.hasKeyConfigured ? '(Key in Vault)' : ''}
                        </span>
                      </td>
                      <td>
                        {p.integrationStatus !== 'deferred_legal_review' && p.requiresApiKey && (
                          <div className="btn-row">
                            <input
                              type="password"
                              className="input"
                              style={{ width: '135px' }}
                              placeholder="API Key..."
                              value={providerKeyInputs[p.id] ?? ''}
                              onChange={(e) => setProviderKeyInputs({ ...providerKeyInputs, [p.id]: e.target.value })}
                            />
                            <button
                              onClick={() =>
                                void LibraryClient.saveProviderAPIKey(p.id, providerKeyInputs[p.id] ?? '').then(() => {
                                  setProviderKeyInputs({ ...providerKeyInputs, [p.id]: '' });
                                  void loadSettingsData();
                                  notify('ok', `Saved ${p.displayName} key to OS Credential Vault`);
                                })
                              }
                            >
                              Save
                            </button>
                          </div>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>

              <h3 style={{ fontSize: '0.9rem', marginTop: '8px' }}>{isAr ? 'سجل التدقيق الأمني (Audit Log)' : 'Security & Safety Audit Log'}</h3>
              <div style={{ maxHeight: '180px', overflowY: 'auto' }}>
                {auditLogs.slice(0, 15).map((a) => (
                  <div key={a.id} className="mono" style={{ padding: '4px 0', borderBottom: '1px solid var(--border-color)' }}>
                    [{a.category}] {a.summary}
                  </div>
                ))}
              </div>
            </section>
          </div>
        )}

        {/* MODAL 1: EDIT PROPOSED FILENAME & DESTINATION */}
        {editTargetModalItem && (
          <div className="modal-backdrop" onClick={() => setEditTargetModalItem(null)}>
            <div className="modal-card" onClick={(e) => e.stopPropagation()}>
              <h2>{isAr ? 'تعديل الاسم المقترح أو مجلد الوجهة' : 'Edit Proposed Filename & Destination Directory'}</h2>
              <div className="mono" style={{ fontSize: '0.78rem', color: 'var(--text-muted)' }}>
                {isAr ? 'العنصر الأصلي: ' : 'Original Item: '}
                {editTargetModalItem.oldRelPath}
              </div>
              {editTargetModalItem.protectedShortsFile && (
                <div className="badge gold">
                  {isAr
                    ? 'قانون الحماية نشط: يجب الاحتفاظ بامتداد .shorts كما هو دون حذف أو تغيير.'
                    : 'Protection Law Active: .shorts extension must remain intact.'}
                </div>
              )}

              <label style={{ fontSize: '0.84rem', fontWeight: 600 }}>
                {isAr ? 'الاسم المقترح الجديد (Proposed Filename):' : 'Proposed Filename:'}
              </label>
              <input
                id="modal-edit-filename-input"
                className="input mono"
                value={modalEditName}
                onChange={(e) => setModalEditName(e.target.value)}
              />

              <label style={{ fontSize: '0.84rem', fontWeight: 600 }}>
                {isAr ? 'مجلد الوجهة النسبي على القرص (Proposed Destination Directory):' : 'Proposed Destination Directory (Relative to Disk):'}
              </label>
              <input
                id="modal-edit-dir-input"
                className="input mono"
                placeholder="Movies/Movie Title (2024)"
                value={modalEditDir}
                onChange={(e) => setModalEditDir(e.target.value)}
              />

              <div className="btn-row" style={{ justifyContent: 'flex-end', marginTop: '8px' }}>
                <button onClick={() => setEditTargetModalItem(null)}>{isAr ? 'إلغاء' : 'Cancel'}</button>
                <button className="primary" disabled={busy} onClick={() => void handleSaveEditedTarget()}>
                  ✓ {isAr ? 'حفظ وفحص عدم التعارض' : 'Save & Check Collisions'}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* MODAL 2: VIEW FULL REVIEW FILE DETAILS */}
        {detailModalData && (
          <div className="modal-backdrop" onClick={() => setDetailModalData(null)}>
            <div className="modal-card" onClick={(e) => e.stopPropagation()}>
              <h2>{isAr ? 'تفاصيل العنصر وفحوص السلامة الكاملة' : 'Full File & Safety Inspection Details'}</h2>
              <div className="kv-grid">
                <div className="kv-k">{isAr ? 'الاسم الأصلي:' : 'Original Filename:'}</div>
                <div className="mono">{detailModalData.item.oldName}</div>

                <div className="kv-k">{isAr ? 'المسار الأصلي الكامل:' : 'Original Full Path:'}</div>
                <div className="mono">{detailModalData.item.oldFullPath}</div>

                <div className="kv-k">{isAr ? 'الاسم المقترح المعتمد:' : 'Effective Filename:'}</div>
                <div className="mono">{detailModalData.item.effectiveName || detailModalData.item.proposedName}</div>

                <div className="kv-k">{isAr ? 'الوجهة الكاملة المقترحة:' : 'Proposed Full Destination:'}</div>
                <div className="mono">{detailModalData.item.effectiveFullPath}</div>

                <div className="kv-k">{isAr ? 'نوع الملف / العنصر:' : 'File Type:'}</div>
                <div>
                  {detailModalData.item.mediaFileKind || detailModalData.item.itemFileKind} ({detailModalData.extension || 'dir'})
                  {detailModalData.item.protectedShortsFile ? ' [.shorts Protected]' : ''}
                </div>

                <div className="kv-k">{isAr ? 'العمل المرتبط:' : 'Associated Movie/Series:'}</div>
                <div>
                  {detailModalData.item.workTitle
                    ? `${detailModalData.item.workTitle} (${detailModalData.productionYear || 'Series'}) [${detailModalData.workStatus}]`
                    : isAr ? 'غير مرتبط' : 'None'}
                </div>

                <div className="kv-k">{isAr ? 'الحجم والدقة:' : 'Size & Resolution:'}</div>
                <div className="mono">
                  {formatBytes(detailModalData.item.fileSizeBytes ?? 0)}
                  {detailModalData.videoWidth > 0 ? ` | ${detailModalData.videoWidth}x${detailModalData.videoHeight} (${detailModalData.item.videoCodec})` : ''}
                  {detailModalData.durationSeconds > 0 ? ` | ${Math.round(detailModalData.durationSeconds)}s` : ''}
                </div>

                <div className="kv-k">{isAr ? 'حالة الفحص الفني وبصمة SHA-256:' : 'Probe & SHA-256:'}</div>
                <div className="mono">
                  {detailModalData.probeStatus || 'n/a'} | {detailModalData.fileSha256 || (isAr ? 'تُحسب وتُوثق عند التنفيذ' : 'Verified on execute')}
                </div>

                <div className="kv-k">{isAr ? 'سبب التصنيف / التسمية:' : 'Classification Reason:'}</div>
                <div>{detailModalData.item.reasonDescription}</div>

                <div className="kv-k">{isAr ? 'مستوى الثقة وحالة التعارض:' : 'Confidence & Conflict:'}</div>
                <div>
                  {Math.round(detailModalData.item.confidenceScore * 100)}% —{' '}
                  {detailModalData.item.conflictDetected
                    ? detailModalData.item.conflictDetails
                    : isAr ? 'لا يوجد تعارض' : 'No Conflict'}
                </div>

                <div className="kv-k">{isAr ? 'المرفقات المرتبطة بالحزمة:' : 'Bundle Companions:'}</div>
                <div className="mono">
                  {detailModalData.item.linkedCompanions.length > 0
                    ? detailModalData.item.linkedCompanions.join(' , ')
                    : '—'}
                </div>
              </div>
              <div className="btn-row" style={{ justifyContent: 'flex-end', marginTop: '8px' }}>
                <button className="primary" onClick={() => setDetailModalData(null)}>
                  {isAr ? 'إغلاق' : 'Close'}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* MODAL 3: INSPECT ERROR CENTER ITEM */}
        {inspectedErrorEntry && (
          <div className="modal-backdrop" onClick={() => setInspectedErrorEntry(null)}>
            <div className="modal-card" onClick={(e) => e.stopPropagation()}>
              <h2>{isAr ? 'فحص عنصر مركز الأخطاء وأصل مساره' : 'Inspect Error Center Item & Provenance'}</h2>
              <div className="kv-grid">
                <div className="kv-k">{isAr ? 'اسم العنصر:' : 'Item Name:'}</div>
                <div className="mono">{inspectedErrorEntry.itemName}</div>

                <div className="kv-k">{isAr ? 'التصنيف الرسمي:' : 'Official Category:'}</div>
                <div>{inspectedErrorEntry.categoryFolderAr} ({inspectedErrorEntry.categoryCode})</div>

                <div className="kv-k">{isAr ? 'الموقع الأصلي الكامل:' : 'Original Full Location:'}</div>
                <div className="mono">{inspectedErrorEntry.originalFullPath || inspectedErrorEntry.originalRelPath}</div>

                <div className="kv-k">{isAr ? 'الموقع الحالي في الأخطاء:' : 'Current Managed Error Path:'}</div>
                <div className="mono">{inspectedErrorEntry.errorFullPath || inspectedErrorEntry.errorRelPath}</div>

                <div className="kv-k">{isAr ? 'سبب التصنيف التفصيلي:' : 'Classification Reason:'}</div>
                <div>{isAr ? inspectedErrorEntry.reasonAr : inspectedErrorEntry.reasonEn || inspectedErrorEntry.reasonAr}</div>

                <div className="kv-k">{isAr ? 'بصمة SHA-256 الموثقة:' : 'Verified SHA-256:'}</div>
                <div className="mono">{inspectedErrorEntry.sha256Hash || (isAr ? 'مجلد فارغ' : 'Directory')}</div>

                <div className="kv-k">{isAr ? 'تاريخ النقل / الدفعة:' : 'Moved At / Batch:'}</div>
                <div className="mono">{inspectedErrorEntry.movedAt} {inspectedErrorEntry.batchTitle ? `(${inspectedErrorEntry.batchTitle})` : ''}</div>
              </div>
              <div className="btn-row" style={{ justifyContent: 'flex-end', marginTop: '8px' }}>
                {inspectedErrorEntry.entryStatus === 'active' && (
                  <button
                    className="primary"
                    disabled={busy}
                    onClick={() => {
                      const id = inspectedErrorEntry.id;
                      setInspectedErrorEntry(null);
                      void handleRestoreSingleErrorEntry(id);
                    }}
                  >
                    ↺ {isAr ? 'استرجاع فوري إلى موقعه الأصلي' : 'Restore to Original Location'}
                  </button>
                )}
                <button onClick={() => setInspectedErrorEntry(null)}>{isAr ? 'إغلاق' : 'Close'}</button>
              </div>
            </div>
          </div>
        )}
      </main>
    </div>
  );
}
