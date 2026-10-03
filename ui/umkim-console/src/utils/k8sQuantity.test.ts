import { describe, expect, it } from 'vitest';
import {
  formatCpuDisplay,
  formatCpuQuantityValue,
  formatMemoryDisplay,
  formatResourceUsagePair,
  parseCpuQuantityParts,
  parseCpuToMillicores,
} from './k8sQuantity';

describe('k8sQuantity', () => {
  describe('formatCpuDisplay', () => {
    it('converts nanocores to millicores', () => {
      expect(formatCpuDisplay('470582637n')).toBe('471m');
    });

    it('keeps whole cores', () => {
      expect(formatCpuDisplay('8')).toBe('8');
    });

    it('formats fractional cores', () => {
      expect(formatCpuDisplay('0.5')).toBe('500m');
      expect(formatCpuDisplay('7.8')).toBe('7.8');
    });

    it('formats millicores input', () => {
      expect(formatCpuDisplay('7800m')).toBe('7.8');
    });
  });

  describe('parseCpuToMillicores', () => {
    it('parses common suffixes', () => {
      expect(parseCpuToMillicores('1')).toBe(1000);
      expect(parseCpuToMillicores('500m')).toBe(500);
      expect(parseCpuToMillicores('0.5')).toBe(500);
    });
  });

  describe('formatMemoryDisplay', () => {
    it('normalizes Ki to Mi/Gi', () => {
      expect(formatMemoryDisplay('16384Ki')).toBe('16Mi');
      expect(formatMemoryDisplay('16777216Ki')).toBe('16Gi');
    });

    it('keeps Mi unchanged', () => {
      expect(formatMemoryDisplay('512Mi')).toBe('512Mi');
    });

    it('normalizes large Ki values', () => {
      expect(formatMemoryDisplay('17179869184Ki')).toBe('16Ti');
    });
  });

  describe('formatResourceUsagePair', () => {
    it('formats cpu used/capacity', () => {
      expect(formatResourceUsagePair('470582637n', '8', 'cpu')).toBe('471m / 8');
    });

    it('formats memory used/capacity', () => {
      expect(formatResourceUsagePair('12Gi', '16Gi', 'memory')).toBe('12Gi / 16Gi');
    });

    it('returns em dash when empty', () => {
      expect(formatResourceUsagePair(undefined, undefined, 'cpu')).toBe('—');
    });
  });

  describe('parseCpuQuantityParts / formatCpuQuantityValue', () => {
    it('round-trips millicores', () => {
      const parts = parseCpuQuantityParts('500m');
      expect(parts).toEqual({ amount: 500, unit: 'millicores' });
      expect(formatCpuQuantityValue(parts)).toBe('500m');
    });

    it('round-trips cores', () => {
      const parts = parseCpuQuantityParts('1');
      expect(parts).toEqual({ amount: 1, unit: 'cores' });
      expect(formatCpuQuantityValue(parts)).toBe('1');
    });

    it('round-trips decimal cores', () => {
      const parts = parseCpuQuantityParts('0.5');
      expect(parts.unit).toBe('millicores');
      expect(formatCpuQuantityValue(parts)).toBe('500m');
    });

    it('enforces minimum 1m on emit', () => {
      expect(formatCpuQuantityValue({ amount: 0, unit: 'millicores' })).toBe('1m');
    });
  });
});
