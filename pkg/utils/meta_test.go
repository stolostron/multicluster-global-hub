package utils

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	mghName      = "mgh"
	mghNamespace = "default"
	ctx          = context.Background()
)

func TestDeleteAnnotation(t *testing.T) {
	tests := []struct {
		name    string
		obj     client.Object
		key     string
		wantErr bool
	}{
		{
			name: "obj with desired key",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mghName,
					Namespace: mghNamespace,
					Annotations: map[string]string{
						"a": "b",
					},
				},
			},
			key:     "a",
			wantErr: false,
		},
		{
			name: "obj do not have desired key",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mghName,
					Namespace: mghNamespace,
				},
			},
			key:     "a",
			wantErr: false,
		},
		{
			name: "do not have obj",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mghName + "no",
					Namespace: mghNamespace,
				},
			},
			key:     "a",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgh := &corev1.Secret{}
			fakeClient := fake.NewClientBuilder().WithObjects(tt.obj).WithScheme(scheme.Scheme).Build()
			if err := DeleteAnnotation(ctx,
				fakeClient,
				tt.obj,
				mghNamespace,
				mghName,
				tt.key); (err != nil) != tt.wantErr {
				t.Errorf("DeleteAnnotation() error = %v, wantErr %v", err, tt.wantErr)
			}

			err := fakeClient.Get(ctx, types.NamespacedName{
				Namespace: mghNamespace,
				Name:      mghName,
			}, mgh)
			if err != nil {
				if errors.IsNotFound(err) {
					return
				}
				t.Errorf("Failed to get mgh, err: %v", err)
			}
			if HasItemKey(mgh.Annotations, tt.key) {
				t.Errorf("Failed to delete annotation")
			}
		})
	}
}
