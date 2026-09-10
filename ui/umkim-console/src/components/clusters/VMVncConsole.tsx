import { useEffect, useRef } from 'react';
import RFB from '@novnc/novnc/lib/rfb.js';
import { fleetClusterVMConsoleWSURL } from '../../api/client';

type Props = {
  clusterName: string;
  namespace: string;
  name: string;
  open: boolean;
};

export function VMVncConsole({ clusterName, namespace, name, open }: Props) {
  const containerRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open || !containerRef.current) {
      return;
    }
    const url = fleetClusterVMConsoleWSURL(clusterName, namespace, name, 'vnc');
    const rfb = new RFB(containerRef.current, url, {
      wsProtocols: ['vnc.kubevirt.io'],
    });
    rfb.scaleViewport = true;
    rfb.resizeSession = true;
    return () => {
      try {
        rfb.disconnect();
      } catch {
        /* ignore */
      }
    };
  }, [open, clusterName, namespace, name]);

  if (!open) {
    return null;
  }
  return (
    <div
      ref={containerRef}
      data-testid="vm-vnc-console"
      style={{ height: '480px', width: '100%', background: '#111', border: '1px solid #444' }}
    />
  );
}
