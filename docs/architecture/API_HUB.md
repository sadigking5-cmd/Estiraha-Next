# API Hub Architecture (`docs/architecture/API_HUB.md`)

## 1. Candidate Sources & Initial Operational Status
| Provider ID | Role | Initial Status | Key Constraints |
|---|---|---|---|
| `tmdb` | Movies / Series / People metadata & artwork | `disabled` (until key + terms check) | Mandatory TMDb logo/notice attribution; respect cache TTL |
| `omdb` | Movie/IMDb ratings & metadata | `disabled` | Tier rate limit & commercial clause review required |
| `tvdb` | Episodic series & seasons | `disabled` | Subscription/attribution terms check required |
| `tvmaze` | TV schedules & show episodes | `disabled` | Attribution check required |
| `trakt` | Watch tracking & metadata | `disabled` | "Powered by Trakt" badge required |
| `simkl` | Anime / TV / Movie tracking | `disabled` | API terms check required |
| `opensubtitles` | Subtitle catalog lookup | `search_only` | **Search-only in initial phases; automatic download blocked** |
| `deepseek` | Optional natural-language/metadata helper | `disabled` + `per_request_consent` | **Explicit user consent per request; local disk paths & 3rd-party payloads scrubbed** |
| `elcinema` | Arabic cinema reference | `deferred_legal_review` | **Strictly deferred until permitted official integration method is verified** |

> **Discovery Reference:** `https://github.com/public-apis/public-apis` is treated strictly as a human-reviewed discovery directory, **never** an auto-connected endpoint.

## 2. Provenance, Confidence & Conflict Resolution
- Every external value is recorded in `metadata_claims(entity_type, entity_id, provider_id, field_name, claimed_value_json, confidence_score, has_conflict)`.
- When two providers disagree (e.g., `production_year` 2000 vs 2001), both claims are stored, `has_conflict = 1` is flagged, and the UI surfaces the discrepancy for user selection rather than silently overwriting user-locked metadata.
