
export class GameSocket {
    constructor(url) {
        this.url = url;
        this.socket = null;
        this.onMessage = () => {};
        this.onStatus = () => {};
    }

    connect() {
        if (this.socket &&
            this.socket.readyState === WebSocket.OPEN) {
            return;
        }

        this.onStatus("connecting");

        try {
            this.socket = new WebSocket(this.url);

            this.socket.addEventListener("open", () => {
                this.onStatus("connected");
            });

            this.socket.addEventListener("message", (event) => {
                try {
                    const message = JSON.parse(event.data);
                    this.onMessage(message);
                } catch {
                    this.onStatus("invalid-message");
                }
            });

            this.socket.addEventListener("error", () => {
                this.onStatus("error");
            });

            this.socket.addEventListener("close", () => {
                this.onStatus("disconnected");
            });
        } catch {
            this.onStatus("error");
        }
    }

    send(message) {
        if (!this.socket ||
            this.socket.readyState !== WebSocket.OPEN) {
            return false;
        }

        this.socket.send(JSON.stringify(message));
        return true;
    }

    disconnect() {
        if (this.socket) {
            this.socket.close();
            this.socket = null;
        }

        this.onStatus("disconnected");
    }
}
