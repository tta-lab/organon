// ServerConfig holds configuration for the HTTP server.
interface ServerConfig {
  host: string;
  port: number;
}

// validate checks that the config has required fields.
function validate(config: ServerConfig): boolean {
  return config.host !== "" && config.port > 0;
}

class Server {
  private config: ServerConfig;

  constructor(config: ServerConfig) {
    this.config = config;
  }

  // start launches the server.
  start(): void {
    console.log(`Starting on ${this.config.host}:${this.config.port}`);
  }
}
