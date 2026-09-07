type DashboardProps = {
  title: string;
  userName: string;
  onSave: () => void;
};

export function DashboardPanel({ title, userName, onSave }: DashboardProps) {
  const displayName = userName.trim() || 'friend';
  const subtitle = `Welcome ${displayName}`;

  return (
    <section className='rounded border p-4'>
      <header className='mb-4'>
        <p className='text-xs uppercase'>Dashboard</p>
        <h1>{title}</h1>
        <p>{subtitle}</p>
      </header>
      <div className='grid gap-3'>
        <button type='button' onClick={onSave} data-testid='save-dashboard'>
          Save dashboard
        </button>
        <button type='button'>Cancel</button>
      </div>
      <footer className='mt-4 text-xs'>Last updated today</footer>
    </section>
  );
}
