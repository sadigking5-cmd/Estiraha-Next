/**
 * Estiraha Next — Phase B Typed Wails v3 Client for `LibraryService`
 * Uses Wails v3's canonical `Call.ByName` IPC mechanism (`github.com/sadigking5-cmd/Estiraha-Next/internal/api.LibraryService.<Method>`)
 * so every UI button invokes real Go + SQLite backend methods with zero mocks.
 */
import { Call } from '@wailsio/runtime';

const SVC = 'github.com/sadigking5-cmd/Estiraha-Next/internal/api.LibraryService';

function callLibrary<T>(method: string, ...args: unknown[]): Promise<T> {
  return Call.ByName(`${SVC}.${method}`, ...args) as Promise<T>;
}

export interface DashboardStats {
  totalDisks: number;
  onlineDisks: number;
  totalRoots: number;
  totalMediaFiles: number;
  totalSizeBytes: number;
  totalMovies: number;
  totalSeries: number;
  totalEpisodesPresent: number;
  protectedShortsCount: number;
  needsReviewCount: number;
  pendingReviewBatches: number;
  verifiedBackupsCount: number;
  watchingNowCount: number;
  issuesByKind: Record<string, number>;
}

export interface DiskView {
  id: string;
  volumeSerial: string;
  label: string;
  mountPath: string;
  filesystemType: string;
  totalBytes: number;
  freeBytes: number;
  connectionState: string;
  rootCount: number;
  mediaFileCount: number;
  lastSeenAt: string;
  lastScanAt: string;
}

export interface RootView {
  id: string;
  diskId: string;
  diskLabel: string;
  mountPath: string;
  relativeRootPath: string;
  fullPath: string;
  contentCategory: string;
  displayName: string;
  scanEnabled: boolean;
  watchEnabled: boolean;
  isAccessible: boolean;
  passiveChangeAlert: boolean;
  indexedFileCount: number;
  lastScanAt: string;
}

export interface ContinueWatchingItem {
  workId: string;
  episodeId: string;
  fileId: string;
  title: string;
  mediaType: string;
  watchStatus: string;
  resumePositionMs: number;
  durationMs: number;
  lastPlayedAt: string;
  posterUrl: string;
}

export interface NotificationView {
  id: string;
  notificationType: string;
  titleAr: string;
  titleEn: string;
  bodyAr: string;
  bodyEn: string;
  isRead: boolean;
  createdAt: string;
}

export interface SystemOverview {
  stats: DashboardStats;
  disks: DiskView[];
  roots: RootView[];
  continueWatching: ContinueWatchingItem[];
  recentNotifications: NotificationView[];
  scanRunning: boolean;
  activePolicies: Record<string, string>;
}

export interface ScanSummary {
  jobId: string;
  state: string;
  rootsScanned: number;
  rootsSkippedOffline: number;
  filesDiscovered: number;
  newFiles: number;
  updatedFiles: number;
  missingMarked: number;
  corruptedDetected: number;
  shortsDetected: number;
  durationMs: number;
  warnings: string[];
}

export interface ScanAndAnalyzeResult {
  scan: ScanSummary;
  identification: {
    processedVideos: number;
    moviesIdentified: number;
    seriesIdentified: number;
    episodesLinked: number;
    needsReviewCount: number;
    companionsLinked: number;
  };
  duplicates: {
    groupsDetected: number;
    crossDiskKeptGroups: number;
    sameDiskReviewGroups: number;
  };
  issueCount: number;
}

export interface AssociatedFileView {
  fileId: string;
  diskId: string;
  diskLabel: string;
  diskOnline: boolean;
  relativePath: string;
  fullPath: string;
  filename: string;
  extension: string;
  fileKind: string;
  sizeBytes: number;
  integrityStatus: string;
  presenceStatus: string;
  resolutionLabel: string;
  videoCodec: string;
  qualityScore: number;
  protectedShorts: boolean;
}

