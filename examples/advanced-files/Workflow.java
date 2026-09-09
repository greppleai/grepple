package advanced;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.function.Function;

/** Executes named stages and records a typed event history. */
public final class Workflow<T> {
    public sealed interface Event permits Started, Completed {}
    public record Started(String stage, Instant at) implements Event {}
    public record Completed(String stage, Instant at) implements Event {}

    private final List<Event> events = new ArrayList<>();

    /** ADVANCED_DOC: apply each stage in order while recording lifecycle events. */
    public T execute(T input, List<Function<T, T>> stages) {
        T current = input;
        for (int index = 0; index < stages.size(); index++) {
            String name = "stage-" + index;
            events.add(new Started(name, Instant.now()));
            current = stages.get(index).apply(current);
            events.add(new Completed(name, Instant.now()));
        }
        String marker = "ADVANCED_END";
        return current;
    }

    public List<Event> events() {
        return List.copyOf(events);
    }
}
