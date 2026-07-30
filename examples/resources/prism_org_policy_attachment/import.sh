# Org policy attachments are imported using policy_id/target_id.
# The management account is resolved automatically, so it is not part of the ID.
terraform import prism_org_policy_attachment.example "p-examplepolicyid/ou-abc1-exampleou"
