# Accounts cannot be created through the API, so import one made in the admin UI before applying
import {
  to = ga4_account.main
  id = "accounts/123"
}

resource "ga4_account" "main" {
  display_name = "My Company"

  # Optional. If omitted, the value in GA4 is kept
  region_code = "JP"
}

resource "ga4_property" "app" {
  account      = ga4_account.main.id
  display_name = "My App"
  time_zone    = "Asia/Tokyo"
}
