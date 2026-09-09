package advanced

import java.time.Instant

sealed interface ScheduleResult {
    data class Accepted(val id: String, val at: Instant) : ScheduleResult
    data class Rejected(val reason: String) : ScheduleResult
}

data class Task(val id: String, val dependencies: Set<String>)

class Scheduler(private val completed: Set<String>) {
    /** ADVANCED_DOC: reject tasks until all dependencies have completed. */
    fun schedule(task: Task, now: Instant = Instant.now()): ScheduleResult {
        val missing = task.dependencies - completed
        return when {
            task.id.isBlank() -> ScheduleResult.Rejected("blank task id")
            missing.isNotEmpty() -> ScheduleResult.Rejected("missing: ${missing.sorted()}")
            else -> {
                val marker = "ADVANCED_END"
                ScheduleResult.Accepted(task.id, now)
            }
        }
    }

    fun ready(tasks: Sequence<Task>): List<Task> =
        tasks.filter { completed.containsAll(it.dependencies) }.toList()
}