export interface MovieItem {
  id: string;
  mediaType: string;
  titleOriginal: string;
  titleAr: string;
  titleEn: string;
  isArabicOrigin: boolean;
  productionYear: number;
  canonicalFolderName: string;
  identificationStatus: string;
  confidenceScore: number;
  watchStatus: string;
  resumePositionMs: number;
  durationMs: number;
  ratingOutOf10: number;
  isFavorite: boolean;
  bestResolution: string;
  versionCount: number;
  subtitleCount: number;
  shortsCount: number;
  posterFileId: string;
  posterUrl: string;
  diskLabels: string;
  synopsisAr?: string;
  synopsisEn?: string;
  personalNote?: string;
  files?: AssociatedFileView[];
}

export interface EpisodeVersionItem {
  fileId: string;
  diskLabel: string;
  fullPath: string;
  filename: string;
  resolutionLabel: string;
  videoCodec: string;
  sizeBytes: number;
  integrityStatus: string;
  qualityScore: number;
  isBestVersion: boolean;
}

export interface EpisodeView {
  id: string;
  seasonNumber: number;
  episodeNumber: number;
  canonicalFileStem: string;
  titleAr: string;
  titleEn: string;
  isMissing: boolean;
  watchStatus: string;
  resumePositionMs: number;
  bestFileId: string;
  bestResolution: string;
  subtitleCount: number;
  hasShorts: boolean;
  versions: EpisodeVersionItem[];
}

export interface SeasonView {
  id: string;
  seasonNumber: number;
  canonicalFolderCode: string;
  expectedEpisodeCount: number;
  presentEpisodeCount: number;
  missingEpisodeNumbers: number[];
  episodes: EpisodeView[];
}

export interface SeriesItem {
  id: string;
  mediaType: string;
  titleOriginal: string;
  titleAr: string;
  titleEn: string;
  isArabicOrigin: boolean;
  canonicalFolderName: string;
  identificationStatus: string;
  confidenceScore: number;
  seasonCount: number;
  presentEpisodeCount: number;
  missingEpisodeCount: number;
  missingSeasonNumbers: number[];
  posterFileId: string;
  posterUrl: string;
  watchStatus: string;
  synopsisAr?: string;
  synopsisEn?: string;
  seasons?: SeasonView[];
}

export interface DuplicateGroupView {
  groupId: string;
  duplicateKind: string;
  workId: string;
  episodeId: string;
  workTitle: string;
  selectedFileId: string;
  resolutionState: string;
  crossDiskKeep: boolean;
  members: Array<{
    fileId: string;
    diskId: string;
    diskLabel: string;
    fullPath: string;
    filename: string;
    sizeBytes: number;
    resolutionLabel: string;
    videoCodec: string;
    integrityStatus: string;
    qualityScore: number;
    memberReason: string;
    proposedAction: string;
    isSelectedBest: boolean;
  }>;
}

export interface ReviewBatchView {
  id: string;
  title: string;
  batchType: string;
  batchStatus: string;
  totalItems: number;
  approvedItems: number;
  pendingItems: number;
  executedItems: number;
  approvedByUserAt: string;
  createdAt: string;
}

export interface ReviewItemView {
  id: string;
  batchId: string;
  fileId: string;
  workId: string;
  actionKind: string;
  sourceDiskId: string;
  diskLabel: string;
  oldRelPath: string;
  oldFullPath: string;
  oldName: string;
  targetDiskId: string;
  proposedRelPath: string;
  effectiveRelPath: string;
  effectiveFullPath: string;
  proposedName: string;
  userEditedRelPath: string;
  userEditedName: string;
  reasonDescription: string;
  decisionStatus: string;
  confidenceScore: number;
  conflictDetected: boolean;
  conflictDetails: string;
  protectedShortsFile: boolean;
  linkedCompanions: string[];
}

export interface JournalEntryView {
  id: string;
  batchId: string;
  stepSequence: number;
  operationState: string;
  sourcePath: string;
  destinationPath: string;
  preSha256: string;
  postSha256: string;
  errorMessage: string;
  completedAt: string;
}

