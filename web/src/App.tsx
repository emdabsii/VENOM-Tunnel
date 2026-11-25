import React, { useEffect, useState } from 'react';
import {
  Activity, Cpu, Globe, KeyRound, Layers, Link2, Play, Plus, RefreshCw,
  Settings, ShieldCheck, Square, Terminal, Trash2, Zap
} from 'lucide-react';
import { Lang, TRANSLATIONS } from './i18n/translations';

type Theme = 'toxic' | 'carnage' | 'cyber' | 'light';
type View = 'dashboard' | 'tunnels' | 'logs' | 'diag' | 'pair' | 'settings';

interface Tunnel {
  id: string;
  name: string;
  engine: string;
  role: 'iran' | 'foreign';
  local_ip: string;
  remote_ip: string;
  control_port: number;
  ports: string;
  token: string;
  imported: boolean;
  state: string;
}

export default function App() {
  const [lang, setLang] = useState<Lang>('fa');
  const [theme, setTheme] = useState<Theme>('toxic');
  const [authed, setAuthed] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [view, setView] = useState<View>('dashboard');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [serverIP, setServerIP] = useState('46.38.142.250');
  const [telemetry, setTelemetry] = useState({ cpu_pct: 14.2, ram_pct: 28.4, rx_mbps: 42.6, tx_mbps: 38.1 });
  const [intlOpen, setIntlOpen] = useState(true);
  const [logsText, setLogsText] = useState('');
  const [selectedTunnel, setSelectedTunnel] = useState('backhaul-main');

  const t = TRANSLATIONS[lang];

  useEffect(() => {
    document.documentElement.lang = lang;
    document.documentElement.dir = lang === 'fa' ? 'rtl' : 'ltr';
  }, [lang]);

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme);
  }, [theme]);

  async function refreshData() {
    try {
      const [tr, tel, intl] = await Promise.all([
        fetch('/api/tunnels').then(r => r.json()),
        fetch('/api/system/telemetry').then(r => r.json()),
        fetch('/api/system/intl').then(r => r.json())
      ]);
      if (tr.tunnels) setTunnels(tr.tunnels);
      if (tr.server_ip) setServerIP(tr.server_ip);
      if (tel.cpu_pct !== undefined) setTelemetry(tel);
      if (intl.international_open !== undefined) setIntlOpen(intl.international_open);
    } catch { /* offline preview */ }
  }

  async function handleLogin(e: React.FormEvent) {
    e.preventDefault();
    setConnecting(true);
    try {
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password })
      });
      setTimeout(() => {
        setConnecting(false);
        if (res.ok) {
          setAuthed(true);
          refreshData();
        }
      }, 1800);
    } catch {
      setTimeout(() => setConnecting(false), 1800);
    }
  }

  async function loadLogs(name: string) {
    setSelectedTunnel(name);
    setView('logs');
    try {
      const r = await fetch(`/api/tunnels/logs?name=${encodeURIComponent(name)}`).then(x => x.json());
      setLogsText(r.logs || '');
    } catch { /* ignore */ }
  }

  if (connecting) {
    return (
      <div className="min-h-screen flex flex-col items-center justify-center gap-6">
        <div className="w-28 h-28 rounded-full border-2 border-[var(--v-accent)] flex items-center justify-center venom-glow animate-venom-ring">
          <ShieldCheck className="w-12 h-12 text-[var(--v-accent)]" />
        </div>
        <div className="font-mono-code text-sm tracking-widest text-[var(--v-accent)]">{t.connecting}</div>
      </div>
    );
  }

  if (!authed) {
    return (
      <div className="min-h-screen flex items-center justify-center p-6">
        <form onSubmit={handleLogin} className="venom-card venom-glow p-8 w-full max-w-md space-y-5">
          <div className="text-center space-y-2">
            <div className="text-3xl font-extrabold tracking-wider text-[var(--v-accent)]">{t.brand}</div>
            <div className="text-xs text-[var(--v-muted)]">{t.login_title}</div>
          </div>
          <input value={username} onChange={e => setUsername(e.target.value)} placeholder="Username" className="w-full px-4 py-3 rounded-xl bg-black/40 border border-[var(--v-border)] outline-none font-mono-code" />
          <input type="password" value={password} onChange={e => setPassword(e.target.value)} placeholder="Password" className="w-full px-4 py-3 rounded-xl bg-black/40 border border-[var(--v-border)] outline-none font-mono-code" />
          <button type="submit" className="w-full py-3 rounded-xl font-bold bg-[var(--v-accent)] text-black flex items-center justify-center gap-2">
            <KeyRound className="w-4 h-4" /> {t.login_btn}
          </button>
        </form>
      </div>
    );
  }

  return (
    <div className="min-h-screen flex">
      <aside className="w-64 p-5 border-e border-[var(--v-border)] bg-[var(--v-surface)] flex flex-col gap-2">
        <div className="text-xl font-extrabold text-[var(--v-accent)] mb-6">{t.brand}</div>
        {([
          ['dashboard', Activity, t.nav_dashboard],
          ['tunnels', Layers, t.nav_tunnels],
          ['logs', Terminal, t.nav_logs],
          ['diag', Zap, t.nav_diag],
          ['pair', Link2, t.nav_pair],
          ['settings', Settings, t.nav_settings]
        ] as const).map(([id, Icon, label]) => (
          <button key={id} onClick={() => setView(id)}
            className={`flex items-center gap-3 px-4 py-3 rounded-xl text-sm font-semibold transition ${view === id ? 'bg-[var(--v-accent)] text-black' : 'hover:bg-white/5'}`}>
            <Icon className="w-4 h-4" /> {label}
          </button>
        ))}
        <div className="mt-auto pt-4 border-t border-[var(--v-border)] space-y-2">
          <div className="flex gap-1">
            {(['fa', 'en', 'zh', 'ru'] as Lang[]).map(l => (
              <button key={l} onClick={() => setLang(l)} className={`flex-1 py-1 text-xs rounded-lg font-mono-code ${lang === l ? 'bg-[var(--v-accent)] text-black font-bold' : 'bg-white/5'}`}>{l.toUpperCase()}</button>
            ))}
          </div>
          <div className="flex gap-1">
            {(['toxic', 'carnage', 'cyber', 'light'] as Theme[]).map(th => (
              <button key={th} onClick={() => setTheme(th)} className={`flex-1 py-1 text-[10px] rounded-lg ${theme === th ? 'bg-[var(--v-accent-2)] text-black font-bold' : 'bg-white/5'}`}>{th}</button>
            ))}
          </div>
        </div>
      </aside>

      <main className="flex-1 p-6 space-y-6">
        {view === 'dashboard' && (
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
            <div className="venom-card p-5"><div className="text-xs text-[var(--v-muted)]">{t.cpu}</div><div className="text-2xl font-bold font-mono-code mt-1">{telemetry.cpu_pct}%</div></div>
            <div className="venom-card p-5"><div className="text-xs text-[var(--v-muted)]">{t.ram}</div><div className="text-2xl font-bold font-mono-code mt-1">{telemetry.ram_pct}%</div></div>
            <div className="venom-card p-5"><div className="text-xs text-[var(--v-muted)]">{t.traffic}</div><div className="text-2xl font-bold font-mono-code mt-1">{telemetry.rx_mbps} Mbps</div></div>
            <div className="venom-card p-5"><div className="text-xs text-[var(--v-muted)]">{t.intl}</div><div className="text-xl font-bold mt-1 text-[var(--v-accent)]">{intlOpen ? t.intl_open : t.intl_blocked}</div></div>
          </div>
        )}

        {view === 'tunnels' && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {tunnels.map(item => (
              <div key={item.name} className="venom-card p-5 space-y-3">
                <div className="flex items-center justify-between">
                  <div className="font-bold text-lg">{item.name}</div>
                  <span className="text-xs px-2 py-1 rounded-full bg-[var(--v-accent)]/20 text-[var(--v-accent)] font-mono-code">{item.engine}</span>
                </div>
                <div className="font-mono-code text-xs text-[var(--v-muted)]">{item.local_ip} ⇄ {item.remote_ip || 'listening'} :{item.control_port}</div>
                <div className="flex gap-2 pt-2">
                  <button onClick={() => loadLogs(item.name)} className="px-3 py-1.5 rounded-lg bg-white/10 text-xs">{t.logs}</button>
                  <button onClick={() => fetch('/api/tunnels/action', { method: 'POST', body: JSON.stringify({ name: item.name, action: 'restart' }) })} className="px-3 py-1.5 rounded-lg bg-white/10 text-xs">{t.restart}</button>
                </div>
              </div>
            ))}
          </div>
        )}

        {view === 'logs' && (
          <pre className="venom-card p-5 font-mono-code text-xs overflow-auto max-h-[70vh] text-[var(--v-accent)]">{logsText || `Waiting for logs of ${selectedTunnel}...`}</pre>
        )}
      </main>
    </div>
  );
}
