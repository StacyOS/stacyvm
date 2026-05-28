import { useState, useEffect } from 'react';
import { Plus, Trash2, Users, ShieldCheck, ChevronDown, ChevronRight, Download } from 'lucide-react';

interface Tenant {
  id: string;
  name: string;
  owner_id: string;
  settings: string;
  created_at: string;
  updated_at: string;
}

interface TenantMember {
  tenant_id: string;
  user_id: string;
  role: string;
  created_at: string;
}

interface Policy {
  id: string;
  tenant_id: string;
  resource_type: string;
  effect: string;
  pattern: string;
  priority: number;
}

const BASE = '/api/v1/admin/tenants';

async function apiFetch<T>(url: string, init?: RequestInit): Promise<T> {
  const apiKey = localStorage.getItem('stacyvm-admin-key') ?? '';
  const res = await fetch(url, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      'X-Admin-API-Key': apiKey,
      ...(init?.headers ?? {}),
    },
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.message ?? res.statusText);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export default function Tenants() {
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [members, setMembers] = useState<Record<string, TenantMember[]>>({});
  const [policies, setPolicies] = useState<Record<string, Policy[]>>({});

  // New tenant form
  const [newName, setNewName] = useState('');
  const [newOwner, setNewOwner] = useState('');
  const [creating, setCreating] = useState(false);

  // New member form
  const [memberUserID, setMemberUserID] = useState('');
  const [memberRole, setMemberRole] = useState('viewer');

  // New policy form
  const [polResourceType, setPolResourceType] = useState('image');
  const [polEffect, setPolEffect] = useState('allow');
  const [polPattern, setPolPattern] = useState('');
  const [polPriority, setPolPriority] = useState(10);

  async function loadTenants() {
    setLoading(true);
    setError(null);
    try {
      const data = await apiFetch<{ tenants: Tenant[] }>(BASE);
      setTenants(data.tenants ?? []);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { loadTenants(); }, []);

  async function expand(tenantID: string) {
    if (expanded === tenantID) { setExpanded(null); return; }
    setExpanded(tenantID);
    if (!members[tenantID]) {
      const [m, p] = await Promise.all([
        apiFetch<{ members: TenantMember[] }>(`${BASE}/${tenantID}/members`),
        apiFetch<{ policies: Policy[] }>(`${BASE}/${tenantID}/policies`),
      ]);
      setMembers(prev => ({ ...prev, [tenantID]: m.members ?? [] }));
      setPolicies(prev => ({ ...prev, [tenantID]: p.policies ?? [] }));
    }
  }

  async function createTenant() {
    if (!newName.trim()) return;
    setCreating(true);
    try {
      await apiFetch(BASE, {
        method: 'POST',
        body: JSON.stringify({ name: newName, owner_id: newOwner }),
      });
      setNewName(''); setNewOwner('');
      await loadTenants();
    } catch (e: any) {
      setError(e.message);
    } finally {
      setCreating(false);
    }
  }

  async function deleteTenant(id: string) {
    if (!confirm(`Delete tenant ${id}?`)) return;
    try {
      await apiFetch(`${BASE}/${id}`, { method: 'DELETE' });
      setTenants(prev => prev.filter(t => t.id !== id));
      if (expanded === id) setExpanded(null);
    } catch (e: any) {
      setError(e.message);
    }
  }

  async function addMember(tenantID: string) {
    if (!memberUserID.trim()) return;
    try {
      const m = await apiFetch<{ member: TenantMember }>(`${BASE}/${tenantID}/members/${memberUserID}`, {
        method: 'PUT',
        body: JSON.stringify({ role: memberRole }),
      });
      setMembers(prev => ({
        ...prev,
        [tenantID]: [...(prev[tenantID] ?? []).filter(x => x.user_id !== memberUserID), m.member],
      }));
      setMemberUserID('');
    } catch (e: any) {
      setError(e.message);
    }
  }

  async function removeMember(tenantID: string, userID: string) {
    try {
      await apiFetch(`${BASE}/${tenantID}/members/${userID}`, { method: 'DELETE' });
      setMembers(prev => ({ ...prev, [tenantID]: (prev[tenantID] ?? []).filter(m => m.user_id !== userID) }));
    } catch (e: any) {
      setError(e.message);
    }
  }

  async function addPolicy(tenantID: string) {
    if (!polPattern.trim()) return;
    try {
      const p = await apiFetch<{ policy: Policy }>(`${BASE}/${tenantID}/policies`, {
        method: 'POST',
        body: JSON.stringify({ resource_type: polResourceType, effect: polEffect, pattern: polPattern, priority: polPriority }),
      });
      setPolicies(prev => ({ ...prev, [tenantID]: [...(prev[tenantID] ?? []), p.policy] }));
      setPolPattern('');
    } catch (e: any) {
      setError(e.message);
    }
  }

  async function removePolicy(tenantID: string, policyID: string) {
    try {
      await apiFetch(`${BASE}/${tenantID}/policies/${policyID}`, { method: 'DELETE' });
      setPolicies(prev => ({ ...prev, [tenantID]: (prev[tenantID] ?? []).filter(p => p.id !== policyID) }));
    } catch (e: any) {
      setError(e.message);
    }
  }

  async function exportAudit(tenantID: string) {
    const data = await apiFetch<object>(`${BASE}/${tenantID}/audit`);
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url; a.download = `audit-${tenantID}-${Date.now()}.json`; a.click();
    URL.revokeObjectURL(url);
  }

  const roleColor = (role: string) => {
    switch (role) {
      case 'admin': return 'bg-red-500/20 text-red-300';
      case 'operator': return 'bg-yellow-500/20 text-yellow-300';
      default: return 'bg-blue-500/20 text-blue-300';
    }
  };

  const effectColor = (effect: string) => effect === 'allow' ? 'text-green-400' : 'text-red-400';

  return (
    <div className="p-6 max-w-5xl mx-auto space-y-6">
      <div className="flex items-center gap-3">
        <Users className="w-6 h-6 text-violet-400" />
        <h1 className="text-2xl font-semibold text-white">Tenants</h1>
      </div>

      {error && (
        <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-3 text-red-300 text-sm">{error}</div>
      )}

      {/* Create tenant */}
      <div className="bg-gray-800/60 rounded-xl border border-gray-700 p-4">
        <h2 className="text-sm font-medium text-gray-300 mb-3">Create Tenant</h2>
        <div className="flex gap-2 flex-wrap">
          <input
            className="flex-1 min-w-[160px] bg-gray-900 border border-gray-600 rounded-lg px-3 py-2 text-sm text-white placeholder-gray-500 focus:outline-none focus:border-violet-500"
            placeholder="Tenant name"
            value={newName}
            onChange={e => setNewName(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && createTenant()}
          />
          <input
            className="flex-1 min-w-[160px] bg-gray-900 border border-gray-600 rounded-lg px-3 py-2 text-sm text-white placeholder-gray-500 focus:outline-none focus:border-violet-500"
            placeholder="Owner user ID (optional)"
            value={newOwner}
            onChange={e => setNewOwner(e.target.value)}
          />
          <button
            onClick={createTenant}
            disabled={creating || !newName.trim()}
            className="flex items-center gap-1.5 px-4 py-2 bg-violet-600 hover:bg-violet-500 disabled:opacity-50 rounded-lg text-sm text-white font-medium transition"
          >
            <Plus className="w-4 h-4" />
            {creating ? 'Creating…' : 'Create'}
          </button>
        </div>
      </div>

      {/* Tenant list */}
      {loading ? (
        <div className="text-gray-400 text-sm">Loading…</div>
      ) : tenants.length === 0 ? (
        <div className="text-gray-500 text-sm">No tenants yet.</div>
      ) : (
        <div className="space-y-3">
          {tenants.map(t => (
            <div key={t.id} className="bg-gray-800/60 rounded-xl border border-gray-700 overflow-hidden">
              {/* Tenant header */}
              <div className="flex items-center gap-3 px-4 py-3">
                <button onClick={() => expand(t.id)} className="flex items-center gap-2 flex-1 text-left">
                  {expanded === t.id ? <ChevronDown className="w-4 h-4 text-gray-400" /> : <ChevronRight className="w-4 h-4 text-gray-400" />}
                  <span className="text-white font-medium">{t.name}</span>
                  <span className="text-gray-500 text-xs font-mono">{t.id}</span>
                  {t.owner_id && <span className="text-gray-500 text-xs">owner: {t.owner_id}</span>}
                </button>
                <button onClick={() => exportAudit(t.id)} title="Export audit log" className="p-1.5 text-gray-400 hover:text-blue-400 transition">
                  <Download className="w-4 h-4" />
                </button>
                <button onClick={() => deleteTenant(t.id)} className="p-1.5 text-gray-400 hover:text-red-400 transition">
                  <Trash2 className="w-4 h-4" />
                </button>
              </div>

              {/* Expanded detail */}
              {expanded === t.id && (
                <div className="border-t border-gray-700 px-4 py-4 space-y-5">
                  {/* Members */}
                  <section>
                    <div className="flex items-center gap-2 mb-2">
                      <Users className="w-4 h-4 text-violet-400" />
                      <span className="text-sm font-medium text-gray-300">Members</span>
                    </div>
                    {(members[t.id] ?? []).length === 0 ? (
                      <p className="text-gray-500 text-xs">No members.</p>
                    ) : (
                      <table className="w-full text-xs mb-2">
                        <thead>
                          <tr className="text-gray-500">
                            <th className="text-left pb-1">User ID</th>
                            <th className="text-left pb-1">Role</th>
                            <th></th>
                          </tr>
                        </thead>
                        <tbody>
                          {(members[t.id] ?? []).map(m => (
                            <tr key={m.user_id} className="border-t border-gray-700/50">
                              <td className="py-1.5 font-mono text-gray-200">{m.user_id}</td>
                              <td className="py-1.5"><span className={`px-2 py-0.5 rounded-full text-xs ${roleColor(m.role)}`}>{m.role}</span></td>
                              <td className="py-1.5 text-right">
                                <button onClick={() => removeMember(t.id, m.user_id)} className="text-gray-500 hover:text-red-400 transition">
                                  <Trash2 className="w-3.5 h-3.5" />
                                </button>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    )}
                    <div className="flex gap-2 mt-2">
                      <input
                        className="flex-1 bg-gray-900 border border-gray-600 rounded px-2 py-1.5 text-xs text-white placeholder-gray-500 focus:outline-none focus:border-violet-500"
                        placeholder="user-id or email"
                        value={memberUserID}
                        onChange={e => setMemberUserID(e.target.value)}
                      />
                      <select
                        value={memberRole}
                        onChange={e => setMemberRole(e.target.value)}
                        className="bg-gray-900 border border-gray-600 rounded px-2 py-1.5 text-xs text-white"
                      >
                        <option value="viewer">viewer</option>
                        <option value="operator">operator</option>
                        <option value="admin">admin</option>
                      </select>
                      <button
                        onClick={() => addMember(t.id)}
                        disabled={!memberUserID.trim()}
                        className="px-3 py-1.5 bg-violet-700 hover:bg-violet-600 disabled:opacity-50 rounded text-xs text-white transition"
                      >Add</button>
                    </div>
                  </section>

                  {/* Policies */}
                  <section>
                    <div className="flex items-center gap-2 mb-2">
                      <ShieldCheck className="w-4 h-4 text-green-400" />
                      <span className="text-sm font-medium text-gray-300">Policies</span>
                    </div>
                    {(policies[t.id] ?? []).length === 0 ? (
                      <p className="text-gray-500 text-xs">No policies (all resources allowed by default).</p>
                    ) : (
                      <table className="w-full text-xs mb-2">
                        <thead>
                          <tr className="text-gray-500">
                            <th className="text-left pb-1">Type</th>
                            <th className="text-left pb-1">Effect</th>
                            <th className="text-left pb-1">Pattern</th>
                            <th className="text-left pb-1">Priority</th>
                            <th></th>
                          </tr>
                        </thead>
                        <tbody>
                          {(policies[t.id] ?? []).map(p => (
                            <tr key={p.id} className="border-t border-gray-700/50">
                              <td className="py-1.5 text-gray-300">{p.resource_type}</td>
                              <td className={`py-1.5 font-medium ${effectColor(p.effect)}`}>{p.effect}</td>
                              <td className="py-1.5 font-mono text-gray-200">{p.pattern}</td>
                              <td className="py-1.5 text-gray-400">{p.priority}</td>
                              <td className="py-1.5 text-right">
                                <button onClick={() => removePolicy(t.id, p.id)} className="text-gray-500 hover:text-red-400 transition">
                                  <Trash2 className="w-3.5 h-3.5" />
                                </button>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    )}
                    <div className="flex gap-2 mt-2 flex-wrap">
                      <select
                        value={polResourceType}
                        onChange={e => setPolResourceType(e.target.value)}
                        className="bg-gray-900 border border-gray-600 rounded px-2 py-1.5 text-xs text-white"
                      >
                        <option value="image">image</option>
                        <option value="provider">provider</option>
                        <option value="network">network</option>
                      </select>
                      <select
                        value={polEffect}
                        onChange={e => setPolEffect(e.target.value)}
                        className="bg-gray-900 border border-gray-600 rounded px-2 py-1.5 text-xs text-white"
                      >
                        <option value="allow">allow</option>
                        <option value="deny">deny</option>
                      </select>
                      <input
                        className="flex-1 min-w-[120px] bg-gray-900 border border-gray-600 rounded px-2 py-1.5 text-xs text-white placeholder-gray-500 focus:outline-none focus:border-violet-500"
                        placeholder="pattern (e.g. alpine:*)"
                        value={polPattern}
                        onChange={e => setPolPattern(e.target.value)}
                      />
                      <input
                        type="number"
                        className="w-16 bg-gray-900 border border-gray-600 rounded px-2 py-1.5 text-xs text-white"
                        value={polPriority}
                        onChange={e => setPolPriority(Number(e.target.value))}
                        title="Priority (lower = higher priority)"
                      />
                      <button
                        onClick={() => addPolicy(t.id)}
                        disabled={!polPattern.trim()}
                        className="px-3 py-1.5 bg-green-700 hover:bg-green-600 disabled:opacity-50 rounded text-xs text-white transition"
                      >Add Policy</button>
                    </div>
                  </section>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
