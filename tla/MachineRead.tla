--------------------------- MODULE MachineRead ----------------------------
EXTENDS Integers, FiniteSets

CONSTANTS EnvironmentA,
          EnvironmentB,
          StrictToken,
          TokenOnlyToken,
          Certificate,
          Resource,
          NamespaceA,
          NamespaceB,
          ItemKey,
          RequestIDs,
          NoCertificate,
          BadCertificate,
          MissingResource,
          MaxRevision

Environments == {EnvironmentA, EnvironmentB}
Tokens == {StrictToken, TokenOnlyToken}
Certificates == {Certificate}
Resources == {Resource}
VaultIdentities == {<<NamespaceA, ItemKey>>, <<NamespaceB, ItemKey>>}
TokenOnlyTokens == {TokenOnlyToken}
InitialGrants == {<<StrictToken, EnvironmentA>>,
                   <<TokenOnlyToken, EnvironmentA>>,
                   <<TokenOnlyToken, EnvironmentB>>}

ASSUME /\ EnvironmentA # EnvironmentB
       /\ StrictToken # TokenOnlyToken
       /\ NamespaceA # NamespaceB
       /\ RequestIDs # {}
       /\ NoCertificate \notin Certificates
       /\ BadCertificate \notin Certificates
       /\ BadCertificate # NoCertificate
       /\ MissingResource \notin Resources
       /\ MaxRevision >= 1

PresentedCertificates == Certificates \cup {NoCertificate, BadCertificate}
PresentedResources == Resources \cup {MissingResource}
Statuses == {"unauthorized", "forbidden", "not_found",
             "unprocessable", "not_modified", "content"}

VARIABLES activeTokens,
          grants,
          activeCertificates,
          archivedEnvironments,
          resolvableResources,
          resolvableVaultIdentities,
          configRevision,
          vaultRevision,
          responses,
          accessEligible,
          accessPublished

vars == <<activeTokens, grants, activeCertificates, archivedEnvironments,
          resolvableResources, resolvableVaultIdentities,
          configRevision, vaultRevision, responses,
          accessEligible, accessPublished>>

UsedRequestIDs == {response.id : response \in responses}

TokenAccepted(token) == token \in activeTokens
EnvironmentAccepted(token, environment) == <<token, environment>> \in grants
CertificateAccepted(token, certificate) ==
    IF certificate = NoCertificate
    THEN token \in TokenOnlyTokens
    ELSE certificate \in activeCertificates
ResourceFound(environment, resource) ==
    environment \notin archivedEnvironments /\ resource \in Resources
ResourceResolved(resource, vaultIdentity) ==
    resource \in resolvableResources /\
    vaultIdentity \in resolvableVaultIdentities

Status(tokenOK, environmentOK, certificateOK, found, resolved, etagMatches) ==
    IF ~tokenOK THEN "unauthorized"
    ELSE IF ~environmentOK THEN "forbidden"
    ELSE IF ~certificateOK THEN "unauthorized"
    ELSE IF ~found THEN "not_found"
    ELSE IF ~resolved THEN "unprocessable"
    ELSE IF etagMatches THEN "not_modified"
    ELSE "content"

Response(request, token, environment, certificate, resource,
         vaultIdentity, etagMatches) ==
    [id |-> request,
     token |-> token,
     environment |-> environment,
     authorizationEnvironment |-> environment,
     certificate |-> certificate,
     resource |-> resource,
     vaultIdentity |-> vaultIdentity,
     tokenOK |-> TokenAccepted(token),
     environmentOK |-> EnvironmentAccepted(token, environment),
     certificateOK |-> CertificateAccepted(token, certificate),
     found |-> ResourceFound(environment, resource),
     resolved |-> ResourceResolved(resource, vaultIdentity),
     etagMatches |-> etagMatches,
     status |-> Status(TokenAccepted(token),
                       EnvironmentAccepted(token, environment),
                       CertificateAccepted(token, certificate),
                       ResourceFound(environment, resource),
                       ResourceResolved(resource, vaultIdentity),
                       etagMatches),
     configRevision |-> configRevision,
     vaultRevisions |-> vaultRevision,
     vaultRevision |-> vaultRevision[vaultIdentity]]

