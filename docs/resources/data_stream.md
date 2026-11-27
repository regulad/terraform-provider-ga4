# ga4_data_stream (Resource)

A GA4 data stream: one of a web, Android app or iOS app stream.
The kind is determined by which of `default_uri` (web), `android_app_stream_data` and `ios_app_stream_data` is written,
and exactly one must be written.

Configuration and state from the earlier version (web streams only, `default_uri` required) show no diff as they are.

## 引数

- `property`（必須、変更で作り直し）: `properties/123`
- `display_name`（必須）
- `default_uri` (for a web stream): the stream's URL. If the stream is used only with the Measurement Protocol, the value has no effect
- `android_app_stream_data` (for an Android app stream, forces replacement)
  - `package_name` (required): e.g. `com.example.myandroidapp`
- `ios_app_stream_data` (for an iOS app stream, forces replacement)
  - `bundle_id` (required): the App Store bundle ID, e.g. `com.example.myiosapp`
- `type` (optional, forces replacement): `WEB_DATA_STREAM` / `ANDROID_APP_DATA_STREAM` / `IOS_APP_DATA_STREAM`.
  If omitted, it is determined by which of the above is written. If written, it must match that

## 属性

- `id`: `properties/123/dataStreams/456`。`ga4_measurement_protocol_secret` の `data_stream` に渡す
- `measurement_id`: `G-XXXXXXXXXX`. Web streams only (null for app streams)
- `firebase_app_id`: the ID of the corresponding Firebase app. For app streams, pass it as the Measurement Protocol's
  `firebase_app_id`. Usually empty for web streams

## Import

```bash
terraform import ga4_data_stream.x properties/123/dataStreams/456
```
