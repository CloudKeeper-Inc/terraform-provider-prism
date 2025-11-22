resource "cloudkeeper_group_membership" "dev_members" {
  group_name = cloudkeeper_group.developers.name
  user_ids = [
    cloudkeeper_user.john_doe.id,
    cloudkeeper_user.jane_smith.id,
  ]
}