Init ==
    /\ activeTokens = Tokens
    /\ grants = InitialGrants
    /\ activeCertificates = Certificates
    /\ archivedEnvironments = {}
    /\ resolvableResources = Resources
    /\ resolvableVaultIdentities = VaultIdentities
    /\ configRevision = 1
    /\ vaultRevision = [identity \in VaultIdentities |-> 1]
    /\ responses = {}
    /\ accessEligible = {}
    /\ accessPublished = {}

Read ==
    \E request \in RequestIDs,
       token \in Tokens,
       environment \in Environments,
       certificate \in PresentedCertificates,
       resource \in PresentedResources,
       vaultIdentity \in VaultIdentities,
       etagMatches \in BOOLEAN:
        LET response == Response(request, token, environment, certificate,
                                 resource, vaultIdentity, etagMatches)
        IN /\ request \notin UsedRequestIDs
           /\ responses' = responses \cup {response}
           /\ accessEligible' =
                  IF response.status = "content"
                  THEN accessEligible \cup {request}
                  ELSE accessEligible
           /\ UNCHANGED <<activeTokens, grants, activeCertificates,
                           archivedEnvironments, resolvableResources,
                           resolvableVaultIdentities,
                           configRevision, vaultRevision, accessPublished>>

RevokeToken ==
    \E token \in activeTokens:
        /\ activeTokens' = activeTokens \ {token}
        /\ UNCHANGED <<grants, activeCertificates, archivedEnvironments,
                        resolvableResources, resolvableVaultIdentities,
                        configRevision, vaultRevision,
                        responses, accessEligible, accessPublished>>

RemoveGrant ==
    \E grant \in grants:
        /\ grants' = grants \ {grant}
        /\ UNCHANGED <<activeTokens, activeCertificates, archivedEnvironments,
                        resolvableResources, resolvableVaultIdentities,
                        configRevision, vaultRevision,
                        responses, accessEligible, accessPublished>>

RevokeCertificate ==
    \E certificate \in activeCertificates:
        /\ activeCertificates' = activeCertificates \ {certificate}
        /\ UNCHANGED <<activeTokens, grants, archivedEnvironments,
                        resolvableResources, resolvableVaultIdentities,
                        configRevision, vaultRevision,
                        responses, accessEligible, accessPublished>>

ArchiveEnvironment ==
    \E environment \in Environments \ archivedEnvironments:
        /\ archivedEnvironments' = archivedEnvironments \cup {environment}
        /\ UNCHANGED <<activeTokens, grants, activeCertificates,
                        resolvableResources, resolvableVaultIdentities,
                        configRevision, vaultRevision,
                        responses, accessEligible, accessPublished>>

BreakResolution ==
    \E resource \in resolvableResources:
        /\ resolvableResources' = resolvableResources \ {resource}
        /\ UNCHANGED <<activeTokens, grants, activeCertificates,
                        archivedEnvironments, resolvableVaultIdentities,
                        configRevision, vaultRevision,
                        responses, accessEligible, accessPublished>>

BreakVaultResolution ==
    \E identity \in resolvableVaultIdentities:
        /\ resolvableVaultIdentities' =
               resolvableVaultIdentities \ {identity}
        /\ UNCHANGED <<activeTokens, grants, activeCertificates,
                        archivedEnvironments, resolvableResources,
                        configRevision, vaultRevision, responses,
                        accessEligible, accessPublished>>

AdvanceConfig ==
    /\ configRevision < MaxRevision
    /\ configRevision' = configRevision + 1
    /\ UNCHANGED <<activeTokens, grants, activeCertificates,
                    archivedEnvironments, resolvableResources,
                    resolvableVaultIdentities, vaultRevision,
                    responses, accessEligible, accessPublished>>

