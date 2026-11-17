# Security, Secrets & Backup Integrity (`docs/security/SECURITY_AND_SECRETS.md`)

1. **Windows Credential Manager Only:** Provider API keys (`TMDb`, `OMDb`, `TVDB`, `Trakt`, `Simkl`, `OpenSubtitles`, `DeepSeek`) are stored in Windows Credential Manager under `EstirahaNext/Providers/<Name>`. SQLite stores only the target key identifier—never the secret value.
2. **DeepSeek Data Minimization:** Disabled by default. When enabled by user, every outbound call requires explicit per-request approval and passes `ValidateOutboundRequest` to ensure zero Windows drive paths (`C:\`, `D:\`, `\\Server\`) or third-party provider payloads leave the machine.
3. **Portable Verified Backups:** Backups bundle SQLite snapshot (`VACUUM INTO`), `app_settings`, `watch_states`, and `operation_journal` alongside a `SHA-256` manifest (`backup_records.sha256_checksum`), enabling verified restoration onto a new Windows PC with disk serial/mount remapping—while deliberately excluding raw API keys for security.
4. **GitHub Leak Prevention:** `.gitignore` blocks `.env*`, `*.key`, `*.db*`, caches, logs, and all media/archive extensions.
