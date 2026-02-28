import { Monitor, Crown } from 'lucide-react';
import type { Peer } from '../types';

interface PeersPanelProps {
  peers: Peer[];
}

export function PeersPanel({ peers }: PeersPanelProps) {
  return (
    <div className="bg-bg-secondary border border-border rounded-xl p-5">
      <h3 className="text-sm font-semibold text-amber mb-4 flex items-center gap-2">
        <Monitor className="w-4 h-4" />
        Peers ({peers.length})
      </h3>

      <div className="space-y-3">
        {peers.map((peer) => (
          <PeerCard key={peer.id} peer={peer} />
        ))}
      </div>
    </div>
  );
}

function PeerCard({ peer }: { peer: Peer }) {
  const vramUsed = peer.resources.vram_total_mb - peer.resources.vram_free_mb;
  const vramPct = Math.round((vramUsed / peer.resources.vram_total_mb) * 100);
  const layerRange = peer.layers.length > 0
    ? `L${peer.layers[0]}-${peer.layers[peer.layers.length - 1]}`
    : '—';

  return (
    <div className="bg-bg-tertiary border border-border rounded-lg p-3">
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-2">
          {peer.is_host && <Crown className="w-3.5 h-3.5 text-amber" />}
          <span className="text-sm font-medium text-text-primary">{peer.name}</span>
          <StatusBadge state={peer.state} />
        </div>
        <span className="text-xs text-text-muted">{peer.ip}</span>
      </div>

      <div className="grid grid-cols-3 gap-3 text-xs">
        <div>
          <span className="text-text-muted block">GPU</span>
          <span className="text-text-secondary truncate block" title={peer.resources.gpu_name}>
            {peer.resources.gpu_name.replace('NVIDIA ', '')}
          </span>
        </div>
        <div>
          <span className="text-text-muted block">Layers</span>
          <span className="text-text-primary font-medium">{layerRange}</span>
        </div>
        <div>
          <span className="text-text-muted block">Latency</span>
          <span className={`font-medium ${getLatencyColor(peer.latency_ms)}`}>
            {peer.latency_ms > 0 ? `${peer.latency_ms.toFixed(0)}ms` : 'local'}
          </span>
        </div>
      </div>

      {/* VRAM bar */}
      <div className="mt-2.5">
        <div className="flex justify-between text-xs mb-1">
          <span className="text-text-muted">VRAM</span>
          <span className="text-text-secondary">
            {Math.round(vramUsed / 1024)}GB / {Math.round(peer.resources.vram_total_mb / 1024)}GB
          </span>
        </div>
        <div className="h-1.5 bg-bg-primary rounded-full overflow-hidden">
          <div
            className="h-full rounded-full transition-all duration-500"
            style={{
              width: `${vramPct}%`,
              backgroundColor: vramPct > 90 ? '#ef4444' : vramPct > 70 ? '#f59e0b' : '#10b981',
            }}
          />
        </div>
      </div>
    </div>
  );
}

function StatusBadge({ state }: { state: string }) {
  const config: Record<string, { color: string; label: string }> = {
    ready: { color: 'text-green bg-green/10 border-green/20', label: 'Ready' },
    syncing: { color: 'text-amber bg-amber/10 border-amber/20', label: 'Syncing' },
    connecting: { color: 'text-blue bg-blue/10 border-blue/20', label: 'Connecting' },
    offline: { color: 'text-red bg-red/10 border-red/20', label: 'Offline' },
  };

  const cfg = config[state] || config.offline;

  return (
    <span className={`text-[10px] px-1.5 py-0.5 rounded-full border ${cfg.color}`}>
      {cfg.label}
    </span>
  );
}

function getLatencyColor(ms: number): string {
  if (ms === 0) return 'text-green';
  if (ms < 50) return 'text-green';
  if (ms < 100) return 'text-amber';
  return 'text-red';
}
