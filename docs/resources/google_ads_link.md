# ga4_google_ads_link (Resource)

A link between a GA4 property and a Google Ads account.

## Arguments

- `property` (required, forces replacement): `properties/123`
- `customer_id` (required, forces replacement): the Google Ads customer ID (10 digits, without hyphens)
- `ads_personalization_enabled` (optional): whether to enable personalized advertising features.
  If omitted, it takes GA4's default (`true`) on creation, and keeps the value in GA4 after that

## Attributes

- `id`: `properties/123/googleAdsLinks/456`. The trailing ID is not the Google Ads customer ID
- `can_manage_clients`: `true` if the linked account is a manager (MCC) account
- `creator_email_address`: the email address of the user who created the link, or empty if it is not available

## Import

```bash
terraform import ga4_google_ads_link.x properties/123/googleAdsLinks/456
```
