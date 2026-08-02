resource "prism_customer_managed_policy" "s3_read" {
  name        = "S3ReadOnly"
  description = "List and read S3 buckets"
  policy_document = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:Get*", "s3:List*"]
      Resource = "*"
    }]
  })
}
