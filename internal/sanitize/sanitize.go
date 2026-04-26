// Package sanitize 在 K8s 资源对象返回给 LLM 前做敏感字段脱敏。
//
// 用途：MCP 工具（kubectl_get / describe）返回 Pod / Secret / ConfigMap 时，
// 用 Clean(obj, opts) 去掉以下内容防止泄漏给 LLM 或日志：
//   - Secret.data 全部置空
//   - ConfigMap.data 中匹配 ConfigMapKeyMask 的 key 置空
//   - Pod env / volumeMounts / serviceAccountToken 中的密文字段
//   - managedFields / lastAppliedConfiguration 等噪声字段
//
// 设计要点：
//   - 输入对象不修改（DeepCopy 后操作）
//   - 幂等：Clean(Clean(x)) == Clean(x)
//   - 配合 trimmer 一起用，先 trim 再 sanitize
package sanitize

import (
	"regexp"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Options configures the sanitizer.
type Options struct {
	// ConfigMapKeyMask matches ConfigMap data keys to redact. If nil, no
	// ConfigMap redaction is performed beyond the generic rules.
	ConfigMapKeyMask *regexp.Regexp
}

// Clean returns a deep-copied, sanitized copy of obj. The original is never
// mutated. The function is idempotent: Clean(Clean(x)) == Clean(x).
func Clean(obj *unstructured.Unstructured, opts Options) *unstructured.Unstructured {
	if obj == nil {
		return nil
	}
	out := obj.DeepCopy()

	stripGenericMetadata(out)

	switch out.GetKind() {
	case "Secret":
		cleanSecret(out)
	case "ConfigMap":
		cleanConfigMap(out, opts.ConfigMapKeyMask)
	case "Pod":
		cleanPod(out)
	case "PodList":
		// handled by trimmer layer; nothing to do here
	}

	return out
}

// stripGenericMetadata removes fields that every resource should lose,
// regardless of Kind: managedFields, selfLink, and the last-applied
// annotation.
func stripGenericMetadata(u *unstructured.Unstructured) {
	meta, ok := u.Object["metadata"].(map[string]interface{})
	if !ok {
		return
	}
	delete(meta, "managedFields")
	delete(meta, "selfLink")

	if annos, ok := meta["annotations"].(map[string]interface{}); ok {
		delete(annos, "kubectl.kubernetes.io/last-applied-configuration")
		if len(annos) == 0 {
			delete(meta, "annotations")
		}
	}
}
