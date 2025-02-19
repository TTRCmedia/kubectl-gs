package login

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/fatih/color"
	"github.com/giantswarm/microerror"
	"github.com/skratchdot/open-golang/open"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/giantswarm/kubectl-gs/cmd/login/template"
	"github.com/giantswarm/kubectl-gs/pkg/callbackserver"
	"github.com/giantswarm/kubectl-gs/pkg/installation"
	"github.com/giantswarm/kubectl-gs/pkg/oidc"
)

const (
	clientID = "zQiFLUnrTFQwrybYzeY53hWWfhOKWRAU"

	oidcCallbackURL  = "http://localhost"
	oidcCallbackPath = "/oauth/callback"

	customerConnectorID   = "customer"
	giantswarmConnectorID = "giantswarm"

	oidcResultTimeout = 1 * time.Minute
)

var (
	oidcScopes = [...]string{gooidc.ScopeOpenID, "profile", "email", "groups", "offline_access", "audience:server:client_id:dex-k8s-authenticator"}
)

// handleOIDC executes the OIDC authentication against an installation's authentication provider.
func handleOIDC(ctx context.Context, out io.Writer, errOut io.Writer, i *installation.Installation, clusterAdmin bool, port int) (authInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, oidcResultTimeout)
	defer cancel()

	var err error
	var authProxy *callbackserver.CallbackServer
	{
		config := callbackserver.Config{
			Port:        port,
			RedirectURI: oidcCallbackPath,
		}
		authProxy, err = callbackserver.New(config)
		if err != nil {
			return authInfo{}, microerror.Mask(err)
		}
	}

	oidcConfig := oidc.Config{
		ClientID:    clientID,
		Issuer:      i.AuthURL,
		RedirectURL: fmt.Sprintf("%s:%d%s", oidcCallbackURL, authProxy.Port(), oidcCallbackPath),
		AuthScopes:  oidcScopes[:],
	}
	auther, err := oidc.New(ctx, oidcConfig)
	if err != nil {
		return authInfo{}, microerror.Mask(err)
	}

	// select dex connector_id based on clusterAdmin value
	var connectorID string
	{
		if clusterAdmin {
			connectorID = giantswarmConnectorID
		} else {
			connectorID = customerConnectorID
		}
	}

	authURL := auther.GetAuthURL(connectorID)

	fmt.Fprintf(out, "\n%s\n", color.YellowString("Your browser should now be opening this URL:"))
	fmt.Fprintf(out, "%s\n\n", authURL)

	// Open the authorization url in the user's browser, which will eventually
	// redirect the user to the local web server we'll create next.
	err = open.Run(authURL)
	if err != nil {
		fmt.Fprintf(errOut, "%s\n\n", color.YellowString("Couldn't open the default browser. Please access the URL above to continue logging in."))
	}

	// Create a local web server, for fetching all the authentication data from
	// the authentication provider.
	p, err := authProxy.Run(ctx, handleOIDCCallback(ctx, auther))
	if callbackserver.IsTimedOut(err) {
		return authInfo{}, microerror.Maskf(authResponseTimedOutError, "failed to get an authentication response on time")
	} else if err != nil {
		return authInfo{}, microerror.Mask(err)
	}

	var authResult authInfo
	{
		user, ok := p.(oidc.UserInfo)
		if !ok {
			return authInfo{}, microerror.Mask(invalidAuthResult)
		}

		authResult.username = user.Username
		authResult.token = user.IDToken
		authResult.refreshToken = user.RefreshToken
		authResult.clientID = clientID
	}

	return authResult, nil
}

// handleOIDCCallback is the callback executed after the authentication response was
// received from the authentication provider.
func handleOIDCCallback(ctx context.Context, a *oidc.Authenticator) func(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	return func(w http.ResponseWriter, r *http.Request) (interface{}, error) {
		res, err := a.HandleIssuerResponse(ctx, r.URL.Query().Get("state"), r.URL.Query().Get("code"))
		if err != nil {
			failureTemplate, tErr := template.GetFailedHTMLTemplateReader()
			if tErr != nil {
				return oidc.UserInfo{}, microerror.Mask(tErr)
			}

			w.Header().Set("Content-Type", "text/html")
			http.ServeContent(w, r, "", time.Time{}, failureTemplate)

			return oidc.UserInfo{}, microerror.Mask(err)
		}

		successTemplate, err := template.GetSuccessHTMLTemplateReader()
		if err != nil {
			return oidc.UserInfo{}, microerror.Mask(err)
		}

		w.Header().Set("Content-Type", "text/html")
		http.ServeContent(w, r, "", time.Time{}, successTemplate)

		return res, nil
	}
}

func validateOIDCProvider(provider *clientcmdapi.AuthProviderConfig) error {
	if len(provider.Config[ClientID]) < 1 || len(provider.Config[Issuer]) < 1 {
		return microerror.Mask(invalidAuthConfigurationError)
	}

	if len(provider.Config[IDToken]) < 1 || len(provider.Config[RefreshToken]) < 1 {
		return microerror.Mask(newLoginRequiredError)
	}

	_, err := url.ParseRequestURI(provider.Config[Issuer])
	if err != nil {
		return microerror.Mask(invalidAuthConfigurationError)
	}

	return nil
}
