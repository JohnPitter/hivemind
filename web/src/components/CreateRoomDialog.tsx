import { useState, useEffect } from 'react';
import { X, Loader2, Cpu, Layers, HardDrive, ChevronDown } from 'lucide-react';
import { fetchCatalog, createRoom } from '../lib/api';
import type { CatalogModel } from '../types';

interface CreateRoomDialogProps {
  open: boolean;
  onClose: () => void;
  onCreated: () => void;
}

const TYPE_LABELS: Record<string, string> = {
  llm: 'LLM',
  code: 'Code',
  diffusion: 'Diffusion',
  embedding: 'Embedding',
  multimodal: 'Multimodal',
};

function formatVRAM(mb: number): string {
  return mb >= 1024 ? `${(mb / 1024).toFixed(0)} GB` : `${mb} MB`;
}

export function CreateRoomDialog({ open, onClose, onCreated }: CreateRoomDialogProps) {
  const [models, setModels] = useState<CatalogModel[]>([]);
  const [selectedId, setSelectedId] = useState('');
  const [maxPeers, setMaxPeers] = useState(4);
  const [loading, setLoading] = useState(false);
  const [catalogLoading, setCatalogLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!open) return;
    setCatalogLoading(true);
    setError('');
    fetchCatalog().then((data) => {
      setModels(data);
      if (data.length > 0 && !selectedId) {
        setSelectedId(data[0].id);
      }
      setCatalogLoading(false);
    });
  }, [open]);

  const selected = models.find((m) => m.id === selectedId);

  const handleSubmit = async () => {
    if (!selectedId) return;
    setLoading(true);
    setError('');
    const result = await createRoom(selectedId, maxPeers);
    setLoading(false);
    if (result.ok) {
      onCreated();
      onClose();
    } else {
      setError(result.error ?? 'Failed to create room');
    }
  };

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div className="relative bg-bg-secondary border border-border rounded-xl w-full max-w-lg mx-4 shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between p-5 border-b border-border">
          <h2 className="text-lg font-bold text-text-primary">Create Room</h2>
          <button
            onClick={onClose}
            className="text-text-muted hover:text-text-primary transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Body */}
        <div className="p-5 space-y-5">
          {/* Model Selector */}
          <div>
            <label className="block text-xs text-text-muted mb-2">Model</label>
            {catalogLoading ? (
              <div className="flex items-center gap-2 text-text-secondary text-sm py-3">
                <Loader2 className="w-4 h-4 animate-spin" />
                Loading catalog...
              </div>
            ) : (
              <div className="relative">
                <select
                  value={selectedId}
                  onChange={(e) => setSelectedId(e.target.value)}
                  className="w-full bg-bg-tertiary border border-border rounded-lg px-3 py-2.5 text-sm text-text-primary appearance-none cursor-pointer focus:outline-none focus:border-amber transition-colors"
                >
                  {models.map((m) => (
                    <option key={m.id} value={m.id}>
                      {m.name} ({m.parameter_size}) — {TYPE_LABELS[m.type] ?? m.type}
                    </option>
                  ))}
                </select>
                <ChevronDown className="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 text-text-muted pointer-events-none" />
              </div>
            )}
          </div>

          {/* Model Info Card */}
          {selected && (
            <div className="bg-bg-tertiary border border-border rounded-lg p-4">
              <div className="flex items-center gap-2 mb-3">
                <Cpu className="w-4 h-4 text-amber" />
                <span className="text-sm font-medium text-text-primary">{selected.name}</span>
                <span className="text-[10px] px-1.5 py-0.5 rounded-full border border-amber/20 bg-amber/10 text-amber">
                  {TYPE_LABELS[selected.type] ?? selected.type}
                </span>
              </div>
              <div className="grid grid-cols-3 gap-3 text-xs">
                <div className="flex items-center gap-1.5">
                  <Layers className="w-3.5 h-3.5 text-text-muted" />
                  <div>
                    <span className="text-text-muted block">Layers</span>
                    <span className="text-text-primary font-medium">{selected.total_layers}</span>
                  </div>
                </div>
                <div className="flex items-center gap-1.5">
                  <HardDrive className="w-3.5 h-3.5 text-text-muted" />
                  <div>
                    <span className="text-text-muted block">Min VRAM</span>
                    <span className="text-text-primary font-medium">{formatVRAM(selected.min_vram_mb)}</span>
                  </div>
                </div>
                <div className="flex items-center gap-1.5">
                  <HardDrive className="w-3.5 h-3.5 text-text-muted" />
                  <div>
                    <span className="text-text-muted block">Recommended</span>
                    <span className="text-text-primary font-medium">{formatVRAM(selected.recommended_vram_mb)}</span>
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* Max Peers */}
          <div>
            <label className="block text-xs text-text-muted mb-2">Max Peers</label>
            <input
              type="number"
              min={1}
              max={32}
              value={maxPeers}
              onChange={(e) => setMaxPeers(Math.max(1, Math.min(32, Number(e.target.value))))}
              className="w-full bg-bg-tertiary border border-border rounded-lg px-3 py-2.5 text-sm text-text-primary focus:outline-none focus:border-amber transition-colors"
            />
            <p className="text-[11px] text-text-muted mt-1">
              Maximum number of peers that can join this room (1-32).
            </p>
          </div>

          {/* Error */}
          {error && (
            <div className="bg-red/10 border border-red/20 rounded-lg px-4 py-2.5 text-sm text-red">
              {error}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-3 p-5 border-t border-border">
          <button
            onClick={onClose}
            disabled={loading}
            className="px-4 py-2 text-sm text-text-secondary hover:text-text-primary transition-colors rounded-lg"
          >
            Cancel
          </button>
          <button
            onClick={handleSubmit}
            disabled={loading || !selectedId || catalogLoading}
            className="px-5 py-2 bg-amber text-black text-sm font-medium rounded-lg hover:bg-amber-light transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
          >
            {loading && <Loader2 className="w-4 h-4 animate-spin" />}
            Create Room
          </button>
        </div>
      </div>
    </div>
  );
}
