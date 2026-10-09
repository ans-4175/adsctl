# Meta API research and design implications

Researched on 2026-10-09 using Meta's first-party documentation, official Business SDK, official Postman workspace and Cobra documentation. Retrieved 28 Meta Markdown guides through the [official documentation index](https://developers.facebook.com/documentation/ads-commerce/llms.txt), then examined relevant guide sections and SDK resource definitions. No Meta account credentials were used, and no live advertising was read or modified.

Some ordinary documentation URLs returned throttling errors. Meta's indexed `.md` documentation was accessible directly and used for this research. Guides themselves sometimes contain old examples or disagree with other current first-party artifacts; the design distinguishes observed documentation from verified live behavior.

## Findings that determine the design

| Finding | Implication | Primary evidence |
| --- | --- | --- |
| Meta has campaign, ad-set, ad and creative resources, plus supporting assets | Use native resource commands and relationships | [Overview](https://developers.facebook.com/documentation/ads-commerce/marketing-api/overview.md) |
| Insights are edges on several advertising object levels | Separate configuration reads from reporting queries; expose scope/level | [Insights](https://developers.facebook.com/documentation/ads-commerce/marketing-api/insights.md) |
| Tokens can represent user or system-user identities | Accept both; keep credentials separate from accounts | [Authentication](https://developers.facebook.com/documentation/ads-commerce/marketing-api/get-started/authentication.md) |
| Other-business access requires appropriate permission access and grants | Document provisioning; diagnose scope and actual asset access separately | [Authorization](https://developers.facebook.com/documentation/ads-commerce/marketing-api/get-started/authorization.md) |
| Permission access levels and Marketing API access tiers are separate | Avoid one vague “access level” result in auth diagnostics | [Authorization](https://developers.facebook.com/documentation/ads-commerce/marketing-api/get-started/authorization.md) |
| Daily/lifetime budgets use account currency minimum denominations; campaign budgets exist | Explicit budget ownership and units; no universal cents conversion | [Budgets](https://developers.facebook.com/documentation/ads-commerce/marketing-api/bidding/overview/budgets.md), [campaign budgets](https://developers.facebook.com/documentation/ads-commerce/marketing-api/bidding/guides/advantage-campaign-budget.md) |
| Parent statuses affect delivery; archive/delete have lifecycle restrictions | Return configured/effective status; distinct pause/archive/delete actions | [Status](https://developers.facebook.com/documentation/ads-commerce/marketing-api/best-practices/manage-your-ad-object-status.md) |
| Creative content is generally replaced; SDK update supports limited metadata | Create replacement creative and attach it; do not promise arbitrary creative edits | [Creative SDK definition](https://github.com/facebook/facebook-python-business-sdk/blob/efd8423a2e595ea8d4c04eb824ce113f2f1d68cd/facebook_business/adobjects/adcreative.py) |
| Large reporting queries use asynchronous AdReportRun jobs | Explicit create/get/wait/results commands and resumable identifiers | [Insights best practices](https://developers.facebook.com/documentation/ads-commerce/marketing-api/insights/best-practices.md) |
| Insights fields, action types, attribution and breakdowns affect meaning | Preserve arrays; record query/attribution; require event selection for derived CPA/ROAS | [Insights SDK fields/enums](https://github.com/facebook/facebook-python-business-sdk/blob/efd8423a2e595ea8d4c04eb824ce113f2f1d68cd/facebook_business/adobjects/adsinsights.py), [account query parameters](https://github.com/facebook/facebook-python-business-sdk/blob/efd8423a2e595ea8d4c04eb824ce113f2f1d68cd/facebook_business/adobjects/adaccount.py) |
| Metric and breakdown combinations have restrictions | Missing values need availability handling rather than zero filling | [Breakdowns](https://developers.facebook.com/documentation/ads-commerce/marketing-api/insights/breakdowns.md) |
| Certain breakdowns became account-opt-in on August 6, 2026 | Flag empty results and access limitations; no unconditional async fallback promise | [2026 changes](https://developers.facebook.com/documentation/ads-commerce/marketing-api/out-of-cycle-changes/occ-2026.md) |
| Older reach reporting has additional constraints and async quotas | Surface omitted unique metrics and query-cost limitations | [Insights best practices](https://developers.facebook.com/documentation/ads-commerce/marketing-api/insights/best-practices.md) |
| Rate limiting varies by API surface, app, account and query load | Pace from returned usage headers; no fixed universal calls-per-hour budget | [Rate limits](https://developers.facebook.com/documentation/ads-commerce/marketing-api/overview/rate-limiting.md) |
| Provider errors include codes/subcodes and transient information | Preserve details and use a normalized machine error classification | [Error reference](https://developers.facebook.com/documentation/ads-commerce/marketing-api/error-reference.md), [SDK exceptions](https://github.com/facebook/facebook-python-business-sdk/blob/efd8423a2e595ea8d4c04eb824ce113f2f1d68cd/facebook_business/exceptions.py) |
| Batch APIs return individual operations; batching does not eliminate per-call rate accounting | Journal resource creation individually; batching is an optimization | [Async/batch](https://developers.facebook.com/documentation/ads-commerce/marketing-api/asyncrequests.md), [SDK batching](https://github.com/facebook/facebook-python-business-sdk#batch-calling) |
| Meta publishes its own agent-capable Ads CLI | Benchmark its noun/verb commands; build adsctl around the multi-platform and multi-client goals | [Official Ads CLI](https://developers.facebook.com/documentation/ads-commerce/ads-ai-connectors/ads-cli/ads-cli-overview.md), [command reference](https://developers.facebook.com/documentation/ads-commerce/ads-ai-connectors/ads-cli/command-reference.md) |
| Recommendations are conditional on eligibility | Offer evidence-backed read commands; do not guarantee recommendations for every account | [Opportunity score/recommendations](https://developers.facebook.com/documentation/ads-commerce/marketing-api/overview/performance-recommendations.md) |
| Ads webhooks need subscriptions and an HTTPS callback | Keep the initial CLI one-shot; design a receiver only if needed later | [Webhooks](https://developers.facebook.com/documentation/ads-commerce/marketing-api/ads-webhooks/ads-webhooks-overview.md) |
| Cobra supports inherited flags, validation and command documentation | Thin commands with central output/error handling | [Cobra flags](https://cobra.dev/docs/how-to-guides/working-with-flags/) |

## Endpoint mapping

Paths are relative to `https://graph.facebook.com/{api_version}`. Asset access and exact supported fields remain version/account dependent.

| CLI operation | Provider operation |
| --- | --- |
| `accounts list` | `GET /me/adaccounts`, subject to identity/asset visibility |
| `accounts get` | `GET /act_{id}` |
| `businesses accounts list` | Business owned/client account edges according to selected relationship |
| `campaigns list/create` | `GET/POST /act_{id}/campaigns` |
| `adsets list/create` | `GET/POST /act_{id}/adsets`; parent campaign edge for scoped listing |
| `ads list/create` | `GET/POST /act_{id}/ads`; parent campaign/ad-set edges for listing |
| Resource get/update/delete | `GET/POST/DELETE /{object_id}`, where that operation is supported |
| `creatives list/create` | `GET/POST /act_{id}/adcreatives` |
| `images list/upload` | `GET/POST /act_{id}/adimages` |
| `videos list/upload` | `GET/POST /act_{id}/advideos` |
| Preview | `GET /{ad_or_creative_id}/previews` with supported format parameters |
| `insights query` | `GET /{ad_object_id}/insights` with level/date/field/query parameters |
| `insights jobs create` | `POST /{ad_object_id}/insights` |
| `insights jobs get/wait` | `GET /{report_run_id}` |
| `insights jobs results` | `GET /{report_run_id}/insights` |
| `activity list` | Account activities edge |
| `reports`, `monitor`, `launch` | adsctl workflows composed from native operations, not single Meta endpoints |

Supporting discovery should use the appropriate identity/account/business edges rather than pretending every asset belongs to the same account hierarchy. Targeting search is a typed search interface, not a universal resource list. [Targeting search](https://developers.facebook.com/documentation/ads-commerce/marketing-api/audiences/reference/targeting-search.md), [SDK account edges](https://github.com/facebook/facebook-python-business-sdk/blob/efd8423a2e595ea8d4c04eb824ce113f2f1d68cd/facebook_business/adobjects/adaccount.py), [business edges](https://github.com/facebook/facebook-python-business-sdk/blob/efd8423a2e595ea8d4c04eb824ce113f2f1d68cd/facebook_business/adobjects/business.py).

## Evidence inconsistencies and implementation gates

1. **Version drift:** the published [version table](https://developers.facebook.com/documentation/ads-commerce/marketing-api/marketing-api-changelog/versions.md) lists v25.0 with expiration TBD. The official SDK cloned at commit `efd8423a2e595ea8d4c04eb824ce113f2f1d68cd` declares API v26.0 / SDK v26.0.2 in `apiconfig.py`. Recommend a documented v25.0 compatibility baseline first, validate against the actual app/account before release, and avoid claiming either artifact is the latest authoritative runtime state.
2. **Examples versus supported combinations:** introductory creation guides contain older objective examples, while current SDK enums include both legacy and `OUTCOME_*` names. Guided templates should use current outcome objectives and live endpoint validation; SDK enums alone do not establish creatability. [Campaign guide](https://developers.facebook.com/documentation/ads-commerce/marketing-api/get-started/basic-ad-creation/create-an-ad-campaign.md), [campaign SDK](https://github.com/facebook/facebook-python-business-sdk/blob/efd8423a2e595ea8d4c04eb824ce113f2f1d68cd/facebook_business/adobjects/campaign.py).
3. **Breakdown documentation:** the 2026 change note and general breakdown guide phrase async access differently. Preserve opt-in/availability findings and treat actual provider responses as evidence; do not promise async execution bypasses permissions or account enablement.
4. **Budget guidance:** do not hard-code an overspend percentage from an example page. Budget semantics and delivery policies can change; configured daily budget is an average, not an immediate enforcement boundary.
5. **Validation coverage:** the SDK exposes `validate_only` execution options for several creation/update operations, but support and conditional requirements must be tested per operation/version. Local plans must disclose deferred checks for resources whose parent IDs do not yet exist.
6. **Mutation recovery:** this research does not establish a universal Meta create-idempotency mechanism. Local run journals improve recovery but cannot guarantee exactly-once creation after an ambiguous network failure.
7. **Prerequisites:** test actual Page/Instagram/pixel grants for the selected token and proposed creative/objective. A successful account report cannot establish those capabilities.

These are implementation validation gates, not reasons to block writing the design. No business verification, app review, token provisioning, scheduling, deployment or advertising activation was performed during this task.
