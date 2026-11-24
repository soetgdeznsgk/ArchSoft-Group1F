import { describe, expect, it, vi } from "vitest";

describe("GameSocket", () => {
  it("imports and sends through an open fake socket", async () => {
    class FakeWebSocket {
      static OPEN = 1;
      constructor(url) {
        this.url = url;
        this.readyState = 1;
        this.listeners = {};
      }
      addEventListener(type, callback) {
        this.listeners[type] = callback;
      }
      send = vi.fn();
      close = vi.fn();
    }
    vi.stubGlobal("WebSocket", FakeWebSocket);
    const { GameSocket } = await import("../../../presentation/game/client/ingame/game-socket.js");
    const socket = new GameSocket("wss://example.test/ws");
    socket.connect();
    expect(socket.send({ type: "draw", source: "hidden" })).toBe(true);
  });

  it("does not send while disconnected", async () => {
    vi.unstubAllGlobals();
    vi.stubGlobal("WebSocket", undefined);
    const { GameSocket } = await import("../../../presentation/game/client/ingame/game-socket.js");
    const socket = new GameSocket("wss://example.test/ws");
    expect(socket.send({ type: "draw" })).toBe(false);
  });
});
