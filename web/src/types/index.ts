export type RoomState = 'creating' | 'active' | 'paused' | 'closed';
export type PeerState = 'connecting' | 'syncing' | 'ready' | 'offline';
export type ModelType = 'llm' | 'diffusion';

export interface ResourceSpec {
  gpu_name: string;
  vram_total_mb: number;
  vram_free_mb: number;
  ram_total_mb: number;
  ram_free_mb: number;
  cuda_available: boolean;
  platform: string;
}

export interface Peer {
  id: string;
  name: string;
  ip: string;
  state: PeerState;
  layers: number[];
  resources: ResourceSpec;
  latency_ms: number;
  joined_at: string;
  is_host: boolean;
}

export interface Room {
  id: string;
  invite_code: string;
  model_id: string;
  model_type: ModelType;
  state: RoomState;
  host_id: string;
  max_peers: number;
  total_layers: number;
  created_at: string;
  peers: Peer[];
}

export interface DistributedStats {
  peer_count: number;
  total_layers: number;
  avg_latency_ms: number;
  tensor_transfers: number;
  bytes_transferred: number;
  compression_ratio: number;
  forward_pass_avg_ms: number;
  is_distributed: boolean;
  tokens_generated: number;
  tokens_per_second: number;
  avg_token_latency_ms: number;
  embed_avg_ms: number;
  sample_avg_ms: number;
  generation_requests: number;
}

export interface RoomStatus {
  room: Room;
  total_vram_mb: number;
  used_vram_mb: number;
  tokens_per_sec: number;
  uptime: string;
  distributed?: DistributedStats;
}

export interface ChatMessage {
  role: 'user' | 'assistant' | 'system';
  content: string;
}

export interface HealthStatus {
  status: 'ok' | 'degraded' | 'error';
  worker_healthy: boolean;
  peers_connected: number;
  model_loaded: boolean;
}

export interface CatalogModel {
  id: string;
  name: string;
  type: string;
  parameter_size: string;
  total_layers: number;
  min_vram_mb: number;
  recommended_vram_mb: number;
  min_peer_vram_mb: number;
}
