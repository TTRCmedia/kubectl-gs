package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/giantswarm/microerror"
	"github.com/giantswarm/micrologger"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/giantswarm/kubectl-gs/pkg/commonconfig"
	"github.com/giantswarm/kubectl-gs/pkg/data/domain/app"
	"github.com/giantswarm/kubectl-gs/pkg/output"
)

type runner struct {
	flag   *flag
	logger micrologger.Logger
	fs     afero.Fs

	service app.Interface

	stdout io.Writer
	stderr io.Writer
}

func (r *runner) Run(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	err := r.flag.Validate()
	if err != nil {
		return microerror.Mask(err)
	}

	err = r.run(ctx, cmd, args)
	if err != nil {
		return microerror.Mask(err)
	}

	return nil
}

func (r *runner) run(ctx context.Context, cmd *cobra.Command, args []string) error {
	var err error

	config := commonconfig.New(r.flag.config)
	{
		err = r.getService(config)
		if err != nil {
			return microerror.Mask(err)
		}
	}

	var namespace string
	{
		if r.flag.AllNamespaces {
			namespace = metav1.NamespaceAll
		} else {
			namespace, _, err = r.flag.config.ToRawKubeConfigLoader().Namespace()
			if err != nil {
				return microerror.Mask(err)
			}
		}
	}

	var name string
	{
		if len(args) > 0 {
			name = strings.ToLower(args[0])
		}
	}

	var appResource app.Resource
	{
		options := app.GetOptions{
			Namespace: namespace,
			Name:      name,
		}
		appResource, err = r.service.Get(ctx, options)
		if app.IsNotFound(err) {
			return microerror.Maskf(notFoundError, fmt.Sprintf("An app '%s/%s' cannot be found.\n", options.Namespace, options.Name))
		} else if app.IsNoMatch(err) {
			r.printNoMatchOutput()
			return nil
		} else if app.IsNoResources(err) && output.IsOutputDefault(r.flag.print.OutputFormat) {
			r.printNoResourcesOutput()
			return nil
		} else if err != nil {
			return microerror.Mask(err)
		}
	}

	err = r.printOutput(appResource)
	if err != nil {
		return microerror.Mask(err)
	}

	return nil
}

func (r *runner) getService(config *commonconfig.CommonConfig) error {
	if r.service != nil {
		return nil
	}

	client, err := config.GetClient(r.logger)
	if err != nil {
		return microerror.Mask(err)
	}

	serviceConfig := app.Config{
		Client: client.CtrlClient(),
	}
	r.service, err = app.New(serviceConfig)
	if err != nil {
		return microerror.Mask(err)
	}

	return nil
}
