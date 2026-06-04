<claude-mem-context>
# Memory Context

# [keirouter] recent context, 2026-06-04 11:42pm GMT+7

Legend: 🎯session 🔴bugfix 🟣feature 🔄refactor ✅change 🔵discovery ⚖️decision 🚨security_alert 🔐security_note
Format: ID TIME TYPE TITLE
Fetch details: get_observations([IDs]) | Search: mem-search skill

Stats: 50 obs (16,043t read) | 0t work

### Jun 2, 2026
250 3:00p ✅ Quota per-row refresh toast updated, Skills.tsx toast audit started
251 " ✅ Skills.tsx toast messages improved
252 " ✅ Skills.tsx all toast messages completed
253 3:01p ✅ MediaProviderDetail.tsx toast messages improved
254 " ✅ MediaProviderDetail remove toast fixed, Usage.tsx toast audit started
255 3:02p ✅ Usage, Settings, and CLIToolDetail toast messages improved
256 3:03p 🔄 Toast component redesigned with colored backgrounds and progress bar
S9 Develop Budget engine per-key/org — hard limit USD, auto-cutoff, alert threshold (plus skills documentation ported from 9router) (Jun 2 at 3:06 PM)
S8 Develop Budget engine per-key/org with hard limit USD, auto-cutoff, alert threshold (Jun 2 at 3:06 PM)
257 4:00p ⚖️ Budget Engine Feature — Per-Key/Org USD Hard Limits with Auto-Cutoff
258 4:02p 🔵 Existing Budget Engine Architecture — Already Scaffolded with CRUD and Status
259 " 🔵 Skills System — CRUD Exists but Prompt Injection Not Wired into Pipeline
260 " 🔵 KeiRouter Server Architecture — Gateway, Pipeline, and Multi-Dialect Routing
261 " 🔵 Wire-Up Features Design Spec — Four Scaffolded Features Identified for Implementation
262 " 🔵 Connector Catalog — 60+ Providers Across 8 Dialects
263 4:04p 🔵 KeiRouter Default Port and Server Config
264 4:05p 🟣 KeiRouter Skills README Created — Hosted Skill URLs Established
265 " 🟣 KeiRouter Entry Skill Created — SKILL.md with Provider Table and Budget Error
266 " 🟣 KeiRouter Chat Skill Created — Supports Three API Formats
267 4:06p 🟣 KeiRouter Image Skill Created
268 " 🟣 KeiRouter TTS Skill Created
269 " 🟣 KeiRouter STT Skill Created
270 " 🟣 KeiRouter Embeddings Skill Created
271 4:07p 🟣 KeiRouter Web Search Skill Created
272 " 🟣 KeiRouter Skills Documentation Complete — All 8 Skill Files Created
273 4:08p 🔴 Frontend Skills Page — Fixed Placeholder GitHub URL to Actual Repo
### Jun 3, 2026
274 1:24p 🔵 Overview Page Statistical Accuracy Investigation Initiated
275 1:25p 🔵 Overview Success Rate Calculated from Only 8 Recent Requests Instead of Full Period
276 " 🔵 Overview Average Latency Uses Only 8 Recent Requests Instead of Period Aggregate
277 " 🔵 Overview Statistical Pipeline Architecture Map
278 " 🔵 KeiRouter Overview Page Stats Bug: Recent-Only Sample Misrepresents Full-Period Metrics
279 1:27p ⚖️ Implementation Plan Written for Overview Statistics Accuracy Fix
280 1:30p 🔵 Cache Hit Metering Can Use Zero-Value Attempt with Target Set
281 6:16p 🔵 XMTP Provider Connector Missing for Mimo Model Access
282 6:18p 🔵 KeiRouter Architecture: Pipeline Dispatch and Fallback Flow
283 " 🔵 MiMo Model Registration and XMTP Connector Gap
284 " 🔵 ToolArgSanitizer Fixes Malformed Tool Arguments from Non-Anthropic Models
S10 Debug 500 error "no connector for provider xmtp" when using MiMo token plan in KeiRouter (Jun 3 at 6:18 PM)
285 6:21p 🔴 TTFT metric always recorded as zero in streaming pipeline
286 10:43p 🟣 Token/Credit-Based Budget Limiting for Budget Feature
287 " 🔵 Keirouter Budget System Architecture
288 10:44p 🔵 Meter Cost Calculation and Provider Pricing Model
289 10:45p 🔵 Budget Engine and Store Files Identified
290 10:46p 🔵 Frontend Budget API Endpoints and QuotaAccount Usage Types
291 " 🔵 9router Reference Implementation Found in _research Directory
292 " 🔵 Explore Agent Reports Budget Feature Not Yet Implemented
293 10:47p 🔵 Database Migration History for Budget Extension
294 10:59p ⚖️ Budget Feature Enhancement Requested
**295** " 🔵 **KeiRouter Store Layer Architecture**
The primary session is exploring the KeiRouter backend's persistence layer to understand the existing budget feature before implementing token/credit usage-based budget limits. The store package in backend/internal/store/ provides a clean repository pattern with dialect-aware SQL, supporting both SQLite for local/single-binary usage and Postgres for team/VPS deployments. The repo_budgets.go file already exists as a typed repository, indicating the budget feature has an established persistence layer that will need to be extended.
~311t -

**296** 11:00p 🔵 **KeiRouter Backend Project Structure**
The primary session is mapping the KeiRouter backend codebase structure before implementing token/credit-based budget limits. The project is a comprehensive API gateway/router with a well-organized internal package structure. Key modules relevant to the budget enhancement include: budget/budget.go (core budget logic), store/repo_budgets.go (budget persistence), store/repo_usage.go (usage data), meter/meter.go (metering), and usagehub/hub.go (usage aggregation). The existing usage tracking infrastructure suggests the project already tracks token/credit consumption, which will be leveraged for the new budget limit dimensions.
~334t -

**297** 11:01p 🔵 **KeiRouter Has Significant Uncommitted Changes on Main**
The primary session discovered that the KeiRouter project on the main branch already has substantial uncommitted work — 32 files with nearly 1,400 lines added. This existing work touches the pipeline, connectors, gateway, store, meter, and frontend, suggesting significant in-progress development before the current budget feature request. The changes span the full stack from API gateway handling through connector integrations to the React frontend's Usage, Quota, and Keys pages. The budget/token limit feature will be built on top of these existing modifications.
~364t -

**298** 11:02p 🔵 **KeiRouter Budget System Currently USD-Only**
The primary session discovered that the current budget system in KeiRouter is entirely USD-based. The budgets table stores limits as limit_micros (micro-USD), and the SpendSince() method only aggregates cost_micros. However, usage_records already captures detailed token data (prompt_tokens, completion_tokens, cached_tokens, cache_write_tokens) per request, providing the raw data needed to implement token-based budget limits. The admin gateway's create-key endpoint already supports atomic key+budget creation with hard cutoff, suggesting the same pattern should be extended for token/credit limits.
~346t -

**299** 11:03p 🔵 **Store Migrations Directory Located**
The primary session located the migrations directory at backend/internal/store/migrations/. This is where schema migrations are stored for adding new columns or tables — any new budget limit types (token-based, credit-based) will require a new migration file. The repo_budgets.go and repo_usage.go files were both modified today (June 3), suggesting active work on these areas.
~202t -
</claude-mem-context>