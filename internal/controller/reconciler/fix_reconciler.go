package reconciler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/metrics"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type DiagnosticFixReconciler struct {
	client.Client
	Store   store.Store
	Metrics *metrics.Metrics // nil-safe
}

func (r *DiagnosticFixReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var fix k8saiV1.DiagnosticFix
	if err := r.Get(ctx, req.NamespacedName, &fix); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	switch fix.Status.Phase {
	case "":
		if fix.Spec.Strategy == "dry-run" {
			fix.Status.Phase = "DryRunComplete"
			fix.Status.Message = "Dry-run: patch content available for review"
		} else if fix.Spec.ApprovalRequired {
			fix.Status.Phase = "PendingApproval"
			fix.Status.Message = "Waiting for human approval"
		} else {
			fix.Status.Phase = "Approved"
			fix.Status.Message = "Auto-approved (approvalRequired=false)"
		}
		if err := r.Status().Update(ctx, &fix); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("fix initialized", "name", fix.Name, "phase", fix.Status.Phase)
		return ctrl.Result{Requeue: true}, nil

	case "Approved":
		fix.Status.Phase = "Applying"
		now := metav1.Now()
		fix.Status.AppliedAt = &now
		if err := r.Status().Update(ctx, &fix); err != nil {
			return ctrl.Result{}, err
		}

		if fix.Spec.Strategy == "create" {
			if err := r.createResource(ctx, &fix); err != nil {
				return r.failFix(ctx, &fix, fmt.Sprintf("create resource failed: %s", err))
			}
		} else {
			if err := r.applyPatch(ctx, &fix); err != nil {
				return r.failFix(ctx, &fix, fmt.Sprintf("apply patch failed: %s", err))
			}
		}

		msg := "Patch applied successfully"
		if fix.Spec.Strategy == "create" {
			msg = "Resource created successfully"
		}
		fix.Status.Phase = "Succeeded"
		fix.Status.Message = msg
		completedNow := metav1.Now()
		fix.Status.CompletedAt = &completedNow
		if err := r.Status().Update(ctx, &fix); err != nil {
			return ctrl.Result{}, err
		}
		_ = r.Store.UpdateFixPhase(ctx, string(fix.UID), store.FixPhaseSucceeded, msg)
		if r.Metrics != nil {
			r.Metrics.RecordFixCompleted("Succeeded", fix.Spec.Target.Namespace, "")
		}
		logger.Info("fix applied", "name", fix.Name, "strategy", fix.Spec.Strategy)
		return ctrl.Result{}, nil

	case "PendingApproval", "DryRunComplete", "Succeeded", "Failed", "RolledBack":
		return ctrl.Result{}, nil

	case "Applying":
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r *DiagnosticFixReconciler) applyPatch(ctx context.Context, fix *k8saiV1.DiagnosticFix) error {
	gvk := kindToGVK(fix.Spec.Target.Kind)
	if gvk.Kind == "" {
		return fmt.Errorf("unsupported target kind: %s", fix.Spec.Target.Kind)
	}

	// Snapshot for rollback
	if fix.Spec.Rollback.SnapshotBefore {
		current := &unstructured.Unstructured{}
		current.SetGroupVersionKind(gvk)
		key := types.NamespacedName{Name: fix.Spec.Target.Name, Namespace: fix.Spec.Target.Namespace}
		if err := r.Get(ctx, key, current); err != nil {
			return fmt.Errorf("get target for snapshot: %w", err)
		}
		data, _ := json.Marshal(current.Object)
		fix.Status.RollbackSnapshot = base64.StdEncoding.EncodeToString(data)
		_ = r.Store.UpdateFixSnapshot(ctx, string(fix.UID), fix.Status.RollbackSnapshot)
	}

	// Apply patch
	target := &unstructured.Unstructured{}
	target.SetGroupVersionKind(gvk)
	target.SetName(fix.Spec.Target.Name)
	target.SetNamespace(fix.Spec.Target.Namespace)

	var patchType types.PatchType
	if fix.Spec.Patch.Type == "json-patch" {
		patchType = types.JSONPatchType
	} else {
		patchType = types.StrategicMergePatchType
	}

	patch := client.RawPatch(patchType, []byte(fix.Spec.Patch.Content))
	if err := r.Patch(ctx, target, patch); err != nil {
		if fix.Spec.Rollback.AutoRollbackOnFailure && fix.Status.RollbackSnapshot != "" {
			_ = r.rollback(ctx, fix)
		}
		return err
	}
	return nil
}

func (r *DiagnosticFixReconciler) rollback(ctx context.Context, fix *k8saiV1.DiagnosticFix) error {
	logger := log.FromContext(ctx)
	data, err := base64.StdEncoding.DecodeString(fix.Status.RollbackSnapshot)
	if err != nil {
		return fmt.Errorf("decode rollback snapshot: %w", err)
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("unmarshal rollback snapshot: %w", err)
	}
	target := &unstructured.Unstructured{Object: obj}
	patch := client.RawPatch(types.MergePatchType, data)
	if err := r.Patch(ctx, target, patch); err != nil {
		logger.Error(err, "rollback failed", "fix", fix.Name)
		return err
	}
	fix.Status.Phase = "RolledBack"
	fix.Status.Message = "Auto-rolled back after apply failure"
	now := metav1.Now()
	fix.Status.CompletedAt = &now
	_ = r.Status().Update(ctx, fix)
	_ = r.Store.UpdateFixPhase(ctx, string(fix.UID), store.FixPhaseRolledBack, "auto-rollback")
	if r.Metrics != nil {
		r.Metrics.RecordFixCompleted("RolledBack", fix.Spec.Target.Namespace, "")
	}
	logger.Info("fix rolled back", "name", fix.Name)
	return nil
}

// createResource handles strategy=create: parses patch.content as a full JSON
// resource manifest and creates it in the cluster.
func (r *DiagnosticFixReconciler) createResource(ctx context.Context, fix *k8saiV1.DiagnosticFix) error {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(fix.Spec.Patch.Content), &obj); err != nil {
		return fmt.Errorf("parse resource JSON: %w", err)
	}
	u := &unstructured.Unstructured{Object: obj}
	// Ensure namespace matches the fix target
	if u.GetNamespace() == "" {
		u.SetNamespace(fix.Spec.Target.Namespace)
	}
	if err := r.Create(ctx, u); err != nil {
		return fmt.Errorf("create %s/%s: %w", u.GetKind(), u.GetName(), err)
	}
	return nil
}

