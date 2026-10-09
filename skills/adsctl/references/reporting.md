# Reporting and monitoring

Use explicit scope in each request. Examples assume the executable is `adsctl`; substitute its resolved path if necessary.

## Inspect resources and diagnose delivery

```sh
adsctl meta accounts get "$ACCOUNT" --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta campaigns list --profile "$PROFILE" --account "$ACCOUNT" --all
adsctl meta ads list --campaign CAMPAIGN_ID --profile "$PROFILE" --account "$ACCOUNT" --all
adsctl meta ads get AD_ID --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta issues list --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta recommendations list --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta activity list --since 2026-10-01T00:00:00Z --until 2026-10-08T00:00:00Z --profile "$PROFILE" --account "$ACCOUNT"
```

Resolve names to IDs before further operations. Discovery also includes businesses, ad sets, creatives, images, videos, Pages, Instagram accounts, pixels, audiences, and targeting; consult each group's help. Page visibility follows credential identity access and does not prove linkage to the chosen ad account. Provider recommendations are evidence to review, not instructions to apply.

## Performance questions

Prefer higher-level reports for summaries, comparisons, and rankings:

```sh
adsctl meta reports summary --date-preset last_7d --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta reports compare --level campaign --date-preset last_7d --compare previous-period --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta reports rank --level ad --metric cpa --action-type offsite_conversion.fb_pixel_purchase --min-spend 25.00 --order asc --top 10 --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta reports breakdown --level campaign --breakdowns publisher_platform,platform_position --date-preset last_7d --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta reports export --level ad --since 2026-10-01 --until 2026-10-07 --format csv --directory NEW_EXPORT_DIRECTORY --profile "$PROFILE" --account "$ACCOUNT"
```

Replace sample dates and the sample purchase action with the user's requested period and actual conversion event. CPA/ROAS requires an explicit action type; minimum spend uses account currency major units. Zero denominators and missing actions remain unavailable. Explain excluded entities or insufficient evidence instead of assigning them a zero CPA/ROAS.

Higher-level `last_Nd` reports use completed account-local days. Use `--include-partial-period` deliberately for periods including today. Native Insights presets retain Meta semantics. Comparisons require entity totals without breakdowns. Account totals and campaign contributors are separate; never sum reach across entities. Do not sum currencies across accounts.

Export requires a new directory and finalizes rows plus `metadata.json` only after a complete read. Native nested values are JSON in CSV cells. Named portfolios are explicit configured profile/account pairs, for example `reports summary --portfolio clients --concurrency 2`; portfolio results preserve account/currency separation. Export is single-account.

## Native Insights and large queries

```sh
adsctl meta insights query --level ad --date-preset last_7d --time-increment 1 --fields ad_id,ad_name,spend,impressions,clicks,actions --attribution adset --action-report-time impression --all --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta insights jobs create --level ad --date-preset last_30d --time-increment 1 --wait --timeout 10m --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta insights jobs get JOB_ID --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta insights jobs wait JOB_ID --timeout 10m --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta insights jobs results JOB_ID --all --profile "$PROFILE" --account "$ACCOUNT"
```

Explicit dates require both `--since` and `--until` and replace the date preset. Root filters `--campaign`, `--adset`, and `--ad` are mutually exclusive; select the root or a descendant reporting level. Attribution is `adset`, `account`, or `windows`; explicit windows require `--attribution windows --attribution-windows 7d_click,1d_view`. Record attribution and action report time with conclusions, and match them when comparing Ads Manager results. Ad set attribution can differ across entities.

Async create returns a report run ID. `--wait` waits for status; fetch rows separately with `jobs results`. After a wait timeout, retain the returned ID and wait on the same job instead of creating another. Use async queries when synchronous requests exceed practical deadlines; higher-level reports do not automatically switch to async.

## Monitor over time

Write a rule document to a user-selected file; adapt thresholds, currency, event, and window to the account:

```yaml
version: 1
id: daily-health
rules:
  - id: rejected-ads
    type: delivery-status
    resource: ads
    effective_status_in: [DISAPPROVED, WITH_ISSUES]
  - id: purchase-cpa
    type: metric-threshold
    resource: campaign
    date_preset: last_7d
    metric: cpa
    operator: gt
    threshold: '30.00'
    action_type: offsite_conversion.fb_pixel_purchase
    attribution: adset
    action_report_time: impression
    currency: USD
    min_spend: '100.00'
    min_actions: 5
    exclude_recent_days: 1
```

```sh
adsctl meta monitor validate --file rules.yaml
adsctl meta monitor check --file rules.yaml --profile "$PROFILE" --account "$ACCOUNT" --state-dir "$STATE_DIR" --fail-on-finding
```

Validation is local. Check is one-shot and read-only, with persisted started/ongoing/recovered transitions. `exclude_recent_days` shifts the completed window to buffer conversion delay. Incomplete evidence leaves prior state unchanged; do not report recovery from an unavailable metric or failed read. Reuse rule IDs and state directory to preserve transitions. Scheduling and notifications require an external runner; `adsctl` does not create them or optimize ads automatically.

## Configuration changes

```sh
adsctl meta snapshots create --save before.json --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta snapshots create --save after.json --profile "$PROFILE" --account "$ACCOUNT"
adsctl meta snapshots diff --before before.json --after after.json
```

Choose unused output paths. Diff requires complete inventories for the same account/API version. Snapshots are configuration evidence, not transactional reads; never infer deletion from an incomplete inventory.
