import { useState } from 'react';
import { X, Loader2 } from 'lucide-react';
import { joinRoom } from '../lib/api';

interface JoinRoomDialogProps {
  open: boolean;
  onClose: () => void;
  onJoined: () => void;
}

export function JoinRoomDialog({ open, onClose, onJoined }: JoinRoomDialogProps) {
  const [inviteCode, setInviteCode] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const handleSubmit = async () => {
    const code = inviteCode.trim();
    if (!code) return;
    setLoading(true);
    setError('');
    const result = await joinRoom(code);
    setLoading(false);
    if (result.ok) {
      setInviteCode('');
      onJoined();
      onClose();
    } else {
      setError(result.error ?? 'Failed to join room');
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && inviteCode.trim() && !loading) {
      handleSubmit();
    }
  };

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/60" onClick={onClose} />
      <div className="relative bg-bg-secondary border border-border rounded-xl w-full max-w-md mx-4 shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between p-5 border-b border-border">
          <h2 className="text-lg font-bold text-text-primary">Join Room</h2>
          <button
            onClick={onClose}
            className="text-text-muted hover:text-text-primary transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Body */}
        <div className="p-5 space-y-5">
          <div>
            <label className="block text-xs text-text-muted mb-2">Invite Code</label>
            <input
              type="text"
              value={inviteCode}
              onChange={(e) => setInviteCode(e.target.value.toUpperCase())}
              onKeyDown={handleKeyDown}
              placeholder="Enter invite code"
              autoFocus
              className="w-full bg-bg-tertiary border border-border rounded-lg px-3 py-2.5 text-sm text-text-primary placeholder:text-text-muted focus:outline-none focus:border-amber transition-colors tracking-widest font-mono text-center text-lg"
            />
            <p className="text-[11px] text-text-muted mt-2">
              Enter the invite code shared by the room host.
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
            disabled={loading || !inviteCode.trim()}
            className="px-5 py-2 bg-amber text-black text-sm font-medium rounded-lg hover:bg-amber-light transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
          >
            {loading && <Loader2 className="w-4 h-4 animate-spin" />}
            Join Room
          </button>
        </div>
      </div>
    </div>
  );
}
