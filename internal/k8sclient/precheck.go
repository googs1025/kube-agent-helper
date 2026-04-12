package k8sclient

import (
	"context"
	"fmt"

	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Precheck verifies the current identity has at least list access to Pods.
// Returns a descriptive error on denial or API failure.
func Precheck(ctx context.Context, client kubernetes.Interface) error {
	ssar := &authv1.SelfSubjectAccessReview{
		Spec: authv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authv1.ResourceAttributes{
				Verb:     "list",
				Resource: "pods",
			},
		},
	}

	result, err := client.AuthorizationV1().
		SelfSubjectAccessReviews().
		Create(ctx, ssar, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("precheck api call failed: %w", err)
	}
	if !result.Status.Allowed {
		return fmt.Errorf("cannot list pods: %s", result.Status.Reason)
	}
	return nil
}
