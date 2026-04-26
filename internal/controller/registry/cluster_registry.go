package registry

import (
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterClientRegistry 多集群客户端注册表。
//
// 用途：在"一份控制器管理多个目标集群"的场景下，按集群名缓存
// 已经从远程 kubeconfig 构建好的 controller-runtime client。
//
// 写入路径：ClusterConfigReconciler 监听 ClusterConfig CR，读取
// 引用的 Secret（kubeconfig），构建 client，调用 Set(name, c) 注册。
//
// 读取路径：DiagnosticRunReconciler 看到 run.Spec.ClusterRef 不为空时，
// 调用 Get(name) 拿到目标集群 client，Job/ConfigMap/SA 都创建到那个集群。
//
// 当 ClusterRef 为空（默认）时，使用本地 mgr.GetClient()，不走该注册表。
type ClusterClientRegistry struct {
	mu      sync.RWMutex
	clients map[string]client.Client
}

func NewClusterClientRegistry() *ClusterClientRegistry {
	return &ClusterClientRegistry{clients: make(map[string]client.Client)}
}

func (r *ClusterClientRegistry) Get(name string) (client.Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.clients[name]
	return c, ok
}

func (r *ClusterClientRegistry) Set(name string, c client.Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[name] = c
}

func (r *ClusterClientRegistry) Delete(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, name)
}
