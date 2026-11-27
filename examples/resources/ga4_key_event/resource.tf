resource "ga4_key_event" "purchase" {
  property        = ga4_property.app.id
  event_name      = "purchase"
  counting_method = "ONCE_PER_EVENT"
}

# Gives events without a value or currency parameter a default
resource "ga4_key_event" "sign_up" {
  property        = ga4_property.app.id
  event_name      = "sign_up"
  counting_method = "ONCE_PER_SESSION"

  default_value = {
    numeric_value = 100
    currency_code = "JPY"
  }
}
