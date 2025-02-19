package nodepool

import (
	"context"

	infrastructurev1alpha3 "github.com/giantswarm/apiextensions/v3/pkg/apis/infrastructure/v1alpha3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	capzexpv1alpha3 "sigs.k8s.io/cluster-api-provider-azure/exp/api/v1alpha3"
	capiv1alpha3 "sigs.k8s.io/cluster-api/api/v1alpha3"
	capiexpv1alpha3 "sigs.k8s.io/cluster-api/exp/api/v1alpha3"
)

type GetOptions struct {
	Name        string
	ClusterName string
	Provider    string
	Namespace   string
}

type Interface interface {
	Get(context.Context, GetOptions) (Resource, error)
}

type Resource interface {
	Object() runtime.Object
}

// Nodepool abstracts away provider-specific
// node pool resources.
type Nodepool struct {
	MachineDeployment *capiv1alpha3.MachineDeployment
	MachinePool       *capiexpv1alpha3.MachinePool

	AWSMachineDeployment *infrastructurev1alpha3.AWSMachineDeployment
	AzureMachinePool     *capzexpv1alpha3.AzureMachinePool
}

func (n *Nodepool) Object() runtime.Object {
	if n.MachineDeployment != nil {
		return n.MachineDeployment
	} else if n.MachinePool != nil {
		return n.MachinePool
	}

	return nil
}

// Collection wraps a list of nodepools.
type Collection struct {
	Items []Nodepool
}

func (nc *Collection) Object() runtime.Object {
	list := &metav1.List{
		TypeMeta: metav1.TypeMeta{
			Kind:       "List",
			APIVersion: "v1",
		},
		ListMeta: metav1.ListMeta{},
	}

	for _, item := range nc.Items {
		obj := item.Object()
		if obj == nil {
			continue
		}

		raw := runtime.RawExtension{
			Object: obj,
		}
		list.Items = append(list.Items, raw)
	}

	return list
}
