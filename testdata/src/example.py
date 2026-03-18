"""Example Python module for testing symbol extraction."""


class Config:
    """Holds server configuration."""

    host: str
    port: int

    def __init__(self, host: str, port: int) -> None:
        self.host = host
        self.port = port

    def validate(self) -> bool:
        """Check that the config is valid."""
        return self.host != "" and self.port > 0


def main() -> None:
    """Entry point."""
    config = Config(host="localhost", port=8080)
    _ = config.validate()
