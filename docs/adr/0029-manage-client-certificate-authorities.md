# Manage client certificate authorities

Configra generates Client Certificate authorities and retains their signing keys encrypted with the existing deployment Master Key, bound to each authority's identity. Administrators receive private-key material only in the initial successful creation response; operation replay and stored audit/results contain metadata only, while public certificates remain downloadable. Issued client private keys are never retained by the service, and authority revocation invalidates its issued client certificates during request authorization, including existing TLS connections.

This complements externally managed client CAs without changing the independent server HTTPS certificate. Loss of the first export response requires revoking and replacing the affected credential; it does not create a private-key recovery endpoint.
