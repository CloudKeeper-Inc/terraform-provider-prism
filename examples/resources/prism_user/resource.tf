resource "prism_user" "john_doe" {
  username   = "john.doe"
  email      = "john.doe@example.com"
  first_name = "John"
  last_name  = "Doe"
  enabled    = true

  attributes = {
    department = "Engineering"
    location   = "US"
  }
}
