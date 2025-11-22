resource "prism_permission_set_assignment" "dev_team" {
  permission_set_id = prism_permission_set.developer.id
  principal_type    = "GROUP"
  principal_id      = prism_group.developers.name
  account_id        = prism_aws_account.production.account_id
}
