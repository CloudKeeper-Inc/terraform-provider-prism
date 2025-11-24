# Identity providers can be imported using the type and alias
# Format: type:alias (though alias is auto-generated from type)
terraform import prism_identity_provider.google "google:google"
terraform import prism_identity_provider.microsoft "microsoft:microsoft"
