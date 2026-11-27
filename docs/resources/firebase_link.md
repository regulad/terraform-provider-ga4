# ga4_firebase_link (Resource)

A link between a GA4 property and a Firebase project. A property has at most one.

The API has no update, so **changing any argument forces replacement.**

## Arguments

- `property` (required, forces replacement): `properties/123`
- `project` (required, forces replacement): `projects/<project number>` or `projects/<project ID>`.
  The API returns only the number form, so a value written in the ID form is kept as written
  (in that case, relinking to another project elsewhere shows no diff). An import stores the number form

## Attributes

- `id`: `properties/123/firebaseLinks/456`

## Import

```bash
terraform import ga4_firebase_link.x properties/123/firebaseLinks/456
```
