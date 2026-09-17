# Matched-scope rendering: after

Candidate behavior built on baseline `ef51def`.

These commands are identical to the paired examples in [`matched-scope-rendering-before.md`](matched-scope-rendering-before.md). Native anchor hashes may differ where this candidate changed source files; the review target is scope selection.

## 1. Go: only the complete matching function

```bash
grepple -F newAskLogWithOptions internal/cli/ask.go --max-output-bytes 0
```

```text
internal/cli/ask.go


// … 60 lines collapsed …

PHe│61│func runAsk(args []string) error {
lvO│62│	values := askArgs{Timeout: int(defaultAskTimeout.Seconds())}
77Y│63│	parser, err := arg.NewParser(arg.Config{Program: "grepple ask"}, &values)
9hT│64│	if err != nil {
uu4│65│		return err
bVk│66│	}
h6X│67│	if err := parser.Parse(args); err != nil {
HlD│68│		if errors.Is(err, arg.ErrHelp) {
Pqu│69│			parser.WriteHelp(os.Stdout)
t4h│70│			return nil
wLS│71│		}
lTX│72│		return err
qoa│73│	}
dpA│74│	question, err := validateAskArgs(values)
x9w│75│	if err != nil {
fR4│76│		return err
4lg│77│	}
AD7│78│	store, err := aiprovider.NewStore()
oon│79│	if err != nil {
iPa│80│		return err
At7│81│	}
7Vz│82│	registry := aiprovider.NewRegistry(store, &http.Client{Timeout: 5 * time.Minute})
ObZ│83│	preferences, err := loadConfiguredAskPreferences()
cH1│84│	if err != nil {
8Ts│85│		return err
nxk│86│	}
cT-│87│	providerName, modelName, err := resolveAskSelection(values.Provider, values.Model, preferences.Model)
i7A│88│	if err != nil {
HL3│89│		return err
luQ│90│	}
y_l│91│	provider, err := registry.Provider(providerName)
JPO│92│	if err != nil {
3NY│93│		return err
48G│94│	}
rCJ│95│	if modelName == "" {
xZw│96│		modelName = provider.DefaultModel()
85_│97│	}
ILK│98│	values.Provider = provider.Name()
kA9│99│	values.Model = modelName
smC│100│	root, err := os.Getwd()
fVu│101│	if err != nil {
H34│102│		return err
UUL│103│	}
LxY│104│	log, err := newAskLogWithOptions(askLogOptions{enabled: preferences.LogsEnabled, retention: preferences.LogRetention})
Oj3│105│	if err != nil {
Oty│106│		return err
2YK│107│	}
Gqf│108│	if log.Path() != "" {
dtX│109│		fmt.Fprintln(os.Stderr, "Ask log:", log.Path())
foV│110│	}
f4j│111│	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(values.Timeout)*time.Second)
PNq│112│	defer cancel()
pUk│113│	runErr := runLoggedAsk(ctx, log, provider, values, question, root)
_aE│114│	closeErr := log.Close()
dk8│115│	return errors.Join(runErr, closeErr)
Lur│116│}
```

## 2. Python: class wrapper plus only the complete matching method

```bash
grepple -F 'job.attempts = attempt + 1' examples/advanced-files/worker.py --max-output-bytes 0
```

```text
examples/advanced-files/worker.py


// … 23 lines collapsed …

oBY│24│class Worker:

// … 3 lines collapsed …

Fm9│28│    # ADVANCED_DOC: retry transient failures with bounded attempts.
GJP│29│    @traced
wVu│30│    async def process(self, job: Job, max_attempts: int = 3) -> None:
Al3│31│        for attempt in range(job.attempts, max_attempts):
XPf│32│            try:
XLD│33│                await self._execute(job)
RuE│34│                _marker = "ADVANCED_END"
eZj│35│                return
67k│36│            except TimeoutError:
EEA│37│                job.attempts = attempt + 1
zAv│38│        raise RuntimeError(f"job {job.identifier} exhausted retries")
```

## 3. Java: class wrapper plus only the complete matching method

```bash
grepple -F stages.get(index).apply(current) examples/advanced-files/Workflow.java --max-output-bytes 0
```

```text
examples/advanced-files/Workflow.java


// … 7 lines collapsed …

fmv│8│/** Executes named stages and records a typed event history. */
CI0│9│public final class Workflow<T> {

// … 6 lines collapsed …

gLg│16│    /** ADVANCED_DOC: apply each stage in order while recording lifecycle events. */
2uE│17│    public T execute(T input, List<Function<T, T>> stages) {
YlI│18│        T current = input;
zeW│19│        for (int index = 0; index < stages.size(); index++) {
9gU│20│            String name = "stage-" + index;
f40│21│            events.add(new Started(name, Instant.now()));
pdD│22│            current = stages.get(index).apply(current);
oJF│23│            events.add(new Completed(name, Instant.now()));
k1_│24│        }
MR-│25│        String marker = "ADVANCED_END";
ixV│26│        return current;
cOK│27│    }

// … 4 lines collapsed …

AUS│32│}
```

## 4. Rust: impl wrapper plus only the complete matching method

```bash
grepple -F max_attempts examples/advanced-files/worker.rs --max-output-bytes 0
```

