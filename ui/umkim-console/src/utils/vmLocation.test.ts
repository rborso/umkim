import { describe, expect, it } from 'vitest';
import { vmLocationIsScheduleToken } from './vmLocation';

describe('vmLocationIsScheduleToken', () => {
  it('treats schedule tokens as deferring placement', () => {
    for (const tok of ['', 'least-utilized', 'least_utilized', 'auto', ' LEAST-UTILIZED ']) {
      expect(vmLocationIsScheduleToken(tok)).toBe(true);
    }
  });

  it('treats concrete hypervisor names as placed', () => {
    expect(vmLocationIsScheduleToken('virt01')).toBe(false);
    expect(vmLocationIsScheduleToken('virt02')).toBe(false);
  });
});
