import { useEffect, useState } from 'react';
import {
  Activity,
  AlertCircle,
  CheckCircle2,
  Database,
  Gauge,
  Loader2,
  RefreshCw,
  Save,
  Search,
  Shield,
  Trash2,
} from 'lucide-react';
import {
  type DiagnosticsResponse,
  type OwnerQuota,
  type OwnerUsage,
  type QuotaSummary,
  deleteOwnerQuota,
  getDiagnostics,
  getOwnerUsage,
  getQuotaSummary,
  listOwnerQuotas,
  saveOwnerQuota,
} from '../api/client';
import { useToast } from '../hooks/useToast';

type OperationsTab = 'quotas' | 'diagnostics';

interface QuotaForm {
  owner_id: string;
  max_sandboxes: string;
  max_ttl: string;
  max_exec_timeout: string;
}

const EMPTY_FORM: QuotaForm = {
  owner_id: '',
  max_sandboxes: '0',
  max_ttl: '',
  max_exec_timeout: '',
};

function formatUnknown(value: unknown): string {
  if (value === null || value === undefined || value === '') return 'none';
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

function toQuotaForm(quota: OwnerQuota): QuotaForm {
  return {
    owner_id: quota.owner_id,
    max_sandboxes: String(quota.max_sandboxes || 0),
    max_ttl: quota.max_ttl || '',
    max_exec_timeout: quota.max_exec_timeout || '',
  };
}

export default function Operations() {
  const [activeTab, setActiveTab] = useState<OperationsTab>('quotas');

  return (
    <div className="space-y-6 max-w-7xl">
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-display font-bold text-gray-100">Operations</h2>
          <p className="text-sm text-gray-400 mt-1">
            Admin quota controls and redacted platform diagnostics
          </p>
        </div>
        <div className="flex items-center gap-1 rounded-lg bg-navy-800 border border-navy-700 p-1">
          <TabButton
            label="Quotas"
            active={activeTab === 'quotas'}
            onClick={() => setActiveTab('quotas')}
          />
          <TabButton
            label="Diagnostics"
            active={activeTab === 'diagnostics'}
            onClick={() => setActiveTab('diagnostics')}
          />
        </div>
      </div>

      {activeTab === 'quotas' ? <QuotaPanel /> : <DiagnosticsPanel />}
    </div>
  );
}

function TabButton({
  label,
  active,
  onClick,
}: {
  label: string;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      onClick={onClick}
      className={`px-3 py-1.5 rounded-md text-sm transition-colors ${
        active
          ? 'bg-primary-500/15 text-primary-400'
          : 'text-gray-400 hover:bg-navy-700 hover:text-gray-200'
      }`}
    >
      {label}
    </button>
  );
}

function QuotaPanel() {
  const { addToast } = useToast();
  const [quotas, setQuotas] = useState<OwnerQuota[]>([]);
  const [summary, setSummary] = useState<QuotaSummary | null>(null);
  const [usage, setUsage] = useState<OwnerUsage | null>(null);
  const [form, setForm] = useState<QuotaForm>(EMPTY_FORM);
  const [usageOwner, setUsageOwner] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [checkingUsage, setCheckingUsage] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = async () => {
    setLoading(true);
    try {
      const [quotaList, quotaSummary] = await Promise.all([
        listOwnerQuotas(),
        getQuotaSummary(),
      ]);
      setQuotas(quotaList);
      setSummary(quotaSummary);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load quotas');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    refresh();
  }, []);

  const submitQuota = async () => {
    const ownerId = form.owner_id.trim();
    if (!ownerId) {
      addToast({ type: 'warning', title: 'Owner ID is required' });
      return;
    }

    setSaving(true);
    try {
      await saveOwnerQuota({
        owner_id: ownerId,
        max_sandboxes: Math.max(0, Number(form.max_sandboxes) || 0),
        max_ttl: form.max_ttl.trim(),
        max_exec_timeout: form.max_exec_timeout.trim(),
      });
      addToast({ type: 'success', title: 'Quota saved', message: ownerId });
      setForm(EMPTY_FORM);
      await refresh();
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Quota save failed',
        message: err instanceof Error ? err.message : 'Unknown error',
      });
    } finally {
      setSaving(false);
    }
  };

  const removeQuota = async (ownerId: string) => {
    try {
      await deleteOwnerQuota(ownerId);
      addToast({ type: 'success', title: 'Quota deleted', message: ownerId });
      await refresh();
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Delete failed',
        message: err instanceof Error ? err.message : 'Unknown error',
      });
    }
  };

  const checkUsage = async (ownerId = usageOwner.trim()) => {
    if (!ownerId) {
      addToast({ type: 'warning', title: 'Owner ID is required' });
      return;
    }

    setCheckingUsage(true);
    try {
      const result = await getOwnerUsage(ownerId);
      setUsage(result);
      setUsageOwner(ownerId);
    } catch (err) {
      addToast({
        type: 'error',
        title: 'Usage check failed',
        message: err instanceof Error ? err.message : 'Unknown error',
      });
    } finally {
      setCheckingUsage(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 sm:grid-cols-4 gap-4">
        <SummaryCard label="Policies" value={summary?.total ?? 0} icon={Shield} />
        <SummaryCard
          label="Sandbox Caps"
          value={summary?.with_max_sandboxes ?? 0}
          icon={Gauge}
        />
        <SummaryCard label="TTL Caps" value={summary?.with_max_ttl ?? 0} icon={Activity} />
        <SummaryCard
          label="Exec Caps"
          value={summary?.with_max_exec_timeout ?? 0}
          icon={Database}
        />
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-[minmax(0,1fr)_360px] gap-6">
        <div className="card !p-0 overflow-hidden">
          <div className="flex items-center justify-between px-5 py-4 border-b border-navy-700">
            <div>
              <h3 className="text-base font-bold text-gray-100">Owner Quotas</h3>
              <p className="text-xs text-gray-500 mt-0.5">
                Persisted quota overrides for tenant owners
              </p>
            </div>
            <button onClick={refresh} disabled={loading} className="btn-ghost text-sm">
              <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
              Refresh
            </button>
          </div>

          {loading ? (
            <div className="flex items-center justify-center py-16 text-gray-500">
              <Loader2 className="w-5 h-5 animate-spin" />
            </div>
          ) : error ? (
            <div className="flex flex-col items-center justify-center py-16 text-gray-500">
              <AlertCircle className="w-8 h-8 mb-3 opacity-60" />
              <p className="text-sm font-medium">{error}</p>
            </div>
          ) : quotas.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-gray-500">
              <Shield className="w-8 h-8 mb-3 opacity-40" />
              <p className="text-sm font-medium">No owner quota overrides</p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="bg-navy-800 text-xs text-gray-500">
                  <tr>
                    <th className="text-left font-medium px-5 py-3">Owner</th>
                    <th className="text-left font-medium px-5 py-3">Sandboxes</th>
                    <th className="text-left font-medium px-5 py-3">TTL</th>
                    <th className="text-left font-medium px-5 py-3">Exec Timeout</th>
                    <th className="text-right font-medium px-5 py-3">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-navy-700">
                  {quotas.map((quota) => (
                    <tr key={quota.owner_id} className="hover:bg-navy-800/50">
                      <td className="px-5 py-3 font-mono text-xs text-gray-300">
                        {quota.owner_id}
                      </td>
                      <td className="px-5 py-3 text-gray-300">{quota.max_sandboxes}</td>
                      <td className="px-5 py-3 text-gray-300">{quota.max_ttl || 'none'}</td>
                      <td className="px-5 py-3 text-gray-300">
                        {quota.max_exec_timeout || 'none'}
                      </td>
                      <td className="px-5 py-3">
                        <div className="flex justify-end gap-2">
                          <button
                            onClick={() => {
                              setForm(toQuotaForm(quota));
                              setUsageOwner(quota.owner_id);
                            }}
                            className="btn-ghost text-xs"
                          >
                            Edit
                          </button>
                          <button
                            onClick={() => checkUsage(quota.owner_id)}
                            className="btn-ghost text-xs"
                          >
                            Usage
                          </button>
                          <button
                            onClick={() => removeQuota(quota.owner_id)}
                            className="btn-ghost text-xs text-red-400 hover:text-red-300"
                            title="Delete quota"
                          >
                            <Trash2 className="w-4 h-4" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        <div className="space-y-6">
          <div className="card space-y-4">
            <h3 className="text-base font-bold text-gray-100">Save Quota</h3>
            <Field label="Owner ID">
              <input
                className="input w-full"
                value={form.owner_id}
                onChange={(e) => setForm((prev) => ({ ...prev, owner_id: e.target.value }))}
                placeholder="owner-a"
              />
            </Field>
            <Field label="Max Sandboxes">
              <input
                className="input w-full"
                type="number"
                min={0}
                value={form.max_sandboxes}
                onChange={(e) =>
                  setForm((prev) => ({ ...prev, max_sandboxes: e.target.value }))
                }
              />
            </Field>
            <Field label="Max TTL">
              <input
                className="input w-full"
                value={form.max_ttl}
                onChange={(e) => setForm((prev) => ({ ...prev, max_ttl: e.target.value }))}
                placeholder="30m"
              />
            </Field>
            <Field label="Max Exec Timeout">
              <input
                className="input w-full"
                value={form.max_exec_timeout}
                onChange={(e) =>
                  setForm((prev) => ({ ...prev, max_exec_timeout: e.target.value }))
                }
                placeholder="30s"
              />
            </Field>
            <button onClick={submitQuota} disabled={saving} className="btn-primary w-full">
              {saving ? <Loader2 className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
              Save Quota
            </button>
          </div>

          <div className="card space-y-4">
            <h3 className="text-base font-bold text-gray-100">Owner Usage</h3>
            <div className="flex gap-2">
              <input
                className="input flex-1 min-w-0"
                value={usageOwner}
                onChange={(e) => setUsageOwner(e.target.value)}
                placeholder="owner-a"
              />
              <button
                onClick={() => checkUsage()}
                disabled={checkingUsage}
                className="btn-secondary flex-shrink-0"
                title="Check usage"
              >
                {checkingUsage ? (
                  <Loader2 className="w-4 h-4 animate-spin" />
                ) : (
                  <Search className="w-4 h-4" />
                )}
              </button>
            </div>
            {usage && (
              <div className="grid grid-cols-2 gap-3 text-sm">
                <UsageStat label="Active" value={usage.active_sandboxes} />
                <UsageStat label="Max" value={usage.max_sandboxes} />
                <UsageStat label="TTL" value={usage.max_ttl || 'none'} />
                <UsageStat label="Exec" value={usage.max_exec_timeout || 'none'} />
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function DiagnosticsPanel() {
  const [diagnostics, setDiagnostics] = useState<DiagnosticsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = async () => {
    setLoading(true);
    try {
      const result = await getDiagnostics();
      setDiagnostics(result);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load diagnostics');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    refresh();
  }, []);

  if (loading) {
    return (
      <div className="card flex items-center justify-center py-16 text-gray-500">
        <Loader2 className="w-5 h-5 animate-spin" />
      </div>
    );
  }

  if (error || !diagnostics) {
    return (
      <div className="card flex flex-col items-center justify-center py-16 text-gray-500">
        <AlertCircle className="w-8 h-8 mb-3 opacity-60" />
        <p className="text-sm font-medium">{error || 'Diagnostics unavailable'}</p>
        <button onClick={refresh} className="btn-secondary text-sm mt-4">
          Try Again
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-end">
        <button onClick={refresh} className="btn-secondary text-sm">
          <RefreshCw className="w-4 h-4" />
          Refresh
        </button>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-4 gap-4">
        <SummaryCard
          label="Store"
          value={diagnostics.store.healthy ? 'Healthy' : 'Degraded'}
          icon={diagnostics.store.healthy ? CheckCircle2 : AlertCircle}
        />
        <SummaryCard label="Providers" value={diagnostics.providers.length} icon={Database} />
        <SummaryCard label="Sandboxes" value={diagnostics.sandboxes.total} icon={Gauge} />
        <SummaryCard label="Redactions" value={diagnostics.redactions.length} icon={Shield} />
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
        <DiagnosticsCard title="Process" data={diagnostics.process} />
        <DiagnosticsCard title="Scheduler" data={diagnostics.scheduler} />
        <DiagnosticsCard title="Rate Limit" data={diagnostics.rate_limit} />
        <DiagnosticsCard title="Build" data={diagnostics.build} />
      </div>

      <div className="card">
        <h3 className="text-base font-bold text-gray-100 mb-3">Redactions</h3>
        <div className="flex flex-wrap gap-2">
          {diagnostics.redactions.map((item) => (
            <span
              key={item}
              className="text-xs rounded-full border border-amber-500/20 bg-amber-500/10 text-amber-300 px-2 py-1"
            >
              {item}
            </span>
          ))}
        </div>
      </div>
    </div>
  );
}

function SummaryCard({
  label,
  value,
  icon: Icon,
}: {
  label: string;
  value: string | number;
  icon: typeof Shield;
}) {
  return (
    <div className="card !p-4 flex items-center gap-3">
      <div className="flex items-center justify-center w-10 h-10 rounded-lg bg-primary-500/10">
        <Icon className="w-5 h-5 text-primary-400" />
      </div>
      <div className="min-w-0">
        <p className="text-2xl font-bold text-gray-100 truncate">{value}</p>
        <p className="text-xs text-gray-500">{label}</p>
      </div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="block text-xs text-gray-500 mb-1.5">{label}</span>
      {children}
    </label>
  );
}

function UsageStat({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-lg bg-navy-800 border border-navy-700 px-3 py-2">
      <p className="text-xs text-gray-500">{label}</p>
      <p className="text-sm font-mono text-gray-200 truncate">{value}</p>
    </div>
  );
}

function DiagnosticsCard({
  title,
  data,
}: {
  title: string;
  data: Record<string, unknown>;
}) {
  return (
    <div className="card !p-0 overflow-hidden">
      <div className="px-5 py-4 border-b border-navy-700">
        <h3 className="text-base font-bold text-gray-100">{title}</h3>
      </div>
      <div className="divide-y divide-navy-700">
        {Object.entries(data).map(([key, value]) => (
          <div key={key} className="grid grid-cols-[160px_minmax(0,1fr)] gap-3 px-5 py-2.5">
            <span className="text-xs font-mono text-gray-500 truncate">{key}</span>
            <span className="text-xs font-mono text-gray-300 truncate">
              {formatUnknown(value)}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
