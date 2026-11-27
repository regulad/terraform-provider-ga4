resource "ga4_data_stream" "web" {
  property     = ga4_property.app.id
  display_name = "My App (web)"

  # Measurement Protocol だけで使うなら値は動作に影響しない
  default_uri = "https://example.com"
}

output "measurement_id" {
  # G-XXXXXXXXXX
  value = ga4_data_stream.web.measurement_id
}

# An Android app stream. Changing package_name forces replacement
resource "ga4_data_stream" "android" {
  property     = ga4_property.app.id
  display_name = "My App (Android)"

  android_app_stream_data = {
    package_name = "com.example.myandroidapp"
  }
}

# An iOS app stream. Changing bundle_id forces replacement
resource "ga4_data_stream" "ios" {
  property     = ga4_property.app.id
  display_name = "My App (iOS)"

  ios_app_stream_data = {
    bundle_id = "com.example.myiosapp"
  }
}

output "android_firebase_app_id" {
  # For app streams, pass this as the Measurement Protocol's firebase_app_id
  value = ga4_data_stream.android.firebase_app_id
}
