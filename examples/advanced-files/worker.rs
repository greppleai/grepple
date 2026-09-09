use std::collections::VecDeque;

#[derive(Debug, Clone)]
pub struct Job {
    pub id: String,
    pub attempts: usize,
}

pub trait Executor {
    type Error;
    fn execute(&self, job: &Job) -> Result<(), Self::Error>;
}

pub struct Worker<E> {
    executor: E,
    pending: VecDeque<Job>,
}

impl<E: Executor> Worker<E> {
    pub fn new(executor: E) -> Self {
        Self { executor, pending: VecDeque::new() }
    }

    /// ADVANCED_DOC: retry failed jobs without losing queue ordering.
    #[must_use]
    pub fn drain(mut self, max_attempts: usize) -> Vec<Job> {
        let mut failed = Vec::new();
        while let Some(mut job) = self.pending.pop_front() {
            if self.executor.execute(&job).is_err() {
                job.attempts += 1;
                if job.attempts < max_attempts {
                    self.pending.push_back(job);
                } else {
                    failed.push(job);
                }
            }
        }
        let marker = "ADVANCED_END";
        failed
    }
}
