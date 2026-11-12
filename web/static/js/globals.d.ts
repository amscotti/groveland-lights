// Global browser APIs provided by vendored libraries and test hooks.
declare global {
  interface Window {
    htmx?: { config: { allowEval: boolean } };
    Alpine?: { data: (name: string, factory: () => Record<string, unknown>) => void };
    __glMapInits?: number;
    __map?: unknown;
  }
}

export {};
