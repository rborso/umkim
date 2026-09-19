import { useEffect, useRef } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';
import { fleetClusterVMConsoleWSURL } from '../../api/client';

type Props = {
  clusterName: string;
  namespace: string;
  name: string;
  active: boolean;
};

export function VMSerialConsole({ clusterName, namespace, name, active }: Props) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const termRef = useRef<Terminal | null>(null);

  useEffect(() => {
    if (!active || !containerRef.current) {
      return;
    }
    const term = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      theme: { background: '#1e1e1e' },
      convertEol: true,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(containerRef.current);
    fit.fit();
    termRef.current = term;
    term.writeln(`Connecting serial console ${namespace}/${name}…`);

    const ws = new WebSocket(
      fleetClusterVMConsoleWSURL(clusterName, namespace, name, 'serial'),
      ['plain.kubevirt.io'],
    );
    ws.binaryType = 'arraybuffer';
    ws.onopen = () => {
      term.writeln('Connected. Click here, then type (login: rocky / rocky).');
      term.focus();
    };
    ws.onmessage = (ev) => {
      if (typeof ev.data === 'string') {
        term.write(ev.data);
        return;
      }
      term.write(new Uint8Array(ev.data as ArrayBuffer));
    };
    ws.onerror = () => term.writeln('\r\n[console error]');
    ws.onclose = () => term.writeln('\r\n[disconnected]');
    const disp = term.onData((data) => {
      if (ws.readyState !== WebSocket.OPEN) {
        return;
      }
      // Match umkim CLI / KubeVirt: send keystrokes as binary frames.
      ws.send(new TextEncoder().encode(data));
    });
    term.attachCustomKeyEventHandler((event) => {
      event.stopPropagation();
      return true;
    });
    const onResize = () => fit.fit();
    window.addEventListener('resize', onResize);

    return () => {
      window.removeEventListener('resize', onResize);
      disp.dispose();
      ws.close();
      term.dispose();
      termRef.current = null;
    };
  }, [active, clusterName, namespace, name]);

  if (!active) {
    return null;
  }
  return (
    <div
      ref={containerRef}
      data-testid="vm-serial-console"
      role="application"
      tabIndex={0}
      onMouseDown={(e) => {
        e.preventDefault();
        termRef.current?.focus();
      }}
      style={{
        height: '420px',
        width: '100%',
        background: '#1e1e1e',
        padding: '4px',
        outline: 'none',
        cursor: 'text',
      }}
    />
  );
}
