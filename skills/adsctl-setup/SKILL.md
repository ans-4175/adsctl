---
name: adsctl-setup
description: "Install and configure adsctl, guide users through obtaining Meta Marketing API user or system-user tokens, assign required asset access, and diagnose credential setup failures. Use for adsctl onboarding, credential rotation, or authentication and permission setup; campaign operations and performance analysis belong to the adsctl operations skill."
---

# adsctl Setup

Set up the adsctl CLI with external credentials and demonstrate access to the intended Meta ad account through bounded reads. adsctl consumes an existing token; it does not create Meta apps, run OAuth, or refresh tokens. This workflow uses Meta's own developer and business tools, without requiring an Ads Manager connector.

## Establish the installation and intended access

- Find `adsctl` on PATH and inspect `adsctl version` and `adsctl config path`. The default home is `~/.adsctl`, with `bin/adsctl`, `config.yaml`, optional `secrets/`, `state/`, and a generated `env` shell file. `ADSCTL_HOME` overrides that root. In a source checkout, `bin/adsctl` or `make build` is also usable (Go 1.25.1 or later). Do not assume the skill directory contains the repository.
- Establish the intended business portfolio, ad account ID, read versus advertising-write needs, and whether this is personal testing, unattended automation, or access to another business's accounts. Ask only for missing scope or choices that change the credential route. App ID, business ID, Page ID, and ad account ID are different identifiers; keep IDs as strings.
- Inspect `adsctl config path`, `adsctl config profiles list`, and relevant `--help`. These commands expose references rather than secret values. Reuse the intended profile when suitable; preserve unrelated profiles and defaults. All examples below require replacing illustrative account IDs and file paths.

## Install the executable

For macOS/Linux, use the published installer when the CLI is missing or needs an upgrade:

```sh
curl -fsSL https://github.com/imrenagicom/adsctl/releases/latest/download/install.sh | sh
. "${ADSCTL_HOME:-$HOME/.adsctl}/env"
adsctl version
adsctl config path
```

The installer chooses the OS/architecture, verifies the release archive's SHA-256 checksum, checks executable and `ADSCTL_HOME` compatibility, and installs into the home's `bin/` directory. It creates a starter config only if absent and preserves secrets and state. It prints the exact commands to add its source line once to the user's shell startup file and reload it. The generated `env` file sets `ADSCTL_HOME` and PATH; it does not contain credentials. Use `--version vX.Y.Z` to select a compatible release, or an absolute `ADSCTL_HOME` for another root. Do not treat an installer failure as a completed installation.

If the release does not include the installer yet, download the matching archive and `checksums.txt` from [adsctl releases](https://github.com/imrenagicom/adsctl/releases), verify the checksum, and install the executable in `~/.adsctl/bin` (or `$ADSCTL_HOME/bin`). Add that directory to PATH and verify `adsctl version` and `config path`; an older binary may still use legacy defaults. Windows uses the release ZIP and manual PATH setup; persistent launch/monitor execution is unsupported there.

For agent tools that start a new shell for each command, source the generated env file in each invocation, supply the home/PATH through the runner, or use the installed binary's absolute path. Shell exports in one process do not alter another process. Do not claim persistent PATH setup from a temporary export.

Existing legacy configuration/state is not automatically moved. Keep using it through explicit `--config`/`--state-dir`, or move it into the new home while no adsctl commands are running and adjust secret references. Preserve existing data and do not initialize an empty profile over a populated configuration.

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

To keep credentials under the same home, have the credential owner store the raw token (plus optional trailing newline) in `~/.adsctl/secrets/meta-read-token` or the matching `ADSCTL_HOME` location, with file mode 0600. Reference it relative to `config.yaml`:

```sh
adsctl config profiles set client-read --platform meta \
  --token-file secrets/meta-read-token \
  --api-version v25.0 --mode read
```

Use exactly one of `--token-env` and `--token-file`. Relative file references resolve from the selected config file's directory, including an explicit `--config` location. Absolute file references and external secret stores remain supported. Keep secret files outside the checkout and restrict access to the runner (directory mode 0700, file mode 0600 on Unix). Ensure the actual execution environment receives any referenced secret variables.

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
