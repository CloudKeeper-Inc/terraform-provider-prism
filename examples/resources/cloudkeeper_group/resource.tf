resource "cloudkeeper_group" "developers" {
  name        = "Developers"
  description = "Development team group"
  path        = "/teams/engineering/"
}
