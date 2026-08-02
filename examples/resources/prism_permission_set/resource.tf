resource "prism_customer_managed_policy" "s3_read" {
  name            = "S3ReadOnly"
  policy_document = jsonencode({ Version = "2012-10-17", Statement = [{ Effect = "Allow", Action = ["s3:Get*", "s3:List*"], Resource = "*" }] })
}

resource "prism_permission_set" "example" {
  name             = "ReadOnlyWithConsoleHome"
  description      = "Read-only access, lands on the S3 console"
  session_duration = "PT4H"
  relay_state      = "https://console.aws.amazon.com/s3/home"
  managed_policies = ["arn:aws:iam::aws:policy/ReadOnlyAccess"]

  customer_managed_policy_ids = [prism_customer_managed_policy.s3_read.id]
}
