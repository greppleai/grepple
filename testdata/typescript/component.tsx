type GreetingProps = {
  name: string;
};

export function Greeting({ name }: GreetingProps) {
  const displayName = name.trim() || "friend";
  return <h1>Hello {displayName}</h1>;
}
