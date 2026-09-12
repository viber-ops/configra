# Limit the API Server read surface

V1 API Server accepts exact Resource Keys and exposes only Resolved Config reads plus exact File Field byte reads addressed by Environment, Vault Namespace, Item, and Field. It does not expose raw Configs, generic Text or Secret Field reads, or Config and Vault listing or search; those management views remain on Management Server. This keeps the machine contract aligned with the Viper Handler while retaining the one direct read required for File Fields.
