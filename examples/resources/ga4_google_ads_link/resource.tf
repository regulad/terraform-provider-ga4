resource "ga4_google_ads_link" "ads" {
  property    = ga4_property.app.id
  customer_id = "1234567890"

  # If omitted, true on creation
  ads_personalization_enabled = false
}
