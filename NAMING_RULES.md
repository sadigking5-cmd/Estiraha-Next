# ESTIRAHA NEXT — OFFICIAL MANDATORY NAMING RULES (`NAMING_RULES.md`)

> **Binding Rule:** These naming conventions are strictly mandatory across `internal/naming`, `internal/organizer`, and the Review Center. Speculative translation of titles is strictly prohibited. Original, Arabic, and English titles are stored separately in SQLite (`title_original`, `title_ar`, `title_en`).

---

## 1. Movies (`movie`)
- **Canonical Format:** `Movie Name + Production Year`
- **Example:** `The Dark Knight 2008`
- **Arabic Movie Rule:** Retains its authentic Arabic title + production year (e.g., `الناظر 2000`).
- **Strictly Prohibited in Names:**
  - Parentheses or brackets around the year (`(2008)`, `[2008]`).
  - Quality/resolution tags (`1080p`, `4K`, `BluRay`, `WEB-DL`).
  - Codec/audio tags (`x265`, `HEVC`, `AAC`, `Atmos`).
  - Release group names (`YIFY`, `RARBG`, etc.).
  - Decorative symbols or dot-separators (`The.Dark.Knight.2008`).
- **Directory Structure:** Every movie lives in its own dedicated folder named `Movie Name YYYY`:
  ```text
  Movies/
    The Dark Knight 2008/
      The Dark Knight 2008.mkv
      The Dark Knight 2008.srt
      The Dark Knight 2008.jpg
      The Dark Knight 2008.shorts
  ```

---

## 2. TV Series (`series`, `anime`, `cartoon` episodic)
- **Canonical Main Folder:** Series Title **WITHOUT** production year.
- **Season Subfolders:** `S01`, `S02`, ...
- **Episode Filenames:** `01.mp4`, `02.mp4`, ...
- **Celebrity Filmography Display Rule:** Series NEVER show production years in folder names or celebrity series lists.
- **Directory Structure:**
  ```text
  باب الحارة/
    poster.jpg
    S01/
      01.mp4
      02.mp4
    S02/
      01.mp4
      02.mp4
  ```

---

## 3. TV Programs (`tv_program`, `talk_show`, `game_show`, `sports_program`, `entertainment_program`)
- **Known / Identified Program:**
  - Recognized via local rules or verified API Hub claims.
  - Seasons and episodes managed systematically; missing episodes detected.
- **Unknown / Unidentified Program:**
  - **Retains its original name** (sanitized only for OS-illegal characters).
  - **Zero automatic season folder creation** (`AllowAutoSeasons = false`).
  - **Routed automatically to the Review Center** (`identification_status = 'unknown_program'`).

---

## 4. Cinematic Franchises (`franchises`)
- **Canonical Parent Folder:** Franchise Name **WITHOUT** production year.
- **Child Movie Folders:** Each film sits in its own subfolder obeying Section 1 Movie Naming Rules:
  ```text
  The Lion King/
    The Lion King 1994/
    The Lion King 2019/
    Mufasa The Lion King 2024/
  ```

---

## 5. Celebrities / People (`people`)
- **Canonical Name Format:** `الاسم بالعربية (English Name)`
- **Example:** `أحمد زكي (Ahmed Zaki)`
- **Works Organization & Display Rules:**
  - Movies and Series are displayed in **two separate sections** (`أفلام` / `مسلسلات`).
  - Movie titles **include** production year (`الناظر 2000`, `ضد الحكومة 1992`).
  - Series titles **exclude** production year.
  - Linked virtually in SQLite (`work_credits`) **without duplicating physical video files** on disk:
  ```text
  أحمد زكي (Ahmed Zaki)/
    مسلسلات/
      الأيام/
      هو وهي/
    أفلام/
      الناظر 2000/
      أرض الخوف 2000/
      ضد الحكومة 1992/
  ```

---

## 6. Documentaries (`documentary_movie`, `documentary_series`)
- **Dedicated Library Section:** Documentaries live in an independent top-level library category.
- **Standalone Documentary (`documentary_movie`):** Obeys **Movie** naming rules (`Title YYYY`).
- **Episodic Documentary Series (`documentary_series`):** Obeys **Series** naming rules (Yearless main folder + `S01/01.ext`).
