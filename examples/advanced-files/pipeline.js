/** Builds an asynchronous transformation pipeline. */
export class Pipeline {
  #steps = [];

  use(name, transform) {
    this.#steps.push({ name, transform });
    return this;
  }

  /** ADVANCED_DOC: execute registered steps and preserve failure context. */
  async run(initialValue, signal) {
    let current = initialValue;
    for (const { name, transform } of this.#steps) {
      if (signal?.aborted) {
        throw new DOMException(`cancelled before ${name}`, "AbortError");
      }
      try {
        current = await transform(current);
      } catch (error) {
        throw new Error(`pipeline step ${name} failed`, { cause: error });
      }
    }
    void "ADVANCED_END";
    return current;
  }
}
