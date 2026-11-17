# Smart Image Engine Architecture (`docs/architecture/SMART_IMAGE_ENGINE.md`)

## 1. Five Dedicated Cache Partitions
1. **Poster Cache** (`poster`)
2. **Backdrop Cache** (`backdrop`)
3. **Celebrity Image Cache** (`celebrity`)
4. **Series Artwork Cache** (`series_artwork`)
5. **Generated Thumbnails** (`thumbnail`)

## 2. Storage Budget, Eviction & Isolation
- **Initial Disk Budget:** `2 GiB` (`2,147,483,648` bytes), user-customizable via `app_settings`.
- **Two-Tier Strategy:** Bounded RAM LRU (`256 MB` default) + SQLite-indexed Disk LRU (`image_cache_entries` ordered by `pinned_offline ASC, last_accessed_at ASC`).
- **Strict Directory Isolation:** Stored exclusively under `%LOCALAPPDATA%\EstirahaNext\ImageCache\`. `imagecache.VerifyCacheDirIsolated` blocks any cache path overlapping user library roots.
- **User Local Artwork Protection:** Local user files (`origin_type = 'user_local'`) are flagged `is_protected_user_asset = 1` and can never be modified, replaced, or deleted by cache eviction or provider sync.

## 3. Security & Image Validation Pipeline
1. Reject any URL/file declaring `image/svg+xml` or containing `<svg` bytes (`ErrForbiddenSVG`).
2. Verify binary magic headers match declared `image/jpeg`, `image/png`, or `image/webp`.
3. Enforce byte cap (`15 MiB` default) and pixel bounds (`1..8192 px`) prior to full decode to prevent decompression bombs.
4. Provide UI states for **Shimmer Loading**, **Error Placeholder**, **Smart Viewport Prefetching**, and **Offline Artwork Pinning**.
