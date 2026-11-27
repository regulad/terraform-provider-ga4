# A property has at most one. There is no update: changing it forces replacement
resource "ga4_firebase_link" "app" {
  property = ga4_property.app.id

  # A project number (projects/1234) or a project ID (projects/my-project)
  project = "projects/my-project"
}
