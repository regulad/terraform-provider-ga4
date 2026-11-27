# Each property has exactly one of these settings. destroy leaves the settings in GA4 unchanged
resource "ga4_data_retention_settings" "app" {
  property = ga4_property.app.id

  # Anything omitted keeps the value in GA4
  event_data_retention            = "FOURTEEN_MONTHS"
  user_data_retention             = "FOURTEEN_MONTHS"
  reset_user_data_on_new_activity = true
}