func (r *DiagnosticFixReconciler) failFix(ctx context.Context, fix *k8saiV1.DiagnosticFix, msg string) (ctrl.Result, error) {
	fix.Status.Phase = "Failed"
	fix.Status.Message = msg
	now := metav1.Now()
	fix.Status.CompletedAt = &now
	if err := r.Status().Update(ctx, fix); err != nil {
		return ctrl.Result{}, err
	}
	_ = r.Store.UpdateFixPhase(ctx, string(fix.UID), store.FixPhaseFailed, msg)
	if r.Metrics != nil {
		r.Metrics.RecordFixCompleted("Failed", fix.Spec.Target.Namespace, "")
	}
	return ctrl.Result{}, nil
}

func (r *DiagnosticFixReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8saiV1.DiagnosticFix{}).
		Complete(r)
}

func kindToGVK(kind string) schema.GroupVersionKind {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet":
		return schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: kind}
	case "Pod", "Service", "ConfigMap", "Secret", "ServiceAccount", "Namespace", "PersistentVolumeClaim":
		return schema.GroupVersionKind{Group: "", Version: "v1", Kind: kind}
	case "Job", "CronJob":
		return schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: kind}
	case "Ingress", "NetworkPolicy":
		return schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: kind}
	case "PodDisruptionBudget":
		return schema.GroupVersionKind{Group: "policy", Version: "v1", Kind: kind}
	case "HorizontalPodAutoscaler":
		return schema.GroupVersionKind{Group: "autoscaling", Version: "v2", Kind: kind}
	case "ClusterRole", "ClusterRoleBinding", "Role", "RoleBinding":
		return schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: kind}
	case "ResourceQuota", "LimitRange":
		return schema.GroupVersionKind{Group: "", Version: "v1", Kind: kind}
	default:
		return schema.GroupVersionKind{}
	}
}
