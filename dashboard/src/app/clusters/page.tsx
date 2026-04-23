"use client";

import { useState } from "react";
import { useI18n } from "@/i18n/context";
import { useClusterConfigs, createClusterConfig, useK8sNamespaces } from "@/lib/api";
import type { ClusterItem } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

const SA_SCRIPT = `# On the REMOTE cluster:
kubectl create sa kah-reader -n kube-system
kubectl create clusterrolebinding kah-reader \\
  --clusterrole=view --serviceaccount=kube-system:kah-reader

TOKEN=$(kubectl create token kah-reader -n kube-system --duration=8760h)
CA=$(kubectl config view --raw -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')
SERVER=$(kubectl config view --raw -o jsonpath='{.clusters[0].cluster.server}')

cat > /tmp/remote-kubeconfig.yaml <<EOF
apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: \${CA}
    server: \${SERVER}
  name: remote
contexts:
- context: {cluster: remote, user: kah-reader}
  name: remote
current-context: remote
users:
- name: kah-reader
  user: {token: \${TOKEN}}
EOF

# Back on the LOCAL cluster:
kubectl create secret generic remote-kubeconfig \\
  -n kube-agent-helper \\
  --from-file=kubeconfig=/tmp/remote-kubeconfig.yaml`;

const phaseColors: Record<string, string> = {
  Connected: "bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-300",
  Error: "bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300",
  Pending: "bg-yellow-100 text-yellow-800 dark:bg-yellow-950 dark:text-yellow-300",
};

function CreateDialog({ onClose }: { onClose: () => void }) {
  const { t } = useI18n();
  const { data: namespaces } = useK8sNamespaces();
  const [form, setForm] = useState({
    name: "",
    namespace: "kube-agent-helper",
    secretName: "",
    secretKey: "kubeconfig",
    prometheusURL: "",
    description: "",
  });
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      await createClusterConfig({
        ...form,
        prometheusURL: form.prometheusURL || undefined,
        description: form.description || undefined,
      });
      onClose();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed");
    } finally {
      setSubmitting(false);
    }
  };

  const inputClass =
    "w-full rounded border border-gray-300 bg-white px-3 py-1.5 text-sm dark:bg-gray-800 dark:border-gray-600 dark:text-gray-100";

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-lg rounded-lg bg-white p-6 shadow-xl dark:bg-gray-900"
      >
        <h2 className="mb-4 text-lg font-semibold">{t("clusters.create.title")}</h2>
        {error && <p className="mb-3 text-sm text-red-500">{error}</p>}

        <div className="grid grid-cols-2 gap-3 mb-4">
          <div>
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("clusters.create.name")}
            </label>
            <input
              className={inputClass}
              required
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </div>
          <div>
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("clusters.create.namespace")}
            </label>
            <select
              className={inputClass}
              value={form.namespace}
              onChange={(e) => setForm({ ...form, namespace: e.target.value })}
            >
              {(namespaces || []).map((ns) => (
                <option key={ns.name} value={ns.name}>
                  {ns.name}
                </option>
              ))}
              {(!namespaces || namespaces.length === 0) && (
                <option value="kube-agent-helper">kube-agent-helper</option>
              )}
            </select>
          </div>
          <div>
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("clusters.create.secretName")}
            </label>
            <input
              className={inputClass}
              required
              value={form.secretName}
              onChange={(e) => setForm({ ...form, secretName: e.target.value })}
            />
          </div>
          <div>
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("clusters.create.secretKey")}
            </label>
            <input
              className={inputClass}
              required
              value={form.secretKey}
              onChange={(e) => setForm({ ...form, secretKey: e.target.value })}
            />
          </div>
          <div className="col-span-2">
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("clusters.create.prometheus")}
            </label>
            <input
              className={inputClass}
              placeholder="http://prometheus:9090"
              value={form.prometheusURL}
              onChange={(e) => setForm({ ...form, prometheusURL: e.target.value })}
            />
          </div>
          <div className="col-span-2">
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("clusters.create.description")}
            </label>
            <input
              className={inputClass}
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
            />
          </div>
        </div>

        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded px-4 py-1.5 text-sm text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
          >
            {t("clusters.create.cancel")}
          </button>
          <button
            type="submit"
            disabled={submitting || !form.name || !form.secretName || !form.secretKey}
            className="rounded bg-blue-600 px-4 py-1.5 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {t("clusters.create.submit")}
          </button>
        </div>
      </form>
    </div>
  );
}

