resource "prism_group_membership" "dev_members" {
  group_name = prism_group.developers.name
  user_ids = [
    prism_user.john_doe.id,
    prism_user.jane_smith.id,
  ]
}
