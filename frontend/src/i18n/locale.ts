/**
 * Estiraha Next — Bilingual Direction & Locale Contract (RTL Arabic / LTR English)
 * Sidebar placement rule:
 * - Arabic ('ar'): RTL layout -> Sidebar anchored RIGHT
 * - English ('en'): LTR layout -> Sidebar anchored LEFT
 */
import type { LanguageCode, LayoutDirection } from '../services/contracts';

export interface LocaleConfig {
  code: LanguageCode;
  dir: LayoutDirection;
  sidebarPosition: 'right' | 'left';
}

export const LOCALES: Record<LanguageCode, LocaleConfig> = {
  ar: {
    code: 'ar',
    dir: 'rtl',
    sidebarPosition: 'right',
  },
  en: {
    code: 'en',
    dir: 'ltr',
    sidebarPosition: 'left',
  },
};
