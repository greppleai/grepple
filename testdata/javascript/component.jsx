export function Button({ label, onClick }) {
  return (
    <button type="button" onClick={onClick}>
      {label.trim()}
    </button>
  );
}
