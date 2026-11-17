/**
 * Estiraha Next — Frontend IPC & Domain Type Contracts (Phase 0 Foundation)
 * NOTE: Phase 0 establishes strict TypeScript contracts synchronized with Go domain models.
 * No mock/fake UI screens are built in Phase 0 per official project rules.
 */

export type LanguageCode = 'ar' | 'en';
export type LayoutDirection = 'rtl' | 'ltr';
export type ThemeMode = 'dark' | 'light';

export type MediaType =
  | 'movie'
  | 'series'
  | 'tv_program'
  | 'talk_show'
  | 'game_show'
  | 'sports_program'
  | 'entertainment_program'
  | 'documentary_movie'
  | 'documentary_series'
  | 'anime'
  | 'cartoon'
  | 'franchise_entry';

export type WatchStatus =
  | 'unwatched'
  | 'watching'
  | 'watched'
  | 'watch_later'
  | 'rewatching';

export type ReviewStatus =
  | 'pending'
  | 'approved'
  | 'rejected'
  | 'deferred'
  | 'excluded'
  | 'executed'
  | 'rolled_back';

export interface SystemHealthSnapshot {
  appName: string;
  version: string;
  schemaVersion: number;
  walEnabled: boolean;
  foreignKeysEnabled: boolean;
  fts5Available: boolean;
  safetyLockActive: boolean;
}
