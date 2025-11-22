resource "cloudkeeper_permission_set_assignment" "dev_team" {
  permission_set_id = cloudkeeper_permission_set.developer.id
  principal_type    = "GROUP"
  principal_id      = cloudkeeper_group.developers.name
  account_id        = cloudkeeper_aws_account.production.account_id
}