export default function ClustersPage() {
  const { t } = useI18n();
  const { data: clusters, isLoading, mutate } = useClusterConfigs();
  const [showCreate, setShowCreate] = useState(false);

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t("clusters.title")}</h1>
        <button
          onClick={() => setShowCreate(true)}
          className="rounded bg-blue-600 px-4 py-1.5 text-sm text-white hover:bg-blue-700"
        >
          + {t("clusters.create.title")}
        </button>
      </div>

      {isLoading && <p className="text-gray-500">{t("clusters.loading")}</p>}

      {!isLoading && (!clusters || clusters.length === 0) && (
        <p className="text-gray-500 dark:text-gray-400">{t("clusters.empty")}</p>
      )}

      {clusters && clusters.length > 0 && (
        <div className="mb-8 overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 text-left text-xs font-medium text-gray-500 dark:bg-gray-800 dark:text-gray-400">
              <tr>
                <th className="px-4 py-3">{t("clusters.col.name")}</th>
                <th className="px-4 py-3">{t("clusters.col.phase")}</th>
                <th className="px-4 py-3">{t("clusters.col.prometheus")}</th>
                <th className="px-4 py-3">{t("clusters.col.description")}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              {clusters.map((c: ClusterItem) => (
                <tr key={c.name} className="hover:bg-gray-50 dark:hover:bg-gray-800/50">
                  <td className="px-4 py-3 font-medium">{c.name}</td>
                  <td className="px-4 py-3">
                    <Badge className={phaseColors[c.phase] || "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300"}>
                      {c.phase}
                    </Badge>
                  </td>
                  <td className="px-4 py-3 text-xs font-mono text-gray-500 dark:text-gray-400">
                    {c.prometheusURL || <span className="italic text-gray-400">-</span>}
                  </td>
                  <td className="px-4 py-3 text-gray-500 dark:text-gray-400">
                    {c.description || "-"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Setup guide */}
      <Card className="mb-6">
        <CardHeader>
          <CardTitle className="text-base">{t("clusters.setup.title")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-6 text-sm">
          {/* Step 1 */}
          <div>
            <p className="font-semibold text-gray-800 dark:text-gray-200">{t("clusters.setup.step1")}</p>
            <p className="mt-1 text-gray-600 dark:text-gray-400">{t("clusters.setup.step1.desc")}</p>
          </div>

          {/* Step 2 */}
          <div>
            <p className="font-semibold text-gray-800 dark:text-gray-200">{t("clusters.setup.step2")}</p>
            <pre className="mt-2 rounded bg-gray-100 p-3 text-xs font-mono text-gray-800 dark:bg-gray-800 dark:text-gray-200 overflow-x-auto">
              {t("clusters.setup.step2.cmd")}
            </pre>
          </div>

          {/* Step 3 */}
          <div>
            <p className="font-semibold text-gray-800 dark:text-gray-200">{t("clusters.setup.step3")}</p>
            <p className="mt-1 text-gray-600 dark:text-gray-400">{t("clusters.setup.step3.desc")}</p>
          </div>

          {/* SA Token recommendation */}
          <div className="rounded-lg border border-blue-200 bg-blue-50 p-4 dark:border-blue-800 dark:bg-blue-950/30">
            <p className="font-semibold text-blue-800 dark:text-blue-300">{t("clusters.setup.sa.title")}</p>
            <p className="mt-1 text-blue-700 dark:text-blue-400">{t("clusters.setup.sa.desc")}</p>
            <pre className="mt-3 rounded bg-white p-3 text-xs font-mono text-gray-800 dark:bg-gray-900 dark:text-gray-200 overflow-x-auto whitespace-pre">
              {SA_SCRIPT}
            </pre>
          </div>
        </CardContent>
      </Card>

      {showCreate && (
        <CreateDialog
          onClose={() => {
            setShowCreate(false);
            mutate();
          }}
        />
      )}
    </div>
  );
}
