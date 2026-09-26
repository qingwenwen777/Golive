import { useCallback, useEffect, useRef, useState } from 'react';

export type ReadyState = 'connecting' | 'open' | 'closing' | 'closed' | 'reconnecting';

export interface UseWebSocketOptions {
  onOpen?: (ev: Event) => void;
  onClose?: (ev: CloseEvent) => void;
  onError?: (ev: Event) => void;
  onMessage?: (ev: MessageEvent) => void;
  heartbeatMs?: number;
  reconnect?: boolean;
  maxRetries?: number;
  // Defaults to the gateway's protocol; the token is offered after these.
  protocols?: string | string[];
  getToken?: () => string | null | undefined;
  // When provided, a change in token triggers a reconnect with the new token.
  token?: string | null;
}

export interface UseWebSocketReturn {
  sendMessage: (data: unknown) => boolean;
  lastMessage: MessageEvent | null;
  readyState: ReadyState;
  retryCount: number;
  connect: () => void;
  disconnect: (code?: number, reason?: string) => void;
}

const BACKOFF_STEPS_MS = [1000, 2000, 4000, 8000, 16000, 30000];
const SERVER_SILENCE_WATCHDOG_MS = 45000;

function backoffFor(attempt: number): number {
  const idx = Math.min(attempt, BACKOFF_STEPS_MS.length - 1);
  return BACKOFF_STEPS_MS[idx]!;
}

// The gateway's protocol. The access token is offered next to it as the
// subprotocol `auth.<token>`: the WebSocket API can't set request headers, and
// a token in the URL ends up in access logs. The gateway only ever selects
// GATEWAY_PROTOCOL, so the token is never echoed back.
const GATEWAY_PROTOCOL = 'golive.v1';
const AUTH_PROTOCOL_PREFIX = 'auth.';

function handshakeProtocols(
  protocols: string | string[],
  token: string | null | undefined,
): string[] {
  const offered = typeof protocols === 'string' ? [protocols] : protocols;
  return token ? [...offered, `${AUTH_PROTOCOL_PREFIX}${token}`] : offered;
}

