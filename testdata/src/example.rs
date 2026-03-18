/// Config holds server configuration.
pub struct Config {
    pub host: String,
    pub port: u16,
}

impl Config {
    /// new creates a Config with the given host and port.
    pub fn new(host: String, port: u16) -> Self {
        Config { host, port }
    }

    /// validate checks that the config is valid.
    pub fn validate(&self) -> bool {
        !self.host.is_empty() && self.port > 0
    }
}

fn main() {
    let c = Config::new("localhost".to_string(), 8080);
    let _ = c.validate();
}
