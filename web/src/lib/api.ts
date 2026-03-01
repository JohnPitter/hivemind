import type { RoomStatus, HealthStatus, CatalogModel } from '../types';

const API_BASE = '';

export async function fetchRoomStatus(): Promise<RoomStatus | null> {
  try {
    const res = await fetch(`${API_BASE}/api/room/status`);
    if (!res.ok) return null;
    return await res.json();
  } catch {
    return null;
  }
}

export async function fetchHealth(): Promise<HealthStatus | null> {
  try {
    const res = await fetch(`${API_BASE}/api/health`);
    if (!res.ok) return null;
    return await res.json();
  } catch {
    return null;
  }
}

export async function leaveRoom(): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE}/room/leave`, { method: 'DELETE' });
    return res.ok;
  } catch {
    return false;
  }
}

export async function stopRoom(): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE}/room/leave`, { method: 'DELETE' });
    return res.ok;
  } catch {
    return false;
  }
}

export interface ChatMessage {
  role: 'user' | 'assistant' | 'system';
  content: string;
}

export async function fetchCatalog(): Promise<CatalogModel[]> {
  try {
    const res = await fetch(`${API_BASE}/api/catalog`);
    if (!res.ok) return [];
    const data = await res.json();
    return data.models ?? [];
  } catch {
    return [];
  }
}

export async function createRoom(modelId: string, maxPeers: number): Promise<{ ok: boolean; error?: string }> {
  try {
    const res = await fetch(`${API_BASE}/api/room/create`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ model_id: modelId, max_peers: maxPeers }),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({ message: 'Request failed' }));
      return { ok: false, error: err.message ?? `Error ${res.status}` };
    }
    return { ok: true };
  } catch {
    return { ok: false, error: 'Network error' };
  }
}

export async function joinRoom(inviteCode: string): Promise<{ ok: boolean; error?: string }> {
  try {
    const res = await fetch(`${API_BASE}/api/room/join`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ invite_code: inviteCode, resources: {} }),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({ message: 'Request failed' }));
      return { ok: false, error: err.message ?? `Error ${res.status}` };
    }
    return { ok: true };
  } catch {
    return { ok: false, error: 'Network error' };
  }
}

export async function* chatCompletionStream(
  model: string,
  messages: ChatMessage[],
): AsyncGenerator<string> {
  const res = await fetch(`${API_BASE}/v1/chat/completions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ model, messages, stream: true }),
  });

  if (!res.ok || !res.body) {
    throw new Error(`Chat request failed: ${res.status}`);
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split('\n');
    buffer = lines.pop() ?? '';

    for (const line of lines) {
      if (!line.startsWith('data: ')) continue;
      const payload = line.slice(6).trim();
      if (payload === '[DONE]') return;
      try {
        const chunk = JSON.parse(payload);
        const delta = chunk.choices?.[0]?.delta?.content;
        if (delta) yield delta;
      } catch {
        // skip malformed chunks
      }
    }
  }
}
