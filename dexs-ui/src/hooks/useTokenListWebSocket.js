import { useEffect, useRef, useState, useCallback } from 'react';

const CHAIN_ID = 100000;

const buildMessageWebSocketUrl = () => {
  const configuredUrl = process.env.REACT_APP_MESSAGE_WS_URL;
  if (configuredUrl) {
    if (configuredUrl.includes('chain_id=')) {
      return configuredUrl;
    }
    const separator = configuredUrl.includes('?') ? '&' : '?';
    return `${configuredUrl}${separator}chain_id=${CHAIN_ID}`;
  }

  if (typeof window === 'undefined') {
    return `ws://localhost:8110/ws/tokens?chain_id=${CHAIN_ID}`;
  }

  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
  const hostname = window.location.hostname || 'localhost';
  const port = process.env.REACT_APP_MESSAGE_WS_PORT || '8110';

  return `${protocol}://${hostname}:${port}/ws/tokens?chain_id=${CHAIN_ID}`;
};

const useTokenListWebSocket = (onNewToken, onTokenUpdate) => {
  const [connectionStatus, setConnectionStatus] = useState('disconnected');
  const wsRef = useRef(null);
  const reconnectTimeoutRef = useRef(null);
  const reconnectCount = useRef(0);

  const connect = useCallback(() => {
    try {
      const wsUrl = buildMessageWebSocketUrl();
      console.log('🔌 [TOKEN WS] Connecting to token list WebSocket:', wsUrl);
      
      const ws = new WebSocket(wsUrl);

      ws.onopen = () => {
        console.log('✅ [TOKEN WS] Token list WebSocket connected');
        setConnectionStatus('connected');
        reconnectCount.current = 0;
        
        const subscribeMsg = {
          type: 'subscribe',
          data: {
            chain_id: CHAIN_ID,
            categories: ['new_creation', 'completing', 'completed']
          }
        };
        ws.send(JSON.stringify(subscribeMsg));
      };

      ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          console.log('📨 [TOKEN WS] Received message:', data);

          switch (data.type) {
            case 'new_token':
              if (data.data && onNewToken) {
                console.log('🆕 [TOKEN WS] New token created:', data.data.token_name || data.data.tokenName);
                const tokenData = {
                  tokenAddress: data.data.token_address,
                  pairAddress: data.data.pair_address,
                  tokenName: data.data.token_name,
                  tokenSymbol: data.data.token_symbol,
                  tokenIcon: data.data.token_icon,
                  launchTime: data.data.launch_time,
                  mktCap: data.data.mkt_cap,
                  holdCount: data.data.hold_count,
                  change24: data.data.change_24,
                  txs24h: data.data.txs_24h,
                  pumpStatus: data.data.pump_status,
                  chainId: data.data.chain_id
                };
                onNewToken(tokenData);
              }
              break;
              
            case 'token_status_update':
              if (data.data && onTokenUpdate) {
                console.log('🔄 [TOKEN WS] Token status updated:', data.data.token_name || data.data.tokenName);
                const updateData = {
                  tokenAddress: data.data.token_address,
                  pairAddress: data.data.pair_address,
                  tokenName: data.data.token_name,
                  tokenSymbol: data.data.token_symbol,
                  tokenIcon: data.data.token_icon,
                  launchTime: data.data.launch_time,
                  mktCap: data.data.mkt_cap,
                  holdCount: data.data.hold_count,
                  change24: data.data.change_24,
                  txs24h: data.data.txs_24h,
                  pumpStatus: data.data.pump_status,
                  chainId: data.data.chain_id
                };
                onTokenUpdate(updateData);
              }
              break;
              
            case 'connection_established':
              console.log('🎯 [TOKEN WS] Connection established:', data.data);
              break;
              
            default:
              console.log('🔍 [TOKEN WS] Unknown message type:', data.type);
          }
        } catch (error) {
          console.error('❌ [TOKEN WS] Error parsing message:', error, event.data);
        }
      };

      ws.onerror = (error) => {
        console.error('❌ [TOKEN WS] WebSocket error:', error);
        setConnectionStatus('error');
      };

      ws.onclose = (event) => {
        console.log('🔌 [TOKEN WS] WebSocket closed:', event.code, event.reason);
        setConnectionStatus('disconnected');
        wsRef.current = null;

        if (!event.wasClean) {
          const delay = Math.min(1000 * Math.pow(2, reconnectCount.current), 30000);
          console.log(`🔄 [TOKEN WS] Reconnecting in ${delay}ms (attempt ${reconnectCount.current + 1})`);
          
          reconnectTimeoutRef.current = setTimeout(() => {
            reconnectCount.current++;
            connect();
          }, delay);
        }
      };

      wsRef.current = ws;
    } catch (error) {
      console.error('❌ [TOKEN WS] Error creating WebSocket connection:', error);
      setConnectionStatus('error');
    }
  }, [onNewToken, onTokenUpdate]);

  const disconnect = useCallback(() => {
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }

    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      console.log('🛑 [TOKEN WS] Manually closing WebSocket connection');
      wsRef.current.close(1000, 'Manual disconnect');
    }
  }, []);

  useEffect(() => {
    connect();

    return () => {
      disconnect();
    };
  }, [connect, disconnect]);

  return {
    connectionStatus,
    disconnect,
    reconnect: connect
  };
};

export default useTokenListWebSocket;
