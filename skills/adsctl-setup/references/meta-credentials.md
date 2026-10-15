# Obtain Meta Marketing API credentials

Procedures checked against Meta's first-party documentation on 2026-10-15. UI labels and eligibility can change: consult the linked docs and the user's current dashboard when they differ. Standard documentation URLs can throttle; Meta's [Ads and Commerce documentation index](https://developers.facebook.com/documentation/ads-commerce/llms.txt) links accessible `.md` guides. Those guides may be wrapped in an HTML `<pre>` element.

## Prerequisites and credential selection

Use the Facebook identity that legitimately has access to the intended ad account. The operator needs a [Meta developer account](https://developers.facebook.com/docs/development/register) and a Meta app with Marketing API access. For business automation, identify the business portfolio that owns or has authorized access to the app and advertising assets. Obtain the ad account number from [Ads Manager](https://adsmanager.facebook.com/) and use `act_<number>` in adsctl; a business portfolio number is not an ad account number.

Open [My Apps](https://developers.facebook.com/apps/) and reuse a suitable app, or create an app for the requested business and enable the Marketing API product/use case. Follow the current [app creation](https://developers.facebook.com/docs/development/create-an-app) flow: labels vary, so confirm that the result exposes Marketing API permissions rather than relying on a hard-coded app-type label. Do not create another app just because an existing one has an access error. Inspect the app's roles and Permissions and Features screen first.

| Need | Credential route | Access to establish |
| --- | --- | --- |
| Initial interactive account/report checks | User access token through Graph API Explorer | The signed-in user has the intended ad account role; the token belongs to the chosen app |
| Unattended business scripts or runners | System-user access token through Business Settings | Business/app association and explicit asset assignments to that system user |
| Other businesses' accounts | Authorized partner asset sharing and/or a supported client authorization flow | Owner consent and required app access approval; a new token alone does not establish access |

Use a user or system-user Marketing API token. An app access token, Page-only token, or a token generated solely for Conversions API does not establish the required ad-account access. adsctl currently implements Meta advertising commands, not general WhatsApp or every Graph API workflow.

## Permissions and access gates

Start with the required capability, then inspect the current endpoint permission requirements:

| Capability | Permission guidance |
| --- | --- |
| Ad-account configuration reads and ad performance reports | `ads_read` |
| Create/update/manage advertising resources | `ads_management` |
| Business discovery and business asset-management endpoints | `business_management` when required; not an automatic prerequisite for every account report |
| List the user's Pages | `pages_show_list`, with access to the intended Page |
| Page data and Page-backed ad operations | Check `pages_read_engagement`, `pages_manage_ads`, and the required Page tasks for the specific endpoint |
| Instagram identity, pixels/datasets, catalogs, or other assets | Check that endpoint's current permissions and assign the intended asset; account access alone is insufficient |

`read_insights` concerns other insights surfaces and is not a universal extra permission for Marketing API ad reports. Do not request every permission in an example or copy catalog/collaborative-ad scopes into ordinary reporting setup. See the [permissions reference](https://developers.facebook.com/docs/permissions).

Check four distinct layers: token scopes, the token identity's asset roles, the app's permission access levels, and its Marketing API access tier. Meta documents Standard/Advanced **permission access** separately from Limited/Full **Marketing API access tiers**. Own-account permission access can use Standard; other people's accounts require the relevant Advanced permissions. Limited tier is described as development access; inspect Full tier eligibility for production automation. App Review, business verification, app mode/roles, and client consent can add requirements. Follow the current dashboard rather than promising that a token or a Live-mode toggle bypasses these gates. [Meta authorization guide](https://developers.facebook.com/documentation/ads-commerce/marketing-api/get-started/authorization.md)

## Route A: user token for initial testing

1. Open [Graph API Explorer](https://developers.facebook.com/tools/explorer/) under the authorized Facebook identity.
2. Select the intended app in **Meta App**, then **User Token** in **User or Page**. Add only the permissions needed for the requested commands.
3. Select **Generate Access Token** and complete Meta's login/consent flow, selecting the intended assets if prompted. A missing permission is a reason to inspect app configuration/access eligibility, not to switch to a different person's token.
4. In Meta's [Access Token Debugger](https://developers.facebook.com/tools/debug/accesstoken/), check the app ID, token identity/type, validity, granted scopes, and expiration/data-access expiration where shown. The user should perform secret entry without exposing the token in chat or a captured screenshot.
5. If appropriate and offered, select **Extend Access Token**, then recheck its properties. Short-lived user tokens are suitable for a quick check; long-lived user tokens commonly last around 60 days, but use the actual displayed expiry. Transfer the resulting token directly into the secret store and configure its reference in adsctl.

Token acquisition and extension happen outside adsctl. Programmatic exchanges require the app's server-side secret and a secure implementation; do not construct a browser URL or shell command containing literal secrets. [Meta authentication guide](https://developers.facebook.com/documentation/ads-commerce/marketing-api/get-started/authentication.md), [long-lived tokens](https://developers.facebook.com/documentation/facebook-login/guides/access-tokens/get-long-lived)

## Route B: system-user token for automation

1. Open [Business Settings](https://business.facebook.com/settings/) and select the intended portfolio. Use an authorized business administrator to perform administration; that does not mean the automation identity needs an admin role.
2. Confirm the intended developer app is owned by/associated with the correct business in its Apps settings and is eligible for system-user use. Resolve ownership and partner access through authorized business procedures; do not move ownership as a troubleshooting shortcut.
3. Under **Users → System Users**, select a suitable existing system user or add one when authorized. Choose the least role that supports the task; do not default to Admin.
4. Use **Add Assets/Assign Assets** to give that identity the intended ad account tasks: reporting/view access for reads, or advertising management for writes. Assign the app as required for token generation. Add Page, Instagram, pixel/dataset, or catalog assets only when the workflow uses them. A human administrator's asset access is not automatically inherited by the system user.
5. Select the system user, choose **Generate new token**, choose the intended app, select the available expiration option, and grant only the needed permissions. If the app or scopes are absent, check app/business association, app assignment to the system user, the operator's role, and app access eligibility before retrying.
6. Copy the token directly to the secret store; Meta may show it only once. Check its app, identity, permissions, and validity in Meta's debugger, then run adsctl's bounded account check from the actual runner.

If the dashboard offers a token without scheduled expiration, record that observed property rather than promising permanent validity. Tokens remain revocable and can stop working when app eligibility, business access, or asset assignments change.

See [System Users](https://developers.facebook.com/docs/marketing-api/system-users/), [install apps and generate tokens](https://developers.facebook.com/docs/marketing-api/system-users/install-apps-and-generate-tokens/), and [add system users](https://www.facebook.com/business/help/503306463479099?id=2190812977867143). Meta's [token-generation UI example](https://developers.facebook.com/documentation/ads-commerce/marketing-api/collaborative-ads/managed-partner-ads/api-guide/prerequisites/generate-access-token-system-user.md) is for collaborative ads; its admin role and catalog permissions are specific to that workflow.

## Client accounts and independent asset checks

For another business, have its owner grant the required partner access or complete the supported client authorization flow, then assign shared assets to the intended identity. Never request the client's Facebook password or assume possession of an account ID is authorization. adsctl has no built-in client OAuth onboarding; an external integration must implement it when needed. Keep client profiles and secret references distinct.

Check a known ad account directly even if discovery is empty. adsctl's account discovery uses `/me/adaccounts`, business discovery uses `/me/businesses`, and Page discovery uses `/me/accounts`; these have different identity and permission semantics. A Page discovery failure does not invalidate a successful ad-account read, and passing `--account` to Page discovery does not filter `/me/accounts` to that account. Verify that returned asset IDs are the intended ones.

## Diagnose and rotate

| Observation | Next useful check |
| --- | --- |
| Missing/unreadable token reference; exit 2 | Variable is injected into the actual runner, or raw-token file exists and is readable; `config validate` alone does not check this |
| Invalid/expired token; typically exit 3 | Debug validity, app ID, token type, expiration and revocation; replace the secret only after identifying the cause |
| Permission/account access denied; typically exit 4 | Correct account ID, scopes, identity asset tasks, client sharing, permission access level and app eligibility |
| App missing or no permissions during system-user generation | Correct business, app ownership/association, app assignment, operator role, and Marketing API setup |
| Account read succeeds; Page/Instagram/creative work fails | Specific asset assignment and endpoint permissions; auth check did not establish these |
| App-secret proof rejected | Secret belongs to the app that issued the token and is available through `--app-secret-env` |
| Throttling/network/provider failure | Inspect the normalized error and provider code; these are not evidence that broader permissions are needed |

Prefer the actual structured error over exit-code guesses. After a cause is corrected, perform one bounded read again. Stop for missing grants, review, login, or eligibility that only the credential owner can resolve and state the exact next action.

For rotation, generate a replacement through the chosen Meta flow, update the existing secret-store reference atomically where supported, and verify the replacement from the runner. Revoke the old token only within the user's authorized scope and after checking other consumers. Deleting an adsctl profile does not revoke its Meta token; rotating a token does not automatically repair missing asset assignments. adsctl never refreshes tokens on its own.
