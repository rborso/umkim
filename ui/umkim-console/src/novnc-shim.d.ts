declare module '@novnc/novnc/lib/rfb.js' {
  export default class RFB {
    constructor(
      target: HTMLElement,
      urlOrChannel: string | WebSocket,
      options?: { wsProtocols?: string[] },
    );
    scaleViewport: boolean;
    resizeSession: boolean;
    disconnect(): void;
  }
}
