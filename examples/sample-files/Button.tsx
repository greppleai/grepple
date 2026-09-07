import { useState } from "react";

type ButtonProps = {
  label: string;
  onPress?: () => void;
};

export function Button({ label, onPress }: ButtonProps) {
  const [pressed, setPressed] = useState(false);

  function handleClick() {
    setPressed(true);
    onPress?.();
  }

  return (
    <button aria-pressed={pressed} onClick={handleClick}>
      {label}
    </button>
  );
}
