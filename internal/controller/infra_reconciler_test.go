package controller

import (
	"context"
	"testing"

	asoresourcesv1 "github.com/Azure/azure-service-operator/v2/api/resources/v1api20200601"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestInfraReconcilerReconcile(t *testing.T) {
	// TODO:
	t.Skip()

	ctx := context.Background()

	rg := &asoresourcesv1.ResourceGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "resource-group",
		},
	}

	s := runtime.NewScheme()
	sb := runtime.NewSchemeBuilder(
		asoresourcesv1.AddToScheme,
	)
	err := sb.AddToScheme(s)
	if err != nil {
		t.Fatal("failed to build scheme, got error", err)
	}

	r := &InfraReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(s).
			Build(),
		resources: []runtime.RawExtension{
			{
				Object: rg,
			},
		},
	}

	err = r.Get(ctx, client.ObjectKey{Namespace: "default", Name: "resource-group"}, rg)
	if !apierrors.IsNotFound(err) {
		t.Error("expected error to be not found, got", err)
	}

	err = r.Reconcile(context.Background())
	if err != nil {
		t.Error("expected success, got error", err)
	}

	err = r.Get(ctx, client.ObjectKey{Namespace: "default", Name: "resource-group"}, rg)
	if err != nil {
		t.Error("expected success, got error", err)
	}
}