AdvanceVault ==
    \E identity \in VaultIdentities:
        /\ vaultRevision[identity] < MaxRevision
        /\ vaultRevision' =
               [vaultRevision EXCEPT ![identity] = @ + 1]
        /\ UNCHANGED <<activeTokens, grants, activeCertificates,
                        archivedEnvironments, resolvableResources,
                        resolvableVaultIdentities, configRevision,
                        responses, accessEligible, accessPublished>>

PublishAccess ==
    \E request \in accessEligible \ accessPublished:
        /\ accessPublished' = accessPublished \cup {request}
        /\ UNCHANGED <<activeTokens, grants, activeCertificates,
                        archivedEnvironments, resolvableResources,
                        resolvableVaultIdentities,
                        configRevision, vaultRevision, responses,
                        accessEligible>>

Next == Read
     \/ RevokeToken
     \/ RemoveGrant
     \/ RevokeCertificate
     \/ ArchiveEnvironment
     \/ BreakResolution
     \/ BreakVaultResolution
     \/ AdvanceConfig
     \/ AdvanceVault
     \/ PublishAccess

TypeOK ==
    /\ activeTokens \subseteq Tokens
    /\ grants \subseteq (Tokens \X Environments)
    /\ activeCertificates \subseteq Certificates
    /\ archivedEnvironments \subseteq Environments
    /\ resolvableResources \subseteq Resources
    /\ resolvableVaultIdentities \subseteq VaultIdentities
    /\ configRevision \in 1..MaxRevision
    /\ vaultRevision \in [VaultIdentities -> 1..MaxRevision]
    /\ responses \subseteq
         [id : RequestIDs,
          token : Tokens,
          environment : Environments,
          authorizationEnvironment : Environments,
          certificate : PresentedCertificates,
          resource : PresentedResources,
          vaultIdentity : VaultIdentities,
          tokenOK : BOOLEAN,
          environmentOK : BOOLEAN,
          certificateOK : BOOLEAN,
          found : BOOLEAN,
          resolved : BOOLEAN,
          etagMatches : BOOLEAN,
          status : Statuses,
          configRevision : 1..MaxRevision,
          vaultRevisions : [VaultIdentities -> 1..MaxRevision],
          vaultRevision : 1..MaxRevision]
    /\ accessEligible \subseteq RequestIDs
    /\ accessPublished \subseteq RequestIDs

OneResponsePerRequest ==
    \A left \in responses, right \in responses:
        left.id = right.id => left = right

DecisionConsistent ==
    \A response \in responses:
        response.status =
            Status(response.tokenOK,
                   response.environmentOK,
                   response.certificateOK,
                   response.found,
                   response.resolved,
                   response.etagMatches)

AuthorizationScopeIsEnvironment ==
    \A response \in responses:
        response.authorizationEnvironment = response.environment

ExactVaultRevisionEvidence ==
    \A response \in responses:
        response.vaultRevision =
            response.vaultRevisions[response.vaultIdentity]

ContentRequiresCompleteAuthorization ==
    \A response \in responses:
        response.status = "content" =>
            /\ response.tokenOK
            /\ response.environmentOK
            /\ response.certificateOK
            /\ response.found
            /\ response.resolved
            /\ ~response.etagMatches

PresentedInvalidCertificateNeverDowngrades ==
    \A response \in responses:
        response.certificate = BadCertificate =>
            /\ ~response.certificateOK
            /\ response.status # "content"

ContentResponses ==
    {response \in responses : response.status = "content"}

AccessOnlyForReturnedContent ==
    /\ accessEligible = {response.id : response \in ContentResponses}
    /\ accessPublished \subseteq accessEligible

Spec == Init /\ [][Next]_vars

=============================================================================
