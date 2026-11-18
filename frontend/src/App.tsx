import React, { useEffect, useState } from 'react';
import { Events } from '@wailsio/runtime';
import * as PoCService from './bindings/github.com/sadigking5-cmd/Estiraha-Next/internal/api/pocservice';
import type {
  BridgePingResponse,
  PlatformDiagnostics,
} from './bindings/github.com/sadigking5-cmd/Estiraha-Next/internal/api/models';
import type { ProbeReport } from './bindings/github.com/sadigking5-cmd/Estiraha-Next/internal/secrets/models';
import type { Snapshot as TraySnapshot } from './bindings/github.com/sadigking5-cmd/Estiraha-Next/internal/tray/models';
import { LOCALES } from './i18n/locale';
import type { LanguageCode, ThemeMode } from './services/contracts';

const COPY = {
  ar: {
    subtitle: 'إثبات المفهوم لسطح مكتب Windows — المرحلة 1A (Wails v3)',
    safetyTag: 'قفل الأمان نشط: صفر فحص للأقراص أو تعديل للملفات',
    sidebarNote: 'الشريط الجانبي مثبت يميناً في الوضع العربي (RTL)',
    switchLang: 'English (LTR)',
    switchTheme: 'تبديل المظهر (داكن / فاتح)',
    quitClean: 'خروج آمن نظيف',
    diagTitle: '1. تشخيص محرك Go وWebView2 وSQLite',
    bridgeTitle: '2. جسر الاتصال ثنائي الاتجاه (React ⇄ Go)',
    sendRpc: 'إرسال RPC إلى Go',
    triggerGoPush: 'طلب حدث مباشر من Go',
    emitReactEvent: 'إرسال حدث من React إلى Go',
    traycredTitle: '3. اختبار System Tray وWindows Credential Manager',
    runTrayCycle: 'إخفاء إلى Tray ثم استعادة (1.2 ث)',
    hideToTray: 'إخفاء إلى System Tray',
    testCredMgr: 'فحص Windows Credential Manager',
    bilingualSampleTitle: '4. فحص العرض ثنائي اللغة (RTL / LTR)',
    bilingualSampleAr: 'مثال تسمية رسمية للمشاهير: أحمد زكي (Ahmed Zaki) — الناظر 2000',
    bilingualSampleEn: 'Canonical Movie Example: The Dark Knight 2008 (No brackets, no quality tags)',
    eventStreamTitle: 'سجل الأحداث الحية (Go ⇄ React)',
  },
  en: {
    subtitle: 'Windows Desktop Proof of Concept — Phase 1A (Wails v3)',
    safetyTag: 'Safety Lock Active: Zero Disk Scans or File Mutations',
    sidebarNote: 'Sidebar anchored LEFT in English mode (LTR)',
    switchLang: 'العربية (RTL)',
    switchTheme: 'Toggle Theme (Dark / Light)',
    quitClean: 'Quit Cleanly',
    diagTitle: '1. Go Engine, WebView2 & SQLite Diagnostics',
    bridgeTitle: '2. Bidirectional IPC Bridge (React ⇄ Go)',
    sendRpc: 'Send RPC to Go',
    triggerGoPush: 'Trigger Go -> React Push',
    emitReactEvent: 'Emit React -> Go Event',
    traycredTitle: '3. System Tray & Windows Credential Manager',
    runTrayCycle: 'Hide to Tray & Auto-Restore (1.2s)',
    hideToTray: 'Hide to System Tray',
    testCredMgr: 'Test Windows Credential Manager',
    bilingualSampleTitle: '4. Bilingual Typography & Direction Check',
    bilingualSampleAr: 'Arabic Sample: أحمد زكي (Ahmed Zaki) — الناظر 2000',
    bilingualSampleEn: 'English Sample: The Dark Knight 2008 (No brackets, no quality tags)',
    eventStreamTitle: 'Live Event Bus Log (Go ⇄ React)',
  },
} as const;