export function useWebSocket(
  url: string | null,
  opts: UseWebSocketOptions = {},
): UseWebSocketReturn {
  const {
    onOpen,
    onClose,
    onError,
    onMessage,
    heartbeatMs = 20000,
    reconnect = true,
    maxRetries = 10,
    protocols = GATEWAY_PROTOCOL,
    getToken,
    token,
  } = opts;

  const [readyState, setReadyState] = useState<ReadyState>('closed');
  const [lastMessage, setLastMessage] = useState<MessageEvent | null>(null);
  const [retryCount, setRetryCount] = useState(0);

  const wsRef = useRef<WebSocket | null>(null);
  const heartbeatTimerRef = useRef<number | null>(null);
  const watchdogTimerRef = useRef<number | null>(null);
  const reconnectTimerRef = useRef<number | null>(null);
  const retryCountRef = useRef(0);
  const manualCloseRef = useRef(false);
  const lastServerTsRef = useRef<number>(0);

  // Keep handler refs so we don't reconnect when they change.
  const handlersRef = useRef({ onOpen, onClose, onError, onMessage });
  handlersRef.current = { onOpen, onClose, onError, onMessage };

  const clearHeartbeat = () => {
    if (heartbeatTimerRef.current != null) {
      window.clearInterval(heartbeatTimerRef.current);
      heartbeatTimerRef.current = null;
    }
    if (watchdogTimerRef.current != null) {
      window.clearInterval(watchdogTimerRef.current);
      watchdogTimerRef.current = null;
    }
  };

  const clearReconnect = () => {
    if (reconnectTimerRef.current != null) {
      window.clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
  };

  const startHeartbeat = useCallback(() => {
    clearHeartbeat();
    if (heartbeatMs > 0) {
      heartbeatTimerRef.current = window.setInterval(() => {
        const ws = wsRef.current;
        if (ws && ws.readyState === WebSocket.OPEN) {
          try {
            ws.send(JSON.stringify({ type: 'heartbeat', ts: Date.now() }));
          } catch {
            /* noop */
          }
        }
      }, heartbeatMs);
    }
    // Server-silence watchdog.
    lastServerTsRef.current = Date.now();
    watchdogTimerRef.current = window.setInterval(() => {
      if (Date.now() - lastServerTsRef.current > SERVER_SILENCE_WATCHDOG_MS) {
        const ws = wsRef.current;
        if (ws && ws.readyState === WebSocket.OPEN) {
          try {
            ws.close(4000, 'heartbeat-timeout');
          } catch {
            /* noop */
          }
        }
      }
    }, 5000);
  }, [heartbeatMs]);

  const open = useCallback(() => {
    if (!url) return;
    clearReconnect();
    manualCloseRef.current = false;

    const tok = token !== undefined ? token : getToken ? getToken() : null;

    try {
      const ws = new WebSocket(url, handshakeProtocols(protocols, tok));
      wsRef.current = ws;
      setReadyState('connecting');

      ws.onopen = (ev) => {
        if (wsRef.current !== ws) return;
        retryCountRef.current = 0;
        setRetryCount(0);
        setReadyState('open');
        startHeartbeat();
        handlersRef.current.onOpen?.(ev);
      };

      ws.onmessage = (ev) => {
        if (wsRef.current !== ws) return;
        lastServerTsRef.current = Date.now();
        setLastMessage(ev);
        handlersRef.current.onMessage?.(ev);
      };

      ws.onerror = (ev) => {
        if (wsRef.current !== ws) return;
        handlersRef.current.onError?.(ev);
      };

      ws.onclose = (ev) => {
        // Stale socket (replaced by a newer connection after url/token change):
        // do not touch shared state and do not schedule a reconnect.
        if (wsRef.current !== ws) {
          handlersRef.current.onClose?.(ev);
          return;
        }
        clearHeartbeat();
        wsRef.current = null;
        handlersRef.current.onClose?.(ev);

        if (manualCloseRef.current || !reconnect) {
          setReadyState('closed');
          return;
        }
        if (retryCountRef.current >= maxRetries) {
          setReadyState('closed');
          return;
        }
        const delay = backoffFor(retryCountRef.current);
        retryCountRef.current += 1;
        setRetryCount(retryCountRef.current);
        setReadyState('reconnecting');
        reconnectTimerRef.current = window.setTimeout(() => {
          open();
        }, delay);
      };
    } catch {
      setReadyState('closed');
    }
  }, [url, protocols, reconnect, maxRetries, startHeartbeat, getToken, token]);

  const connect = useCallback(() => {
    retryCountRef.current = 0;
    setRetryCount(0);
    open();
  }, [open]);

  const disconnect = useCallback((code = 1000, reason = 'client-disconnect') => {
    manualCloseRef.current = true;
    clearReconnect();
    clearHeartbeat();
    const ws = wsRef.current;
    if (ws) {
      setReadyState('closing');
      try {
        ws.close(code, reason);
      } catch {
        /* noop */
      }
    } else {
      setReadyState('closed');
    }
  }, []);

  const sendMessage = useCallback((data: unknown): boolean => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return false;
    try {
      const payload = typeof data === 'string' ? data : JSON.stringify(data);
      ws.send(payload);
      return true;
    } catch {
      return false;
    }
  }, []);

  useEffect(() => {
    if (!url) {
      setReadyState('closed');
      return;
    }
    open();
    return () => {
      manualCloseRef.current = true;
      clearReconnect();
      clearHeartbeat();
      const ws = wsRef.current;
      wsRef.current = null;
      if (ws) {
        try {
          ws.close(1000, 'unmount');
        } catch {
          /* noop */
        }
      }
    };
    // Reconnect when url or token changes (token change requires a fresh
    // handshake so the gateway upgrades the connection from anonymous to
    // authenticated).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [url, token]);

  return { sendMessage, lastMessage, readyState, retryCount, connect, disconnect };
}
