---
name: adsctl-setup
description: "Install and configure adsctl, guide users through obtaining Meta Marketing API user or system-user tokens, assign required asset access, and diagnose credential setup failures. Use for adsctl onboarding, credential rotation, or authentication and permission setup; campaign operations and performance analysis belong to the adsctl operations skill."
---

# adsctl Setup

Set up the adsctl CLI with external credentials and demonstrate access to the intended Meta ad account through bounded reads. adsctl consumes an existing token; it does not create Meta apps, run OAuth, or refresh tokens. This workflow uses Meta's own developer and business tools, without requiring an Ads Manager connector.

## Establish the installation and intended access

- Find `adsctl` on PATH and inspect `adsctl version`. In a source checkout, use `bin/adsctl` or `make build`; Go 1.25.1 or later is required by this checkout. Outside a checkout, obtain the matching OS/architecture archive from [adsctl releases](https://github.com/imrenagicom/adsctl/releases), verify its SHA-256 against `checksums.txt`, and install the binary in a user-selected PATH directory. Do not assume the skill directory is the repository. Windows persistent launch/monitor execution is unsupported.
- Establish the intended business portfolio, ad account ID, read versus advertising-write needs, and whether this is personal testing, unattended automation, or access to another business's accounts. Ask only for missing scope or choices that change the credential route. App ID, business ID, Page ID, and ad account ID are different identifiers; keep IDs as strings.
- Inspect `adsctl config path`, `adsctl config profiles list`, and relevant `--help`. These commands expose references rather than secret values. Reuse the intended profile when suitable; preserve unrelated profiles and defaults. All examples below require replacing illustrative account IDs and file paths.

## Obtain or repair credentials

Read [Meta credentials](references/meta-credentials.md) when obtaining a token, fixing permissions/asset access, or planning rotation. It gives concrete developer-app, Graph API Explorer, and Business Settings procedures.

Use a user token for initial interactive testing; prefer a system-user token for eligible business automation. Request `ads_read` for reporting and `ads_management` for advertising changes, with additional permissions only for required endpoints. Token scopes, identity asset assignments, app permission access levels, and Marketing API access tiers are separate checks.

Keep credential entry in the user's trusted Meta UI and secret store. Never ask the user to paste tokens or app secrets into chat. Never place secret values in command arguments, generated docs, tracked files, or screenshots/tool output. Do not dump environment variables or read token-file contents into the conversation. The credential owner should transfer newly generated values directly into the chosen secret store. Guide any login, consent, or two-factor step that needs the user and continue independent local work while waiting.

Credential guidance does not authorize changing business ownership, adding people, granting broad roles, revoking existing tokens, or running advertising writes. Proceed with authorized setup actions; establish authorization for any additional external changes before executing them. Do not create or activate ads to test credentials.

## Configure references

Let the runner or secret manager inject the token into a named environment variable, then configure its **name**, never its value:

```sh
adsctl config profiles set client-read --platform meta \
  --token-env META_CLIENT_READ_TOKEN --api-version v25.0 --mode read
```

Alternatively, reference a private file containing just the raw token and an optional trailing newline:

```sh
adsctl config profiles set client-read --platform meta \
  --token-file /absolute/private/path/meta-read-token \
  --api-version v25.0 --mode read
```

Use exactly one of `--token-env` and `--token-file`. Store files outside the checkout with access restricted to the runner (for example, directory mode 0700 and file mode 0600 on Unix). Shell exports in a separate process do not configure an already-running agent or future scheduler; ensure the actual execution environment receives the secret.

Only when advertising writes are requested, configure a separate profile such as `client-write` with a suitable token reference and `--mode write`. Local mode is a guard, not a grant of Meta permission. Advertising writes additionally require explicit `--profile` and `--account` flags.

`v25.0` is this repository's compatibility baseline, not a claim about Meta's latest release. Inspect installed help and existing configuration before choosing a version; verify its current availability for the app. Do not silently migrate an existing profile's API version.

If app-secret proof is required, configure `--app-secret-env META_APP_SECRET` and have the secret store inject the secret belonging to the token's app. adsctl computes the proof. An app secret is not a substitute for a user/system-user token.

## Verify access and report the result

```sh
adsctl config validate
# Discovery is useful when the account ID is unknown:
adsctl meta accounts list --profile client-read --all --max-items 100
adsctl meta auth check --profile client-read --account act_123
# When reporting is needed, verify that endpoint too:
adsctl meta insights query --profile client-read --account act_123 \
  --level account --date-preset last_7d \
  --fields spend,impressions,clicks --max-items 10
```

`config validate` validates configuration structure, not secret presence, token validity, or Meta access. `auth check` reads the selected account and reports `scopes: "unknown"`; success does not prove Page/Instagram/pixel access, every write permission, app review approval, or ad delivery. Verify additional required assets with their bounded read commands and appropriate permissions. Empty Insights can be valid for an account with no activity; do not equate empty rows with failed authentication.

Inspect exit status, JSON `ok`, `error`, `warnings`, and `meta.complete`; account discovery with `--all` remains bounded and may be incomplete. When several accounts are visible, resolve the user's intended account rather than selecting the first. Fix a demonstrated configuration or authorization cause before retrying; do not loop through accounts, broaden scopes, or regenerate tokens blindly.

Finish with the executable/version, profile name, token source **reference**, intended account, observed checks, and remaining user actions. If no token has been supplied, report that local setup is ready and live verification is pending. Never describe that state as authenticated.
