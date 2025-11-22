resource "prism_permission_set" "developer" {
  name             = "DeveloperAccess"
  description      = "Developer access permissions"
  session_duration = "PT4H"

  managed_policies = [
    "arn:aws:iam::aws:policy/ReadOnlyAccess"
  ]

  inline_policies = {
    s3_access = jsonencode({
      Version = "2012-10-17"
      Statement = [
        {
          Effect = "Allow"
          Action = [
            "s3:ListBucket",
            "s3:GetObject",
            "s3:PutObject"
          ]
          Resource = "*"
        }
      ]
    })
    lambda_basic = jsonencode({
      Version = "2012-10-17"
      Statement = [
        {
          Effect = "Allow"
          Action = [
            "lambda:GetFunction",
            "lambda:ListFunctions"
          ]
          Resource = "*"
        }
      ]
    })
  }
}
