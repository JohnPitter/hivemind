import { useState, useEffect, useCallback } from 'react';
import type { RoomStatus } from '../types';
import { fetchRoomStatus } from '../lib/api';

const POLL_INTERVAL = 5000;

export function useRoomStatus() {
  const [status, setStatus] = useState<RoomStatus | null>(null);
  const [loading, setLoading] = useState(true);

  const refetch = useCallback(async () => {
    const data = await fetchRoomStatus();
    setStatus(data);
    setLoading(false);
  }, []);

  useEffect(() => {
    refetch();
    const interval = setInterval(refetch, POLL_INTERVAL);
    return () => clearInterval(interval);
  }, [refetch]);

  return { status, loading, refetch };
}