export interface HealthIssueView {
  id: string;
  issueKind: string;
  severity: string;
  workId: string;
  fileId: string;
  diskId: string;
  titleAr: string;
  titleEn: string;
  detailsAr: string;
  detailsEn: string;
  suggestedFixAr: string;
  suggestedFixEn: string;
  detectedAt: string;
}

export interface PlaybackSession {
  sessionId: string;
  workId: string;
  episodeId: string;
  fileId: string;
  title: string;
  filename: string;
  fullPath: string;
  streamUrl: string;
  mimeType: string;
  resumePositionMs: number;
  durationMs: number;
  watchStatus: string;
  nextEpisodeId: string;
  prevEpisodeId: string;
  subtitles: Array<{
    trackId: string;
    fileId: string;
    label: string;
    languageCode: string;
    format: string;
    webVttUrl: string;
  }>;
}

export interface BackupRecord {
  id: string;
  backupKind: string;
  archivePath: string;
  manifestPath: string;
  sizeBytes: number;
  sha256Checksum: string;
  schemaVersion: number;
  verificationStatus: string;
  scopeNoticeAr: string;
  scopeNoticeEn: string;
  createdAt: string;
  verifiedAt: string;
}

export interface ProviderStatusView {
  id: string;
  displayName: string;
  category: string;
  integrationStatus: string;
  requiresApiKey: boolean;
  hasKeyConfigured: boolean;
  credentialManagerTarget: string;
  requirePerRequestConsent: boolean;
  attributionRequirement: string;
}

export interface AIAssistResponse {
  mode: string;
  answerAr: string;
  answerEn: string;
  recommendations: string[];
  privacyGuaranteed: boolean;
}

export interface AuditLogView {
  id: string;
  category: string;
  severity: string;
  actor: string;
  summary: string;
  createdAt: string;
}

