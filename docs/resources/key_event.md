# ga4_key_event (Resource)

A GA4 key event (formerly a "conversion"). Events with the given event name are counted as key events.
The deprecated conversion events (`conversionEvents`) are not supported.

## Arguments

- `property` (required, forces replacement): `properties/123`
- `event_name` (required, forces replacement): e.g. `purchase`
- `counting_method` (required): `ONCE_PER_EVENT` (every event) / `ONCE_PER_SESSION` (at most once per session)
- `default_value` (optional): the default used when an event has no `value` or `currency` parameter
  - `numeric_value` (required)
  - `currency_code` (required): ISO 4217, e.g. `JPY`

## Attributes

- `id`: `properties/123/keyEvents/456`
- `custom`: `true` for a custom event, `false` for a GA4 default event (such as `purchase`)
- `deletable`: whether it can be deleted. destroy fails on one where this is `false`

## Import

```bash
terraform import ga4_key_event.x properties/123/keyEvents/456
```