export function App(): React.JSX.Element {
  const [lang, setLang] = useState<LanguageCode>('ar');
  const [theme, setTheme] = useState<ThemeMode>('dark');
  const [diag, setDiag] = useState<PlatformDiagnostics | null>(null);
  const [rpcInput, setRpcInput] = useState<string>('أحمد زكي — Estiraha Next 2026');
  const [lastPing, setLastPing] = useState<BridgePingResponse | null>(null);
  const [credReport, setCredReport] = useState<ProbeReport | null>(null);
  const [trayState, setTrayState] = useState<TraySnapshot | null>(null);
  const [eventLogs, setEventLogs] = useState<string[]>([]);
  const [errorMsg, setErrorMsg] = useState<string>('');

  const localeCfg = LOCALES[lang];
  const t = COPY[lang];

  const appendLog = (entry: string) => {
    setEventLogs((prev) => [`[${new Date().toISOString().slice(11, 23)}] ${entry}`, ...prev].slice(0, 25));
  };

  useEffect(() => {
    document.documentElement.lang = localeCfg.code;
    document.documentElement.dir = localeCfg.dir;
    document.documentElement.setAttribute('data-theme', theme);
  }, [localeCfg, theme]);

  useEffect(() => {
    PoCService.GetPlatformDiagnostics()
      .then((d) => {
        setDiag(d);
        setTrayState(d.tray);
        appendLog(`Connected to Go PID ${d.processId} (${d.goVersion} / Wails ${d.wailsVersion})`);
      })
      .catch((err: unknown) => setErrorMsg(String(err)));

    const offPush = Events.On('estiraha:go-push', (ev: unknown) => {
      appendLog(`Go->React Push: ${JSON.stringify(ev)}`);
    });
    const offLocale = Events.On('estiraha:locale-changed', (ev: unknown) => {
      appendLog(`Go->React Locale Sync: ${JSON.stringify(ev)}`);
    });
    const offAck = Events.On('estiraha:go-event-ack', (ev: unknown) => {
      appendLog(`Go Ack for React Event: ${JSON.stringify(ev)}`);
    });

    return () => {
      offPush();
      offLocale();
      offAck();
    };
  }, []);

  const handleToggleLanguage = async () => {
    const next: LanguageCode = lang === 'ar' ? 'en' : 'ar';
    setLang(next);
    try {
      const updated = await PoCService.SetActiveLocale(next);
      setDiag(updated);
    } catch (err: unknown) {
      setErrorMsg(String(err));
    }
  };

  const handleSendRpc = async () => {
    try {
      const res = await PoCService.PingGoFromReact({ message: rpcInput, locale: lang });
      setLastPing(res);
      appendLog(`RPC Reply #${res.roundtripSequence}: norm="${res.normalizedArabic}"`);
    } catch (err: unknown) {
      setErrorMsg(String(err));
    }
  };

  const handleTriggerGoEvent = async () => {
    try {
      await PoCService.TriggerGoEvent();
    } catch (err: unknown) {
      setErrorMsg(String(err));
    }
  };

  const handleEmitReactEvent = () => {
    Events.Emit('estiraha:react-event', { sentBy: 'react-ui', lang, ts: Date.now() });
    appendLog('React->Go Event emitted (estiraha:react-event)');
  };

  const handleTrayCycle = async () => {
    try {
      const snap = await PoCService.RunTrayCycle(1200);
      setTrayState(snap);
      setTimeout(async () => {
        const refreshed = await PoCService.GetPlatformDiagnostics();
        setTrayState(refreshed.tray);
        appendLog(`Tray cycle verified: hides=${refreshed.tray.hideCount}, restores=${refreshed.tray.restoreCount}`);
      }, 1500);
    } catch (err: unknown) {
      setErrorMsg(String(err));
    }
  };

  const handleHideToTray = async () => {
    try {
      const snap = await PoCService.HideWindowToTray();
      setTrayState(snap);
    } catch (err: unknown) {
      setErrorMsg(String(err));
    }
  };

  const handleTestCredManager = async () => {
    try {
      const report = await PoCService.TestCredentialManager();
      setCredReport(report);
      appendLog(`CredManager (${report.backend}): write=${report.writeSucceeded} purge=${report.confirmedPurged}`);
    } catch (err: unknown) {
      setErrorMsg(String(err));
    }
  };

  return (
    <div className="poc-shell" dir={localeCfg.dir}>
      <aside className="poc-sidebar" id="directional-sidebar">
        <div className="poc-brand">
          <h1>Estiraha Next</h1>
          <p>{t.subtitle}</p>
        </div>
        <span className="poc-badge">{t.safetyTag}</span>
        <p style={{ fontSize: '0.85rem', color: 'var(--text-muted)' }}>{t.sidebarNote}</p>
        <dl className="poc-kv">
          <dt>Direction</dt>
          <dd>{localeCfg.dir.toUpperCase()}</dd>
          <dt>Sidebar</dt>
          <dd>{localeCfg.sidebarPosition.toUpperCase()}</dd>
          <dt>Theme</dt>
          <dd>{theme.toUpperCase()}</dd>
        </dl>
        <div style={{ marginTop: 'auto', display: 'flex', flexDirection: 'column', gap: '10px' }}>
          <button onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}>{t.switchTheme}</button>
          <button className="danger" onClick={() => void PoCService.RequestCleanQuit()}>
            {t.quitClean}
          </button>
        </div>
      </aside>

      <main className="poc-main" id="poc-workspace">
        <header className="poc-topbar">
          <div>
            <strong>Estiraha Next — استراحة نكست</strong>
            {errorMsg && <div style={{ color: '#ef4444', fontSize: '0.82rem' }}>{errorMsg}</div>}
          </div>
          <div className="poc-btn-row">
            <button className="primary" id="toggle-locale-btn" onClick={() => void handleToggleLanguage()}>
              {t.switchLang}
            </button>
          </div>
        </header>

        <section className="poc-grid">
          <article className="poc-card" id="diagnostics-card">
            <h2>{t.diagTitle}</h2>
            {diag ? (
              <dl className="poc-kv">
                <dt>Go / Wails</dt>
                <dd>
                  {diag.goVersion} / {diag.wailsVersion}
                </dd>
                <dt>OS / Arch / PID</dt>
                <dd>
                  {diag.os}/{diag.arch} (PID {diag.processId})
                </dd>
                <dt>WebView2</dt>
                <dd>
                  {diag.webView2.available ? `OK (${diag.webView2.version})` : 'Missing'}
                </dd>
                <dt>SQLite Schema</dt>
                <dd>
                  v{diag.schemaVersion} | FK={String(diag.foreignKeysOn)} | FTS5={String(diag.fts5Enabled)}
                </dd>
                <dt>Secrets Vault</dt>
                <dd>{diag.secretsBackend}</dd>
              </dl>
            ) : (
              <p>Loading runtime diagnostics...</p>
            )}
          </article>

          <article className="poc-card" id="bridge-card">
            <h2>{t.bridgeTitle}</h2>
            <input
              id="rpc-message-input"
              className="poc-input"
              value={rpcInput}
              onChange={(e) => setRpcInput(e.target.value)}
            />
            <div className="poc-btn-row">
              <button className="primary" onClick={() => void handleSendRpc()}>
                {t.sendRpc}
              </button>
              <button onClick={() => void handleTriggerGoEvent()}>{t.triggerGoPush}</button>
              <button onClick={handleEmitReactEvent}>{t.emitReactEvent}</button>
            </div>
            {lastPing && (
              <dl className="poc-kv">
                <dt>Normalized AR</dt>
                <dd>{lastPing.normalizedArabic}</dd>
                <dt>Go Reply</dt>
                <dd>{lang === 'ar' ? lastPing.replyAr : lastPing.replyEn}</dd>
              </dl>
            )}
          </article>

          <article className="poc-card" id="tray-cred-card">
            <h2>{t.traycredTitle}</h2>
            <div className="poc-btn-row">
              <button className="primary" onClick={() => void handleTrayCycle()}>
                {t.runTrayCycle}
              </button>
              <button onClick={() => void handleHideToTray()}>{t.hideToTray}</button>
              <button onClick={() => void handleTestCredManager()}>{t.testCredMgr}</button>
            </div>
            {trayState && (
              <dl className="poc-kv">
                <dt>Tray Status</dt>
                <dd>
                  init={String(trayState.initialized)} | hides={trayState.hideCount} | restores={trayState.restoreCount}
                </dd>
              </dl>
            )}
            {credReport && (
              <dl className="poc-kv">
                <dt>Vault Probe</dt>
                <dd>
                  write={String(credReport.writeSucceeded)} | sha256Match={String(credReport.hashMatched)} | purged=
                  {String(credReport.confirmedPurged)} (hash:{credReport.payloadSha256Short})
                </dd>
              </dl>
            )}
          </article>

          <article className="poc-card" id="bilingual-card">
            <h2>{t.bilingualSampleTitle}</h2>
            <p dir="rtl" style={{ fontSize: '0.95rem' }}>
              {t.bilingualSampleAr}
            </p>
            <p dir="ltr" style={{ fontSize: '0.92rem', color: 'var(--text-muted)' }}>
              {t.bilingualSampleEn}
            </p>
            <h2 style={{ marginTop: '6px' }}>{t.eventStreamTitle}</h2>
            <div className="poc-log">
              {eventLogs.map((line, idx) => (
                <div key={idx}>{line}</div>
              ))}
            </div>
          </article>
        </section>
      </main>
    </div>
  );
}
