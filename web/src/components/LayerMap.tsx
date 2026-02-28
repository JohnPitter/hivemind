import { Layers } from 'lucide-react';
import type { Peer } from '../types';

interface LayerMapProps {
  peers: Peer[];
  totalLayers: number;
}

const PEER_COLORS = ['#10b981', '#3b82f6', '#8b5cf6', '#f59e0b', '#ef4444'];

export function LayerMap({ peers, totalLayers }: LayerMapProps) {
  return (
    <div className="bg-bg-secondary border border-border rounded-xl p-5">
      <h3 className="text-sm font-semibold text-amber mb-4 flex items-center gap-2">
        <Layers className="w-4 h-4" />
        Layer Distribution
      </h3>

      {/* Visual bar */}
      <div className="h-8 bg-bg-tertiary rounded-lg overflow-hidden flex mb-4">
        {peers.map((peer, idx) => {
          if (peer.layers.length === 0) return null;

          const startPct = (peer.layers[0] / totalLayers) * 100;
          const widthPct = (peer.layers.length / totalLayers) * 100;
          const color = PEER_COLORS[idx % PEER_COLORS.length];

          return (
            <div
              key={peer.id}
              className="h-full flex items-center justify-center text-[10px] font-bold text-black/70 transition-all duration-500"
              style={{
                width: `${widthPct}%`,
                backgroundColor: color,
                marginLeft: idx === 0 ? `${startPct}%` : 0,
              }}
              title={`${peer.name}: layers ${peer.layers[0]}-${peer.layers[peer.layers.length - 1]}`}
            >
              {widthPct > 15 && peer.name.split('-').pop()}
            </div>
          );
        })}
      </div>

      {/* Scale */}
      <div className="flex justify-between text-[10px] text-text-muted mb-4 px-0.5">
        <span>Layer 0</span>
        <span>Layer {Math.floor(totalLayers / 2)}</span>
        <span>Layer {totalLayers}</span>
      </div>

      {/* Legend */}
      <div className="space-y-2">
        {peers.map((peer, idx) => {
          if (peer.layers.length === 0) return null;
          const color = PEER_COLORS[idx % PEER_COLORS.length];
          const pct = Math.round((peer.layers.length / totalLayers) * 100);

          return (
            <div key={peer.id} className="flex items-center gap-3 text-xs">
              <div
                className="w-3 h-3 rounded-sm flex-shrink-0"
                style={{ backgroundColor: color }}
              />
              <span className="text-text-primary font-medium w-28 truncate">{peer.name}</span>
              <span className="text-text-muted">
                L{peer.layers[0]}–{peer.layers[peer.layers.length - 1]}
              </span>
              <span className="text-text-secondary ml-auto">{peer.layers.length} layers ({pct}%)</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
