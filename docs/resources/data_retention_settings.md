# ga4_data_retention_settings (Resource)

A GA4 property's data retention settings. Each property has exactly one, which can be neither created nor deleted.

create and update update the settings. **destroy only removes them from state; the settings in GA4 stay as they are.**

## Arguments

- `property` (required, forces replacement): `properties/123`. On replacement, the old property's settings are removed from state unchanged
- `event_data_retention` (optional): how long event-level data is retained. `TWO_MONTHS` / `FOURTEEN_MONTHS` /
  `TWENTY_SIX_MONTHS` / `THIRTY_EIGHT_MONTHS` / `FIFTY_MONTHS` (the last three on 360 properties only)
- `user_data_retention` (optional): how long user-level data is retained. `TWO_MONTHS` / `FOURTEEN_MONTHS`
- `reset_user_data_on_new_activity` (optional): whether to extend a user identifier's retention period with each new event from that user

Omitted arguments keep the value in GA4.

## Attributes

- `id`: `properties/123/dataRetentionSettings`

## Import

Import by the property's resource name (`properties/123/dataRetentionSettings` works too).

```bash
terraform import ga4_data_retention_settings.x properties/123
```
