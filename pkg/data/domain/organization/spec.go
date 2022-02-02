// Package organization provides services to deal with Organizations
// in the Giant Swarm Management API.
package organization

import (
	"context"

	securityv1alpha1 "github.com/giantswarm/apiextensions/v3/pkg/apis/security/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Organization gives access to the actual Organization resource.
type Organization struct {
	*securityv1alpha1.Organization
}

type Collection struct {
	Items []Organization
}

type Interface interface {
	Get(context.Context, GetOptions) (Resource, error)
}

type GetOptions struct {
	Name string
}

type Resource interface {
	Object() runtime.Object
}

func (o *Organization) Object() runtime.Object {
	return o.Organization
}

func (c *Collection) Object() runtime.Object {
	list := &metav1.List{
		TypeMeta: metav1.TypeMeta{
			Kind:       "List",
			APIVersion: "v1",
		},
		ListMeta: metav1.ListMeta{},
	}

	for _, item := range c.Items {
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
