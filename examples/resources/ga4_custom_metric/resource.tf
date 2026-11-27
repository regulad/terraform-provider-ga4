# Uses an event parameter as a metric
resource "ga4_custom_metric" "score" {
  property         = ga4_property.app.id
  parameter_name   = "score"
  display_name     = "Score"
  measurement_unit = "STANDARD"
  description      = "Score when a stage is cleared"
}

# A currency metric must list the kinds of restricted data it may contain
resource "ga4_custom_metric" "item_revenue" {
  property               = ga4_property.app.id
  parameter_name         = "item_revenue"
  display_name           = "Item revenue"
  measurement_unit       = "CURRENCY"
  restricted_metric_type = ["REVENUE_DATA"]
}
