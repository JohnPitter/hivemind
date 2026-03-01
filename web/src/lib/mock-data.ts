import type { RoomStatus, HealthStatus } from '../types';

export const mockRoomStatus: RoomStatus = {
  room: {
    id: 'a1b2c3d4e5f6',
    invite_code: 'HIVE-X7K9M2',
    model_id: 'meta-llama/Llama-3-70B',
    model_type: 'llm',
    state: 'active',
    host_id: 'peer-001',
    max_peers: 5,
    total_layers: 80,
    created_at: new Date(Date.now() - 45 * 60 * 1000).toISOString(),
    peers: [
      {
        id: 'peer-001',
        name: 'host-node',
        ip: '10.0.0.1',
        state: 'ready',
        layers: Array.from({ length: 28 }, (_, i) => i),
        resources: {
          gpu_name: 'NVIDIA RTX 4090',
          vram_total_mb: 24576,
          vram_free_mb: 2048,
          ram_total_mb: 65536,
          ram_free_mb: 32768,
          cuda_available: true,
          platform: 'Linux',
        },
        latency_ms: 0,
        joined_at: new Date(Date.now() - 45 * 60 * 1000).toISOString(),
        is_host: true,
      },
      {
        id: 'peer-002',
        name: 'worker-alpha',
        ip: '10.0.0.2',
        state: 'ready',
        layers: Array.from({ length: 28 }, (_, i) => i + 28),
        resources: {
          gpu_name: 'NVIDIA RTX 3080',
          vram_total_mb: 10240,
          vram_free_mb: 1024,
          ram_total_mb: 32768,
          ram_free_mb: 16384,
          cuda_available: true,
          platform: 'Windows',
        },
        latency_ms: 45.2,
        joined_at: new Date(Date.now() - 30 * 60 * 1000).toISOString(),
        is_host: false,
      },
      {
        id: 'peer-003',
        name: 'worker-beta',
        ip: '10.0.0.3',
        state: 'ready',
        layers: Array.from({ length: 24 }, (_, i) => i + 56),
        resources: {
          gpu_name: 'NVIDIA RTX 3060',
          vram_total_mb: 12288,
          vram_free_mb: 2048,
          ram_total_mb: 32768,
          ram_free_mb: 20480,
          cuda_available: true,
          platform: 'Windows',
        },
        latency_ms: 72.8,
        joined_at: new Date(Date.now() - 15 * 60 * 1000).toISOString(),
        is_host: false,
      },
    ],
  },
  total_vram_mb: 47104,
  used_vram_mb: 41984,
  tokens_per_sec: 14.7,
  uptime: '45m12s',
  distributed: {
    peer_count: 3,
    total_layers: 80,
    avg_latency_ms: 59.0,
    tensor_transfers: 1247,
    bytes_transferred: 8_432_910_336,
    compression_ratio: 0.62,
    forward_pass_avg_ms: 142.5,
    is_distributed: true,
    tokens_generated: 3842,
    tokens_per_second: 14.7,
    avg_token_latency_ms: 68.0,
    embed_avg_ms: 12.3,
    sample_avg_ms: 5.1,
    generation_requests: 47,
  },
};

export const mockHealth: HealthStatus = {
  status: 'ok',
  worker_healthy: true,
  peers_connected: 3,
  model_loaded: true,
};

export const mockStreamResponse = (_input: string): string[] => {
  const responses = [
    'This response is being generated through distributed tensor parallelism across 3 nodes in the HiveMind network.',
    'Each node processes its assigned model layers and passes activation tensors to the next node via the WireGuard mesh.',
    'The cooperative approach means no single machine needs the full model in memory.',
    `Processing your input across the distributed network. The model layers are split between ${mockRoomStatus.room.peers.length} peers.`,
  ];
  const response = responses[Math.floor(Math.random() * responses.length)];
  return response.split(' ');
};
