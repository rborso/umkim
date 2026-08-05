import type { APIRequestContext } from '@playwright/test';
import { dcmApiUrl } from './env';

export async function catalogItemExists(
  apiRequest: APIRequestContext,
  catalogId: string,
): Promise<boolean> {
  const res = await apiRequest.get(`${dcmApiUrl()}/catalog-items/${catalogId}`);
  return res.ok();
}

export async function requireCatalogItem(
  apiRequest: APIRequestContext,
  catalogId: string,
): Promise<boolean> {
  const exists = await catalogItemExists(apiRequest, catalogId);
  if (!exists) {
    return false;
  }
  return true;
}
