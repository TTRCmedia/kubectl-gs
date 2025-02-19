package login

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
	"github.com/giantswarm/microerror"
	"github.com/giantswarm/micrologger"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/giantswarm/kubectl-gs/pkg/kubeconfig"
)

type runner struct {
	flag   *flag
	logger micrologger.Logger
	fs     afero.Fs

	k8sConfigAccess clientcmd.ConfigAccess
	loginOptions    LoginOptions

	stdout io.Writer
	stderr io.Writer
}

type LoginOptions struct {
	isMCLogin         bool
	isWCLogin         bool
	selfContainedMC   bool
	selfContainedWC   bool
	switchToMCcontext bool
	switchToWCcontext bool
	originContext     string
}

func (r *runner) Run(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	err := r.flag.Validate()
	if err != nil {
		return microerror.Mask(err)
	}

	r.setLoginOptions(ctx, args)
	err = r.run(ctx, cmd, args)
	if err != nil {
		return microerror.Mask(err)
	}

	return nil
}

func (r *runner) run(ctx context.Context, cmd *cobra.Command, args []string) error {
	var err error

	// Try to reuse the current context if no MC argument is given
	if !r.loginOptions.isMCLogin {
		return r.tryToReuseExistingContext(ctx)
	} else {
		// MC argument can be a kubernetes context name,
		// installation code name, or happa/k8s api URL.
		installationIdentifier := strings.ToLower(args[0])
		err = r.handleMCLogin(ctx, installationIdentifier)
		if err != nil {
			return microerror.Mask(err)
		}
	}
	// Login to WC after MC if desired
	if r.loginOptions.isWCLogin {
		return r.handleWCLogin(ctx)
	}

	return nil
}

func (r *runner) tryToGetCurrentContext(ctx context.Context) (string, error) {
	config, err := r.k8sConfigAccess.GetStartingConfig()
	if err != nil {
		return "", microerror.Mask(err)
	}
	return config.CurrentContext, nil
}

func (r *runner) setLoginOptions(ctx context.Context, args []string) {
	originContext, err := r.tryToGetCurrentContext(ctx)
	if err != nil {
		fmt.Fprintln(r.stdout, color.YellowString("Failed trying to determine current context. %s", err))
	}
	r.loginOptions = LoginOptions{
		originContext:   originContext,
		isWCLogin:       len(r.flag.WCName) > 0,
		isMCLogin:       !(len(args) < 1),
		selfContainedMC: len(r.flag.SelfContained) > 0 && !(len(r.flag.WCName) > 0),
		selfContainedWC: len(r.flag.SelfContained) > 0 && len(r.flag.WCName) > 0,
	}
	r.loginOptions.switchToMCcontext = r.loginOptions.isWCLogin || !(r.loginOptions.selfContainedMC || r.flag.KeepContext)
	r.loginOptions.switchToWCcontext = r.loginOptions.isWCLogin && !(r.loginOptions.selfContainedWC || r.flag.KeepContext)
}

func (r *runner) tryToReuseExistingContext(ctx context.Context) error {
	config, err := r.k8sConfigAccess.GetStartingConfig()
	if err != nil {
		return microerror.Mask(err)
	}

	currentContext := config.CurrentContext
	kubeContextType := kubeconfig.GetKubeContextType(currentContext)

	switch kubeContextType {
	case kubeconfig.ContextTypeMC:
		authType := kubeconfig.GetAuthType(config, currentContext)
		if authType == kubeconfig.AuthTypeAuthProvider {
			authProvider, exists := kubeconfig.GetAuthProvider(config, currentContext)
			if !exists {
				return microerror.Maskf(incorrectConfigurationError, "There is no authentication configuration for the '%s' context", currentContext)
			}

			err = validateOIDCProvider(authProvider)
			if IsNewLoginRequired(err) {
				issuer := authProvider.Config[Issuer]

				err = r.loginWithURL(ctx, issuer, false, "")
				if err != nil {
					return microerror.Mask(err)
				}

				return nil
			} else if err != nil {
				return microerror.Maskf(incorrectConfigurationError, "The authentication configuration is corrupted, please log in again using a URL.")
			}
		} else if authType == kubeconfig.AuthTypeUnknown {
			return microerror.Maskf(incorrectConfigurationError, "There is no authentication configuration for the '%s' context", currentContext)
		}

		codeName := kubeconfig.GetCodeNameFromKubeContext(currentContext)
		fmt.Fprint(r.stdout, color.GreenString("You are logged in to the management cluster of installation '%s'.\n", codeName))

		if r.loginOptions.isWCLogin {
			err = r.handleWCLogin(ctx)
			if err != nil {
				return microerror.Mask(err)
			}
		}

		return nil

	case kubeconfig.ContextTypeWC:
		codeName := kubeconfig.GetCodeNameFromKubeContext(currentContext)
		clusterName := kubeconfig.GetClusterNameFromKubeContext(currentContext)
		fmt.Fprint(r.stdout, color.GreenString("You are logged in to the workload cluster '%s' of installation '%s'.\n", clusterName, codeName))

		return nil

	default:
		if currentContext != "" {
			return microerror.Maskf(selectedContextNonCompatibleError, "The current context '%s' does not seem to belong to a Giant Swarm management cluster.\nPlease run 'kubectl gs login --help' to find out how to log in to a particular management cluster.", currentContext)
		}

		return microerror.Maskf(selectedContextNonCompatibleError, "The current context does not seem to belong to a Giant Swarm management cluster.\nPlease run 'kubectl gs login --help' to find out how to log in to a particular management cluster.")
	}
}
