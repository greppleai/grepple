type WidgetProps = {
  title: string;
};

export function Widget({ title }: WidgetProps) {
  return <section>{title}</section>;
}
