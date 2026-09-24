import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { SiteProvisionerInventoryPanel } from './SiteProvisionerInventoryPanel';

const getSiteInventory = vi.fn();
const listSiteInventoryFiles = vi.fn();
const listSiteInventoryRevisions = vi.fn();
const getSiteInventoryFileOptional = vi.fn();
const syncSiteInventory = vi.fn();

vi.mock('../../api/client', () => ({
  getSiteInventory: (...args: unknown[]) => getSiteInventory(...args),
  listSiteInventoryFiles: (...args: unknown[]) => listSiteInventoryFiles(...args),
  listSiteInventoryRevisions: (...args: unknown[]) => listSiteInventoryRevisions(...args),
  getSiteInventoryFileOptional: (...args: unknown[]) => getSiteInventoryFileOptional(...args),
  syncSiteInventory: (...args: unknown[]) => syncSiteInventory(...args),
  putSiteInventoryFile: vi.fn(),
  deleteSiteInventoryFile: vi.fn(),
  applySiteInventoryImport: vi.fn(),
  getSiteInventoryImportPreview: vi.fn(),
  getSiteInventoryRevision: vi.fn(),
  previewSiteInventoryImport: vi.fn(),
}));

describe('SiteProvisionerInventoryPanel dirty buffer', () => {
  beforeEach(() => {
    getSiteInventory.mockResolvedValue({ meta: { syncPhase: 'synced' } });
    listSiteInventoryFiles.mockResolvedValue([{ path: 'guests.yml' }]);
    listSiteInventoryRevisions.mockResolvedValue([]);
    getSiteInventoryFileOptional.mockResolvedValue({ content: 'guests: {}\n', missing: false });
    syncSiteInventory.mockResolvedValue({});
  });

  it('keeps an edited buffer when sync reloads the file list', async () => {
    render(<SiteProvisionerInventoryPanel siteName="homelab" />);
    await waitFor(() => expect(screen.getByTestId('site-inventory-editor')).toHaveValue('guests: {}\n'));

    fireEvent.change(screen.getByTestId('site-inventory-editor'), { target: { value: 'draft: true\n' } });
    expect(screen.getByTestId('site-inventory-editor')).toHaveValue('draft: true\n');
    getSiteInventoryFileOptional.mockResolvedValue({ content: 'server: new\n', missing: false });
    fireEvent.click(screen.getByRole('button', { name: 'Sync to jumphost' }));

    await waitFor(() => expect(syncSiteInventory).toHaveBeenCalled());
    expect(screen.getByTestId('site-inventory-editor')).toHaveValue('draft: true\n');
  });
});
