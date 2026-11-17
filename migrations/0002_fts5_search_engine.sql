-- ==============================================================================
-- ESTIRAHA NEXT — MIGRATION 0002: SQLITE FTS5 BILINGUAL LOCAL SEARCH ENGINE
-- Supports instant offline search across 200,000+ items in Arabic & English:
--   - Work titles (Original, Arabic, English), synopses, canonical names
--   - People / Celebrities (Arabic, English, Original names)
--   - Built-in SQLite trigger normalization for Arabic Alef variants (أ/إ/آ -> ا),
--     Teh Marbuta (ة -> ه), Alef Maksura (ى -> ي), and Tatweel removal (ـ)
--     alongside FTS5 `unicode61 remove_diacritics 2`.
-- ==============================================================================

CREATE VIRTUAL TABLE IF NOT EXISTS works_fts USING fts5(
    work_id UNINDEXED,
    media_type UNINDEXED,
    title_original,
    title_ar,
    title_ar_norm,
    title_en,
    canonical_folder_name,
    synopsis_ar,
    synopsis_en,
    production_year_text,
    tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TRIGGER IF NOT EXISTS trg_works_fts_insert AFTER INSERT ON works BEGIN
    INSERT INTO works_fts (
        rowid, work_id, media_type, title_original, title_ar, title_ar_norm, title_en,
        canonical_folder_name, synopsis_ar, synopsis_en, production_year_text
    ) VALUES (
        new.rowid,
        new.id,
        new.media_type,
        new.title_original,
        new.title_ar,
        REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(new.title_ar, 'أ', 'ا'), 'إ', 'ا'), 'آ', 'ا'), 'ة', 'ه'), 'ى', 'ي'), 'ـ', ''),
        new.title_en,
        new.canonical_folder_name,
        new.synopsis_ar,
        new.synopsis_en,
        COALESCE(CAST(new.production_year AS TEXT), '')
    );
END;

CREATE TRIGGER IF NOT EXISTS trg_works_fts_delete AFTER DELETE ON works BEGIN
    DELETE FROM works_fts WHERE rowid = old.rowid;
END;

CREATE TRIGGER IF NOT EXISTS trg_works_fts_update AFTER UPDATE ON works BEGIN
    DELETE FROM works_fts WHERE rowid = old.rowid;
    INSERT INTO works_fts (
        rowid, work_id, media_type, title_original, title_ar, title_ar_norm, title_en,
        canonical_folder_name, synopsis_ar, synopsis_en, production_year_text
    ) VALUES (
        new.rowid,
        new.id,
        new.media_type,
        new.title_original,
        new.title_ar,
        REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(new.title_ar, 'أ', 'ا'), 'إ', 'ا'), 'آ', 'ا'), 'ة', 'ه'), 'ى', 'ي'), 'ـ', ''),
        new.title_en,
        new.canonical_folder_name,
        new.synopsis_ar,
        new.synopsis_en,
        COALESCE(CAST(new.production_year AS TEXT), '')
    );
END;

CREATE VIRTUAL TABLE IF NOT EXISTS people_fts USING fts5(
    person_id UNINDEXED,
    name_ar,
    name_ar_norm,
    name_en,
    name_original,
    canonical_display_name,
    tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TRIGGER IF NOT EXISTS trg_people_fts_insert AFTER INSERT ON people BEGIN
    INSERT INTO people_fts (
        rowid, person_id, name_ar, name_ar_norm, name_en, name_original, canonical_display_name
    ) VALUES (
        new.rowid,
        new.id,
        new.name_ar,
        REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(new.name_ar, 'أ', 'ا'), 'إ', 'ا'), 'آ', 'ا'), 'ة', 'ه'), 'ى', 'ي'), 'ـ', ''),
        new.name_en,
        new.name_original,
        new.canonical_display_name
    );
END;

CREATE TRIGGER IF NOT EXISTS trg_people_fts_delete AFTER DELETE ON people BEGIN
    DELETE FROM people_fts WHERE rowid = old.rowid;
END;

CREATE TRIGGER IF NOT EXISTS trg_people_fts_update AFTER UPDATE ON people BEGIN
    DELETE FROM people_fts WHERE rowid = old.rowid;
    INSERT INTO people_fts (
        rowid, person_id, name_ar, name_ar_norm, name_en, name_original, canonical_display_name
    ) VALUES (
        new.rowid,
        new.id,
        new.name_ar,
        REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(new.name_ar, 'أ', 'ا'), 'إ', 'ا'), 'آ', 'ا'), 'ة', 'ه'), 'ى', 'ي'), 'ـ', ''),
        new.name_en,
        new.name_original,
        new.canonical_display_name
    );
END;
