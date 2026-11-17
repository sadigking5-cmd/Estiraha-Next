# ESTIRAHA NEXT — NON-NEGOTIABLE SAFETY & PROTECTION LAWS (`SAFETY_RULES.md`)

Estiraha Next manages irreplaceable multi-terabyte personal media archives. Every module in `internal/` is bound by the following **12 Iron Safety Laws**:

1. **Zero Execution Without Explicit User Approval:**
   - Scanner, Organizer, Duplicate Detector, Subtitle/Artwork Cleaners, and Disk Watcher **never** mutate files directly.
   - Every proposed rename, move, error-folder relocation, or trash action must enter `review_batches` / `review_items` and hold an explicit user-approved `ExecutionPermit`.

2. **Never Overwrite Existing Files:**
   - `fsops.VerifySafeDestination` checks destination existence before every move/rename and uses exclusive creation flags. Any target collision immediately aborts that item (`aborted_target_exists`) without touching either file.

3. **Zero Automatic Deletion:**
   - Neither duplicates, lower-quality versions, redundant subtitles/images, nor unknown extras are ever deleted automatically.
   - Deletion is **100% manual** after item-by-item selection and explicit confirmation.

4. **Internal Trash with Manual-Only Purge:**
   - Soft-deleted items move to the managed Trash vault (`trash_items`) retaining origin disk, path, and SHA-256 hash. Trash is emptied **manually only**.

5. **Full Pre/Post Preview & Granular Control:**
   - Users inspect side-by-side Old Path/Name vs. Proposed Path/Name, edit suggested names inline, exclude specific files, approve independent batches, or reject/defer items.

6. **Power-Loss & Disk-Disconnect Recovery:**
   - Staged execution writes substeps (`queued` -> `precheck_passed` -> `in_progress` -> `verifying_hash` -> `completed`) into `operation_journal` (`WAL` synced).
   - If power fails or an external USB/SATA disk disconnects mid-batch:
     - Affected operations halt immediately (`interrupted_power_or_disk`).
     - Original source files are preserved until destination integrity is verified.
     - On next launch, Estiraha Next offers verified **Resume** or **Safe Rollback** via `rollback_steps`.

7. **Read-Only Disk Watcher:**
   - `internal/watcher` and System Tray monitoring strictly **detect and notify** (new files, missing files, disk offline/online). They possess zero write/move/delete permissions.

8. **Read-Only Archive Inspection:**
   - `.zip` and `.rar` archives are inspected in read-only mode to catalog internal entries (`archive_entries_json`), **never** automatically extracted.

9. **Cross-Disk Keep Respect:**
   - Exact SHA-256 duplicates or alternate versions stored intentionally across separate backup disks honor explicit `keep_across_disks` decisions.

10. **User Local Artwork Immunity & Cache Isolation:**
    - User-owned local images (`origin_type = 'user_local'`) are never deleted or replaced.
    - Smart Image Engine caches live strictly in `%LOCALAPPDATA%\EstirahaNext\`, never inside user media directories. Unsafe SVG files are blocked.

11. **Credential & Privacy Isolation:**
    - API keys live in **Windows Credential Manager**, never in SQLite, plaintext config files, logs, or GitHub.
    - Optional DeepSeek calls require per-request user consent and scrub all local disk paths and third-party provider payloads.

12. **Repository Hygiene:**
    - `.gitignore` and developer workflows forbid committing API keys, passwords, user SQLite databases, real media files, or personal test data to GitHub.
