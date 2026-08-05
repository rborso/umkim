import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { AuthProvider } from './auth/AuthContext';
import { RequireAuth } from './auth/RequireAuth';
import { AppLayout } from './components/AppLayout';
import { OverviewPage } from './pages/OverviewPage';
import { ProvidersPage } from './pages/ProvidersPage';
import { CatalogPage } from './pages/CatalogPage';
import { InstancesPage } from './pages/InstancesPage';
import { PoliciesPage } from './pages/PoliciesPage';
import { SitesPage } from './pages/SitesPage';
import { SiteDetailPage } from './pages/SiteDetailPage';
import { ClustersPage } from './pages/ClustersPage';
import { ClusterDetailPage } from './pages/ClusterDetailPage';
import { InfrastructurePage } from './pages/InfrastructurePage';
import { LoginPage } from './pages/LoginPage';
import { UsersPage } from './pages/UsersPage';
import { GroupsPage } from './pages/GroupsPage';
import { RoleBindingsPage } from './pages/RoleBindingsPage';
import { ServiceAccountsPage } from './pages/ServiceAccountsPage';
import { AuditPage } from './pages/AuditPage';
import { OrgsPage } from './pages/OrgsPage';
import { LLMApiKeysPage } from './pages/LLMApiKeysPage';
import { MembersPage } from './pages/MembersPage';

export function App() {
  return (
    <AuthProvider>
      <BrowserRouter basename="/fleet">
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route
            path="/"
            element={
              <RequireAuth>
                <AppLayout />
              </RequireAuth>
            }
          >
            <Route index element={<OverviewPage />} />
            <Route path="providers" element={<ProvidersPage />} />
            <Route path="catalog" element={<CatalogPage />} />
            <Route path="instances" element={<InstancesPage />} />
            <Route path="operators" element={<Navigate to="/instances" replace />} />
            <Route path="domains" element={<Navigate to="/sites" replace />} />
            <Route path="policies" element={<PoliciesPage />} />
            <Route path="sites" element={<SitesPage />} />
            <Route path="sites/:name" element={<SiteDetailPage />} />
            <Route path="infrastructure" element={<InfrastructurePage />} />
            <Route path="clusters/umkim-hub" element={<Navigate to="/infrastructure" replace />} />
            <Route path="clusters" element={<ClustersPage />} />
            <Route path="clusters/:name" element={<ClusterDetailPage />} />
            <Route path="admin/users" element={<UsersPage />} />
            <Route path="admin/groups" element={<GroupsPage />} />
            <Route path="admin/role-bindings" element={<RoleBindingsPage />} />
            <Route path="admin/service-accounts" element={<ServiceAccountsPage />} />
            <Route path="admin/audit" element={<AuditPage />} />
            <Route path="admin/orgs" element={<OrgsPage />} />
            <Route path="admin/members" element={<MembersPage />} />
            <Route path="admin/llm-api-keys" element={<LLMApiKeysPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  );
}
