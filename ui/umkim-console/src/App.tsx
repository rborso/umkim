import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { AuthProvider } from './auth/AuthContext';
import { RequireAuth } from './auth/RequireAuth';
import { AppLayout } from './components/AppLayout';
import { AppsPage } from './pages/AppsPage';
import { LlamaProfilesPage } from './pages/LlamaProfilesPage';
import { OverviewPage } from './pages/OverviewPage';
import { SitesPage } from './pages/SitesPage';
import { SiteDetailPage } from './pages/SiteDetailPage';
import { ClustersPage } from './pages/ClustersPage';
import { ClusterDetailPage } from './pages/ClusterDetailPage';
import { ClusterVMsPage } from './pages/ClusterVMsPage';
import { InfrastructurePage } from './pages/InfrastructurePage';
import { LoginPage } from './pages/LoginPage';
import { InviteRedeemPage } from './pages/InviteRedeemPage';
import { InvitesPage } from './pages/InvitesPage';
import { TenantsPage } from './pages/TenantsPage';
import { UsersPage } from './pages/UsersPage';
import { GroupsPage } from './pages/GroupsPage';
import { RoleBindingsPage } from './pages/RoleBindingsPage';
import { ServiceAccountsPage } from './pages/ServiceAccountsPage';
import { AuditPage } from './pages/AuditPage';
import { LLMApiKeysPage } from './pages/LLMApiKeysPage';

export function App() {
  return (
    <AuthProvider>
      <BrowserRouter basename="/fleet">
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/invite" element={<InviteRedeemPage />} />
          <Route
            path="/"
            element={
              <RequireAuth>
                <AppLayout />
              </RequireAuth>
            }
          >
            <Route index element={<OverviewPage />} />
            <Route path="sites" element={<SitesPage />} />
            <Route path="sites/:name" element={<SiteDetailPage />} />
            <Route path="infrastructure" element={<InfrastructurePage />} />
            <Route path="clusters/umkim-hub" element={<Navigate to="/infrastructure" replace />} />
            <Route path="clusters" element={<ClustersPage />} />
            <Route path="clusters/:name" element={<ClusterDetailPage />} />
            <Route path="clusters/:name/vms" element={<ClusterVMsPage />} />
            <Route path="apps" element={<AppsPage />} />
            <Route path="apps/llama-profiles" element={<LlamaProfilesPage />} />
            <Route path="admin/users" element={<UsersPage />} />
            <Route path="admin/groups" element={<GroupsPage />} />
            <Route path="admin/role-bindings" element={<RoleBindingsPage />} />
            <Route path="admin/service-accounts" element={<ServiceAccountsPage />} />
            <Route path="admin/invites" element={<InvitesPage />} />
            <Route path="admin/tenants" element={<TenantsPage />} />
            <Route path="admin/audit" element={<AuditPage />} />
            <Route path="admin/llm-api-keys" element={<LLMApiKeysPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  );
}
