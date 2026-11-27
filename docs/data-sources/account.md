# ga4_account (Data Source)

Looks up a GA4 account by display name. Accounts cannot be created through the API, so use this to refer to one without managing it.
To manage its display name and country code with Terraform as well, import it as a [`ga4_account` resource](../resources/account.md).

## 引数

- `display_name`（必須）: アカウントの表示名。完全一致で 1 件に絞れる必要がある

## 属性

- `id`: アカウントのリソース名（`accounts/123`）。`ga4_property` の `account` に渡す
- `region_code`: 国コード（例: `JP`）