export const LibraryClient = {
  getSystemOverview: () => callLibrary<SystemOverview>('GetSystemOverview'),
  pickFolderDialog: () => callLibrary<string>('PickFolderDialog'),
  addLibraryRoot: (input: {
    fullPath: string;
    contentCategory: string;
    displayName: string;
    diskLabel: string;
  }) => callLibrary<RootView>('AddLibraryRoot', input),
  removeLibraryRoot: (rootId: string) => callLibrary<void>('RemoveLibraryRoot', rootId),
  listDisksAndRoots: () => callLibrary<{ disks: DiskView[]; roots: RootView[] }>('ListDisksAndRoots'),
  runManualScanAndAnalyze: (opts: { rootIds: string[]; computeHash: boolean }) =>
    callLibrary<ScanAndAnalyzeResult>('RunManualScanAndAnalyze', opts),
  cancelActiveScan: () => callLibrary<boolean>('CancelActiveScan'),

  listMovies: (filter: {
    query: string;
    identificationStatus: string;
    watchStatus: string;
    arabicOnly: boolean;
    foreignOnly: boolean;
    diskId: string;
    limit: number;
  }) => callLibrary<MovieItem[]>('ListMovies', filter),
  getMovieDetail: (workId: string) => callLibrary<MovieItem>('GetMovieDetail', workId),
  updateMovieMetadata: (input: {
    workId: string;
    titleOriginal: string;
    titleAr: string;
    titleEn: string;
    productionYear: number;
    isArabicOrigin: boolean;
    synopsisAr: string;
    synopsisEn: string;
    ratingOutOf10: number;
    personalNote: string;
    isFavorite: boolean;
  }) => callLibrary<MovieItem>('UpdateMovieMetadata', input),

  listSeries: (filter: {
    query: string;
    mediaType: string;
    missingOnly: boolean;
    arabicOnly: boolean;
    limit: number;
  }) => callLibrary<SeriesItem[]>('ListSeries', filter),
  getSeriesDetail: (workId: string) => callLibrary<SeriesItem>('GetSeriesDetail', workId),
  updateSeriesMetadata: (input: {
    workId: string;
    titleOriginal: string;
    titleAr: string;
    titleEn: string;
    isArabicOrigin: boolean;
    synopsisAr: string;
    synopsisEn: string;
  }) => callLibrary<SeriesItem>('UpdateSeriesMetadata', input),
  setSeasonExpectedEpisodes: (seasonId: string, expectedCount: number) =>
    callLibrary<void>('SetSeasonExpectedEpisodes', seasonId, expectedCount),

  previewNamingRules: (title: string, year: number, isSeries: boolean, seasonNum: number, epNum: number) =>
    callLibrary<Record<string, unknown>>('PreviewNamingRules', title, year, isSeries, seasonNum, epNum),
  listDuplicateGroups: () => callLibrary<DuplicateGroupView[]>('ListDuplicateGroups'),
  generateOrganizationPlan: (customTitle: string) =>
    callLibrary<ReviewBatchView>('GenerateOrganizationPlan', customTitle),
  listReviewBatches: () => callLibrary<ReviewBatchView[]>('ListReviewBatches'),
  listReviewBatchItems: (batchId: string) =>
    callLibrary<ReviewItemView[]>('ListReviewBatchItems', batchId),
  decideReviewItem: (input: {
    itemId: string;
    decision: string;
    userEditedRelPath: string;
    userEditedName: string;
  }) => callLibrary<ReviewItemView>('DecideReviewItem', input),
  approveAllSafeReviewItems: (batchId: string) =>
    callLibrary<ReviewBatchView>('ApproveAllSafeReviewItems', batchId),
  executeApprovedBatch: (batchId: string) =>
    callLibrary<{
      batchId: string;
      preBatchBackupId: string;
      executedCount: number;
      skippedCount: number;
      abortedCount: number;
      errors: string[];
      completedAt: string;
    }>('ExecuteApprovedBatch', batchId),
  rollbackBatch: (batchId: string) =>
    callLibrary<{
      batchId: string;
      restoredCount: number;
      blockedCount: number;
      errors: string[];
      completedAt: string;
    }>('RollbackBatch', batchId),
  listOperationJournal: (batchId: string) =>
    callLibrary<JournalEntryView[]>('ListOperationJournal', batchId),

  refreshAndListHealthIssues: (kindFilter: string) =>
    callLibrary<HealthIssueView[]>('RefreshAndListHealthIssues', kindFilter),

  startPlaybackSession: (workId: string, episodeId: string, explicitFileId: string) =>
    callLibrary<PlaybackSession>('StartPlaybackSession', workId, episodeId, explicitFileId),
  savePlaybackProgress: (input: {
    workId: string;
    episodeId: string;
    fileId: string;
    positionMs: number;
    durationMs: number;
    watchStatus: string;
  }) => callLibrary<string>('SavePlaybackProgress', input),
  launchExternalFallbackPlayer: (fileId: string) =>
    callLibrary<void>('LaunchExternalFallbackPlayer', fileId),

  askSmartAssistant: (req: { prompt: string; workId: string; allowCloudDeepSeek: boolean }) =>
    callLibrary<AIAssistResponse>('AskSmartAssistant', req),
  lookupTMDbCandidates: (query: string, mediaType: string, year: number) =>
    callLibrary<Array<{
      externalId: string;
      mediaType: string;
      titleOriginal: string;
      titleAr: string;
      titleEn: string;
      productionYear: number;
      overview: string;
      confidence: number;
    }>>('LookupTMDbCandidates', query, mediaType, year),
  listProviders: () => callLibrary<ProviderStatusView[]>('ListProviders'),
  saveProviderAPIKey: (providerId: string, apiKey: string) =>
    callLibrary<void>('SaveProviderAPIKey', providerId, apiKey),

  createVerifiedBackup: () => callLibrary<BackupRecord>('CreateVerifiedBackup'),
  verifyBackup: (backupId: string) => callLibrary<BackupRecord>('VerifyBackup', backupId),
  listBackups: () => callLibrary<BackupRecord[]>('ListBackups'),
  listNotifications: () => callLibrary<NotificationView[]>('ListNotifications'),
  markNotificationsRead: () => callLibrary<void>('MarkNotificationsRead'),
  listAuditLogs: () => callLibrary<AuditLogView[]>('ListAuditLogs'),
};
