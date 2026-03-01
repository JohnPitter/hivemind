import { Activity, ArrowRightLeft, Gauge, Shrink, Zap, Timer, Hash } from 'lucide-react';
import type { DistributedStats } from '../types';

interface DistributedPanelProps {
  stats: DistributedStats;
}

export function DistributedPanel({ stats }: DistributedPanelProps) {
  if (!stats.is_distributed) return null;

  return (
    <div className="bg-bg-secondary border border-border rounded-xl p-5">
      <h3 className="text-sm font-semibold text-amber mb-4 flex items-center gap-2">
        <Activity className="w-4 h-4" />
        Distributed Inference
      </h3>

      {/* Generation metrics (prominent) */}
      {stats.generation_requests > 0 && (
        <div className="grid grid-cols-2 lg:grid-cols-3 gap-4 mb-4">
          <MetricCard
            icon={Zap}
            label="Tokens/sec"
            value={stats.tokens_per_second > 0 ? stats.tokens_per_second.toFixed(1) : '—'}
            detail={`${formatCount(stats.tokens_generated)} tokens generated`}
            highlight
          />
          <MetricCard
            icon={Timer}
            label="Token Latency"
            value={stats.avg_token_latency_ms > 0 ? `${stats.avg_token_latency_ms.toFixed(0)}ms` : '—'}
            detail={`embed: ${stats.embed_avg_ms.toFixed(0)}ms / sample: ${stats.sample_avg_ms.toFixed(0)}ms`}
            highlight
          />
          <MetricCard
            icon={Hash}
            label="Generations"
            value={formatCount(stats.generation_requests)}
            detail={`${formatCount(stats.tokens_generated)} total tokens`}
            highlight
          />
        </div>
      )}

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <MetricCard
          icon={ArrowRightLeft}
          label="Tensor Transfers"
          value={formatCount(stats.tensor_transfers)}
          detail={formatBytes(stats.bytes_transferred)}
        />
        <MetricCard
          icon={Gauge}
          label="Avg Forward Pass"
          value={`${stats.forward_pass_avg_ms.toFixed(0)}ms`}
          detail={`${stats.peer_count} peers in chain`}
        />
        <MetricCard
          icon={Shrink}
          label="Compression"
          value={`${Math.round((1 - stats.compression_ratio) * 100)}%`}
          detail={`saved (ratio: ${stats.compression_ratio.toFixed(2)})`}
        />
        <MetricCard
          icon={Activity}
          label="Avg Latency"
          value={`${stats.avg_latency_ms.toFixed(0)}ms`}
          detail={`${stats.total_layers} layers distributed`}
        />
      </div>

      {/* Pipeline visualization */}
      <div className="mt-4 pt-4 border-t border-border">
        <div className="flex items-center gap-2 text-xs text-text-muted mb-2">
          <span>Forward Pass Pipeline</span>
        </div>
        <div className="flex items-center gap-1">
          {Array.from({ length: stats.peer_count }).map((_, i) => (
            <div key={i} className="flex items-center gap-1 flex-1">
              <div className="flex-1 h-2 rounded-full bg-amber/20 overflow-hidden">
                <div
                  className="h-full rounded-full bg-amber animate-pulse"
                  style={{
                    animationDelay: `${i * 200}ms`,
                    animationDuration: '2s',
                  }}
                />
              </div>
              {i < stats.peer_count - 1 && (
                <ArrowRightLeft className="w-3 h-3 text-text-muted flex-shrink-0" />
              )}
            </div>
          ))}
        </div>
        <div className="flex justify-between mt-1">
          <span className="text-[10px] text-text-muted">L0</span>
          <span className="text-[10px] text-text-muted">L{stats.total_layers}</span>
        </div>
      </div>
    </div>
  );
}

interface MetricCardProps {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: string;
  detail: string;
  highlight?: boolean;
}

function MetricCard({ icon: Icon, label, value, detail, highlight }: MetricCardProps) {
  return (
    <div className={`border rounded-lg p-3 ${highlight ? 'bg-amber/5 border-amber/20' : 'bg-bg-tertiary border-border'}`}>
      <div className="flex items-center gap-1.5 mb-2">
        <Icon className={`w-3.5 h-3.5 ${highlight ? 'text-amber' : 'text-text-muted'}`} />
        <span className="text-[10px] text-text-muted uppercase tracking-wider">{label}</span>
      </div>
      <div className={`text-lg font-bold ${highlight ? 'text-amber' : 'text-text-primary'}`}>{value}</div>
      <div className="text-[10px] text-text-muted mt-0.5">{detail}</div>
    </div>
  );
}

function formatBytes(bytes: number): string {
  if (bytes >= 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${bytes} B`;
}

function formatCount(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}