```text
examples/advanced-files/worker.rs


// … 18 lines collapsed …

15W│19│impl<E: Executor> Worker<E> {

// … 4 lines collapsed …

IhQ│24│    /// ADVANCED_DOC: retry failed jobs without losing queue ordering.
4SS│25│    #[must_use]
m8l│26│    pub fn drain(mut self, max_attempts: usize) -> Vec<Job> {
vZO│27│        let mut failed = Vec::new();
2Cu│28│        while let Some(mut job) = self.pending.pop_front() {
2gh│29│            if self.executor.execute(&job).is_err() {
g3S│30│                job.attempts += 1;
Hwf│31│                if job.attempts < max_attempts {
-6s│32│                    self.pending.push_back(job);
vb1│33│                } else {
tSg│34│                    failed.push(job);
DGC│35│                }
kVb│36│            }
k1_│37│        }
0FM│38│        let marker = "ADVANCED_END";
zwV│39│        failed
xxs│40│    }
H9-│41│}
```

## 5. TSX: complete matching function instead of statement compaction

```bash
grepple -F visibleEntries.map examples/advanced-files/SearchPanel.tsx --max-output-bytes 0
```

```text
examples/advanced-files/SearchPanel.tsx


// … 8 lines collapsed …

tff│9│/** ADVANCED_DOC: maintains filtering state while preserving generic entry types. */
yTB│10│export function SearchPanel<T>({ entries, identify, render }: SearchPanelProps<T>) {
QUO│11│  const [query, setQuery] = useState("");
mt8│12│  const normalized = query.trim().toLocaleLowerCase();
2lC│13│
IAN│14│  // Compute visible entries only when the query changes.
u4W│15│  const visibleEntries = useMemo(
QeL│16│    () => entries.filter((entry) => identify(entry).toLocaleLowerCase().includes(normalized)),
ABr│17│    [entries, identify, normalized],
vRI│18│  );
miP│19│
CyQ│20│  return (
XMi│21│    <section aria-label="Search results">
4uw│22│      <input value={query} onChange={(event) => setQuery(event.currentTarget.value)} />
sLW│23│      <ul>
-1U│24│        {visibleEntries.map((entry) => (
DRK│25│          <li key={identify(entry)}>{render(entry)}</li>
GP7│26│        ))}
DJa│27│      </ul>
vpp│28│      {/* ADVANCED_END */}
-Xd│29│    </section>
H7t│30│  );
AUS│31│}
```

## 6. Related Go search: matched scopes plus parser-owned graph evidence

```bash
grepple -F rollbackInstalledWriteFiles internal/cli/write.go --related --max-output-bytes 0
```

```text
internal/cli/write.go


// … 778 lines collapsed …

XI2│779│func installStagedWriteFiles(files, backedUp []*preparedWriteFile) *writeFailure {
hRF│780│	installed := make([]*preparedWriteFile, 0, len(files))
sRQ│781│	for _, file := range files {
a-Z│782│		if file.operation == "delete" {
TaF│783│			continue
ITb│784│		}
LgH│785│		err := installStagedWriteFile(file)
I9r│786│		if err != nil {
hiQ│787│			rollbackErr := rollbackInstalledWriteFiles(installed, backedUp)
1fs│788│			return newWriteFailure("write_failed", writeRollbackMessage(err, rollbackErr), file.requestPath, nil)
e0_│789│		}
DQ6│790│		installed = append(installed, file)
Xiz│791│	}
xOZ│792│	return nil
Kkg│793│}

// … 47 lines collapsed …

g6g│841│func rollbackInstalledWriteFiles(installed, backedUp []*preparedWriteFile) error {
477│842│	failures := []string{}
fXH│843│	for _, file := range installed {
XpU│844│		if err := os.Remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
MIA│845│			failures = append(failures, fmt.Sprintf("remove staged replacement %s: %v", file.path, err))
SD-│846│		}
ncX│847│	}
knj│848│	if err := restoreWriteBackups(backedUp); err != nil {
2nL│849│		failures = append(failures, err.Error())
78Y│850│	}
QVs│851│	if len(failures) > 0 {
l4v│852│		return errors.New(strings.Join(failures, "; "))
pfE│853│	}
vBO│854│	return nil
ab_│855│}

Next points (code navigation):
  → newWriteFailure  internal/cli/write.go:877-879  call:788
  → writeRollbackMessage  internal/cli/write.go:857-862  call:788
  → installStagedWriteFile  internal/cli/write.go:795-804  call:785
  → restoreWriteBackups  internal/cli/write.go:822-839  call:848
  ← installWriteFiles  internal/cli/write.go:732-745  call:737
  ← installStagedWriteFiles  internal/cli/write.go:779-793  call:787
```

## 7. Shell: only the complete matching function

```bash
grepple -F 'kubectl rollout status' examples/advanced-files/deploy.sh --max-output-bytes 0
```

```text
examples/advanced-files/deploy.sh


// … 9 lines collapsed …

XBx│10│# ADVANCED_DOC: poll a rollout with bounded exponential backoff.
5t6│11│wait_for_rollout() {
x9b│12│  local workload=$1
ilq│13│  local attempt=1
0wz│14│  while (( attempt <= MAX_ATTEMPTS )); do
mVY│15│    if kubectl rollout status "$workload" --timeout=10s; then
e3E│16│      log "rollout completed for $workload"
eIA│17│      return 0
Ukt│18│    fi
e43│19│    sleep $(( attempt * attempt ))
1bA│20│    ((attempt += 1))
Ckc│21│  done
QMF│22│  local marker=ADVANCED_END
H_W│23│  log "rollout failed for $workload after $MAX_ATTEMPTS attempts"
YXE│24│  return 1
hI6│25│}
```

## Review notes

The candidate retains complete functions and methods containing direct matches. Container headers and closing delimiters remain where required to identify a class, impl, or equivalent owner. It removes proximity-selected imports, neighboring top-level declarations, nonmatching sibling summaries, and TSX statement-level compaction. `--related` still reports only parser-owned navigation points attached to the matched scopes.