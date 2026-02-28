import { HardDrive, Cpu, Zap, Network } from 'lucide-react';
import type { RoomStatus } from '../types';

interface ResourceMonitorProps {
  status: RoomStatus;
}

export function ResourceMonitor({ status }: ResourceMonitorProps) {
  const vramPct = Math.round((status.used_vram_mb / status.total_vram_mb) * 100);

  const totalRAM = status.room.peers.reduce((sum, p) => sum + p.resources.ram_total_mb, 0);
  const usedRAM = status.room.peers.reduce(
    (sum, p) => sum + (p.resources.ram_total_mb - p.resources.ram_free_mb),
    0
  );
  const ramPct = Math.round((usedRAM / totalRAM) * 100);

  const avgLatency =
    status.room.peers.filter((p) => p.latency_ms > 0).reduce((sum, p) => sum + p.latency_ms, 0) /
    Math.max(status.room.peers.filter((p) => p.latency_ms > 0).length, 1);

  return (
    <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
      <StatCard
        icon={HardDrive}
        label="VRAM Usage"
        value={`${vramPct}%`}
        detail={`${Math.round(status.used_vram_mb / 1024)}GB / ${Math.round(status.total_vram_mb / 1024)}GB`}
        barPct={vramPct}
        barColor={vramPct > 90 ? '#ef4444' : vramPct > 70 ? '#f59e0b' : '#10b981'}
      />
      <StatCard
        icon={Cpu}
        label="RAM Usage"
        value={`${ramPct}%`}
        detail={`${Math.round(usedRAM / 1024)}GB / ${Math.round(totalRAM / 1024)}GB`}
        barPct={ramPct}
        barColor={ramPct > 90 ? '#ef4444' : '#3b82f6'}
      />
      <StatCard
        icon={Zap}
        label="Inference Speed"
        value={`${status.tokens_per_sec}`}
        detail="tokens/sec"
        barPct={Math.min((status.tokens_per_sec / 30) * 100, 100)}
        barColor="#f59e0b"
      />
      <StatCard
        icon={Network}
        label="Avg Latency"
        value={`${avgLatency.toFixed(0)}ms`}
        detail={`${status.room.peers.length} peers connected`}
        barPct={Math.min((avgLatency / 200) * 100, 100)}
        barColor={avgLatency < 50 ? '#10b981' : avgLatency < 100 ? '#f59e0b' : '#ef4444'}
      />
    </div>
  );
}

interface StatCardProps {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: string;
  detail: string;
  barPct: number;
  barColor: string;
}

function StatCard({ icon: Icon, label, value, detail, barPct, barColor }: StatCardProps) {
  return (
    <div className="bg-bg-secondary border border-border rounded-xl p-4">
      <div className="flex items-center gap-2 mb-3">
        <Icon className="w-4 h-4 text-text-muted" />
        <span className="text-xs text-text-muted">{label}</span>
      </div>

      <div className="text-2xl font-bold text-text-primary mb-1">{value}</div>
      <div className="text-xs text-text-muted mb-3">{detail}</div>

      <div className="h-1.5 bg-bg-tertiary rounded-full overflow-hidden">
        <div
          className="h-full rounded-full transition-all duration-700"
          style={{ width: `${barPct}%`, backgroundColor: barColor }}
        />
      </div>
    </div>
  );
}
