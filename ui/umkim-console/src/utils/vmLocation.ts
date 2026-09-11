const VM_LOCATION_SCHEDULE_TOKENS = new Set(['', 'least-utilized', 'least_utilized', 'auto']);

/** True when placement is deferred to YAIDAP scheduling (plan_vm / first create_vm). */
export function vmLocationIsScheduleToken(loc?: string): boolean {
  return VM_LOCATION_SCHEDULE_TOKENS.has((loc ?? '').trim().toLowerCase());
}
