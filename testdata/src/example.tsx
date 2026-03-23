// ButtonProps defines the props for the Button component.
interface ButtonProps {
  label: string;
  onClick: () => void;
  disabled?: boolean;
}

// Button is a reusable button component.
function Button({ label, onClick, disabled }: ButtonProps): JSX.Element {
  return <button onClick={onClick} disabled={disabled}>{label}</button>;
}

class Counter {
  private count: number;

  constructor() {
    this.count = 0;
  }

  increment(): void {
    this.count++;
  }
}

type Theme = "light" | "dark";
