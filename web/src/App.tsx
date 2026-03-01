import { useState } from 'react';
import { Sidebar } from './components/Sidebar';
import { Header } from './components/Header';
import { PeersPanel } from './components/PeersPanel';
import { ResourceMonitor } from './components/ResourceMonitor';
import { ChatPlayground } from './components/ChatPlayground';
import { LayerMap } from './components/LayerMap';
import { DistributedPanel } from './components/DistributedPanel';
import { useRoomStatus } from './hooks/useRoomStatus';
import { leaveRoom, stopRoom } from './lib/api';
import type { RoomStatus } from './types';
import { Loader2, WifiOff } from 'lucide-react';

type Tab = 'dashboard' | 'chat' | 'room';

export default function App() {
  const [activeTab, setActiveTab] = useState<Tab>('dashboard');
  const { status, loading, refetch } = useRoomStatus();

  if (loading) {
    return <LoadingScreen />;
  }

  if (!status) {
    return <NoRoomScreen />;
  }

  return (
    <div className="flex h-screen overflow-hidden">
      <Sidebar
        status={status}
        activeTab={activeTab}
        onTabChange={setActiveTab}
      />

      <main className="flex-1 flex flex-col overflow-hidden">
        <Header status={status} />

        <div className="flex-1 overflow-y-auto p-6">
          {activeTab === 'dashboard' && (
            <div className="space-y-6 max-w-7xl">
              <ResourceMonitor status={status} />
              {status.distributed?.is_distributed && (
                <DistributedPanel stats={status.distributed} />
              )}
              <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
                <PeersPanel peers={status.room.peers} />
                <LayerMap
                  peers={status.room.peers}
                  totalLayers={status.room.total_layers}
                />
              </div>
            </div>
          )}

          {activeTab === 'chat' && (
            <div className="max-w-4xl mx-auto h-full">
              <ChatPlayground modelId={status.room.model_id} />
            </div>
          )}

          {activeTab === 'room' && (
            <div className="max-w-2xl space-y-6">
              <RoomInfo status={status} onRefetch={refetch} />
            </div>
          )}
        </div>
      </main>
    </div>
  );
}

function LoadingScreen() {
  return (
    <div className="flex items-center justify-center h-screen bg-bg-primary">
      <div className="text-center">
        <Loader2 className="w-8 h-8 text-amber animate-spin mx-auto mb-4" />
        <p className="text-text-secondary text-sm">Connecting to HiveMind...</p>
      </div>
    </div>
  );
}

function NoRoomScreen() {
  return (
    <div className="flex items-center justify-center h-screen bg-bg-primary">
      <div className="text-center max-w-md">
        <WifiOff className="w-12 h-12 text-text-muted mx-auto mb-4" />
        <h2 className="text-xl font-bold text-text-primary mb-2">No Active Room</h2>
        <p className="text-text-secondary text-sm mb-6">
          Create or join a room to start distributed inference.
        </p>
        <div className="bg-bg-secondary border border-border rounded-xl p-4 text-left space-y-3">
          <div>
            <p className="text-text-muted text-xs mb-1">Create a room:</p>
            <code className="text-amber text-sm">hivemind create --model meta-llama/Llama-3-70B</code>
          </div>
          <div>
            <p className="text-text-muted text-xs mb-1">Join a room:</p>
            <code className="text-amber text-sm">hivemind join &lt;invite-code&gt;</code>
          </div>
        </div>
      </div>
    </div>
  );
}

function RoomInfo({ status, onRefetch }: { status: RoomStatus; onRefetch: () => void }) {
  const { room } = status;

  const handleLeave = async () => {
    if (await leaveRoom()) onRefetch();
  };

  const handleStop = async () => {
    if (await stopRoom()) onRefetch();
  };

  return (
    <div className="space-y-6">
      <div className="bg-bg-secondary border border-border rounded-xl p-6">
        <h2 className="text-lg font-bold text-amber mb-4">Room Information</h2>
        <div className="grid grid-cols-2 gap-4">
          <InfoRow label="Room ID" value={room.id} />
          <InfoRow label="Model" value={room.model_id} />
          <InfoRow label="Type" value={room.model_type.toUpperCase()} />
          <InfoRow label="State" value={room.state} />
          <InfoRow label="Total Layers" value={String(room.total_layers)} />
          <InfoRow label="Max Peers" value={String(room.max_peers)} />
          <InfoRow label="Uptime" value={status.uptime} />
          <InfoRow label="Speed" value={`${status.tokens_per_sec} tok/s`} />
        </div>
      </div>

      <div className="bg-bg-secondary border border-border rounded-xl p-6">
        <h2 className="text-lg font-bold text-amber mb-4">Invite Code</h2>
        <div className="flex items-center gap-4">
          <code className="bg-amber text-black px-4 py-2 rounded-lg font-bold text-lg tracking-wider">
            {room.invite_code}
          </code>
          <button
            onClick={() => navigator.clipboard.writeText(room.invite_code)}
            className="px-4 py-2 bg-bg-tertiary border border-border rounded-lg text-text-secondary hover:text-text-primary hover:border-amber transition-colors text-sm"
          >
            Copy
          </button>
        </div>
        <p className="text-text-muted text-sm mt-3">
          Share this code with peers: <code className="text-amber">hivemind join {room.invite_code}</code>
        </p>
      </div>

      <div className="flex gap-3">
        <button
          onClick={handleLeave}
          className="px-6 py-2 bg-red/10 border border-red/30 text-red rounded-lg hover:bg-red/20 transition-colors text-sm font-medium"
        >
          Leave Room
        </button>
        <button
          onClick={handleStop}
          className="px-6 py-2 bg-red/10 border border-red/30 text-red rounded-lg hover:bg-red/20 transition-colors text-sm font-medium"
        >
          Stop Room
        </button>
      </div>
    </div>
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <span className="text-text-muted text-xs block">{label}</span>
      <span className="text-text-primary font-medium text-sm">{value}</span>
    </div>
  );
}
