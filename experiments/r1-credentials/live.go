package credentials

// LiveAuthorization is the explicit live GitHub App/API capability. This slice
// does not implement an authorized transport; no field enables network access,
// JWT minting, private-key reading or installation-token exchange.
type LiveAuthorization struct{}

// NewLiveGitHubAPI is the production GitHub App identity adapter constructor.
// The authorized path, once separately reviewed and maintainer-dispatched, is
// GET /app, GET /orgs/{org}/installation, then repository corroboration using
// an installation token that must never enter worker argv, environment or files.
// This constructor fails closed: it does not pin an SDK, open sockets, mint
// tokens or return an API value.
func NewLiveGitHubAPI(LiveAuthorization) (API, error) {
	return nil, ErrLiveUnauthorized
}
