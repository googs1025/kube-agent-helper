package k8sclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestPrecheck_Allowed(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &authv1.SelfSubjectAccessReview{
				ObjectMeta: metav1.ObjectMeta{},
				Status:     authv1.SubjectAccessReviewStatus{Allowed: true},
			}, nil
		},
	)

	err := Precheck(context.Background(), client)
	require.NoError(t, err)
}

func TestPrecheck_Denied(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &authv1.SelfSubjectAccessReview{
				Status: authv1.SubjectAccessReviewStatus{
					Allowed: false,
					Reason:  "RBAC: not allowed",
				},
			}, nil
		},
	)

	err := Precheck(context.Background(), client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot list pods")
}
