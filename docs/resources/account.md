# ga4_account (Resource)

An existing GA4 account. Terraform manages its display name and country code.

**Accounts cannot be created through the API** (Admin API v1beta has only `provisionAccountTicket`, which becomes
an account only once a person accepts the terms of service in the admin UI). Import an account made in the admin UI before using it.
An apply without an import fails with an error asking for the import. To refer to an account without managing it, use `data.ga4_account`.

destroy moves the account to the trash, from which it can be restored in the admin UI for 35 days. If it is not restored, it is deleted for good, with its properties and streams.

## Arguments

- `display_name` (required)
- `region_code` (optional): the country of the business (a Unicode CLDR region code, e.g. `JP`). If omitted, the value in GA4 is kept

## Attributes

- `id`: `accounts/123`. Pass it as `ga4_property`'s `account`
- `gmp_organization`: the URI of the Google Marketing Platform organization the account is linked to, or empty if it is not linked to one

## Import

```bash
terraform import ga4_account.x accounts/123
```
