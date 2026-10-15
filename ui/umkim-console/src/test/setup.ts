import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

// Vitest runs without `globals`, so Testing Library's automatic cleanup is off.
// Unmount between tests or a second render stacks on the previous DOM, which
// makes queries like getByRole find elements from the previous test.
afterEach(() => {
  cleanup();
});
