import { Activity, Zap, HardDrive, Clock } from 'lucide-react';
import type { RoomStatus } from '../types';

interface HeaderProps {
  status: RoomStatus;
}

export function Header({ status }: HeaderProps) {
  const vramPct = Math.round((status.used_vram_mb / status.total_vram_mb) * 100);

  return (
    <header className="h-14 bg-bg-secondary border-b border-border flex items-center px-6 gap-6">
      <div className="flex items-center gap-2">
        <Activity className="w-4 h-4 text-green" />
        <span className="text-sm text-text-secondary">
          <span className="text-text-primary font-medium">{status.room.peers.length}</span> peers
        </span>
      </div>

      <div className="flex items-center gap-2">
        <Zap className="w-4 h-4 text-amber" />
        <span className="text-sm text-text-secondary">
          <span className="text-text-primary font-medium">{status.tokens_per_sec}</span> tok/s
        </span>
      </div>

      <div className="flex items-center gap-2">
        <HardDrive className="w-4 h-4 text-blue" />
        <span className="text-sm text-text-secondary">
          VRAM <span className="text-text-primary font-medium">{vramPct}%</span>
        </span>
      </div>

      <div className="flex items-center gap-2">
        <Clock className="w-4 h-4 text-purple" />
        <span className="text-sm text-text-secondary">
          <span className="text-text-primary font-medium">{status.uptime}</span>
        </span>
      </div>

      <div className="ml-auto">
        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-green/10 text-green text-xs font-medium border border-green/20">
          <span className="w-1.5 h-1.5 rounded-full bg-green animate-pulse" />
          Active
        </span>
      </div>
    </header>
  );
}
