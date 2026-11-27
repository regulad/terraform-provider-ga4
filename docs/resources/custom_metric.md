# ga4_custom_metric (Resource)

A GA4 custom metric.

**GA4 has no delete, so destroy archives the metric.** Archiving cannot be undone (a new metric can be created with the same
`parameter_name`, but the archived one stays in the property). Changing `parameter_name` or `scope` archives the metric and creates it again under the new name.

## Arguments

- `property` (required, forces replacement): `properties/123`
- `parameter_name` (required, forces replacement): the event parameter name. Letters, digits and `_`, starting with a letter, at most 40 characters
- `display_name` (required): at most 82 characters
- `measurement_unit` (required): `STANDARD` / `CURRENCY` / `FEET` / `METERS` / `KILOMETERS` / `MILES` /
  `MILLISECONDS` / `SECONDS` / `MINUTES` / `HOURS`
- `scope` (optional, default `EVENT`, forces replacement): `EVENT` only
- `description` (optional): at most 150 characters
- `restricted_metric_type` (optional, empty by default): a set of `COST_DATA` / `REVENUE_DATA`.
  Required when `measurement_unit` is `CURRENCY`; leave it empty otherwise

## Attributes

- `id`: `properties/123/customMetrics/456`

## Import

```bash
terraform import ga4_custom_metric.x properties/123/customMetrics/456
```
