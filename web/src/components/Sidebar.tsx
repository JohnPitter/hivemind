import { LayoutDashboard, MessageSquare, Settings, Cpu, Wifi, WifiOff } from 'lucide-react';
import type { RoomStatus } from '../types';

interface SidebarProps {
  status: RoomStatus;
  activeTab: string;
  onTabChange: (tab: 'dashboard' | 'chat' | 'room') => void;
}

export function Sidebar({ status, activeTab, onTabChange }: SidebarProps) {
  const { room } = status;

  const navItems = [
    { id: 'dashboard' as const, label: 'Dashboard', icon: LayoutDashboard },
    { id: 'chat' as const, label: 'Chat', icon: MessageSquare },
    { id: 'room' as const, label: 'Room', icon: Settings },
  ];

  return (
    <aside className="w-60 bg-bg-secondary border-r border-border flex flex-col">
      {/* Logo */}
      <div className="p-4 border-b border-border">
        <div className="flex items-center gap-2">
          <Cpu className="w-6 h-6 text-amber" />
          <span className="text-lg font-bold text-amber">HiveMind</span>
        </div>
        <p className="text-text-muted text-xs mt-1">Distributed AI Inference</p>
      </div>

      {/* Room status */}
      <div className="p-4 border-b border-border">
        <div className="flex items-center gap-2 mb-2">
          {room.state === 'active' ? (
            <Wifi className="w-3.5 h-3.5 text-green" />
          ) : (
            <WifiOff className="w-3.5 h-3.5 text-red" />
          )}
          <span className="text-xs text-text-secondary">
            {room.state === 'active' ? 'Connected' : 'Disconnected'}
          </span>
        </div>
        <p className="text-sm font-medium text-text-primary truncate" title={room.model_id}>
          {room.model_id.split('/').pop()}
        </p>
        <p className="text-xs text-text-muted mt-0.5">
          {room.peers.length} peers · {room.total_layers} layers
        </p>
      </div>

      {/* Navigation */}
      <nav className="flex-1 p-2">
        {navItems.map((item) => {
          const Icon = item.icon;
          const isActive = activeTab === item.id;

          return (
            <button
              key={item.id}
              onClick={() => onTabChange(item.id)}
              className={`
                w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm mb-1
                transition-colors
                ${isActive
                  ? 'bg-amber/10 text-amber border border-amber/20'
                  : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'
                }
              `}
            >
              <Icon className="w-4 h-4" />
              {item.label}
            </button>
          );
        })}
      </nav>

      {/* Footer */}
      <div className="p-4 border-t border-border">
        <div className="flex items-center justify-between">
          <span className="text-xs text-text-muted">v0.2.0</span>
          <span className="text-xs text-text-muted">{status.uptime}</span>
        </div>
      </div>
    </aside>
  );
}
