# Keep bootstrap configuration outside Configra

Configra's Cold-start Configuration is supplied outside its own managed configuration domain, while application and business configuration is managed by Configra. API Tokens and Client Certificates are created or registered through Management, but their plaintext Token and private-key material are provisioned to Machine Clients through an external secret channel such as a Kubernetes Secret. This avoids an unrecoverable bootstrap cycle at the cost of maintaining a separate deployment-time configuration path.
