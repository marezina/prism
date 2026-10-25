package gh

// Endpoints are the GitHub base URLs. This is the only place github.com and
// api.github.com may appear (github-api skill); GHES support later means
// passing different Endpoints.
type Endpoints struct {
	Web     string // https://github.com — OAuth device flow, SSO links
	API     string // REST base, with trailing slash
	GraphQL string // GraphQL endpoint
}

// DotCom are the github.com endpoints.
var DotCom = Endpoints{
	Web:     "https://github.com",
	API:     "https://api.github.com/",
	GraphQL: "https://api.github.com/graphql",
}

// DeviceCodeURL is where the OAuth device flow starts.
func (e Endpoints) DeviceCodeURL() string { return e.Web + "/login/device/code" }

// AccessTokenURL is polled during the OAuth device flow.
func (e Endpoints) AccessTokenURL() string { return e.Web + "/login/oauth/access_token" }

// OrgSSOURL is the page where a user starts a SAML session for org, used when
// GitHub did not hand us an authorization URL.
func (e Endpoints) OrgSSOURL(org string) string { return e.Web + "/orgs/" + org + "/sso" }
